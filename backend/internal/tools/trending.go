package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
)

// hackerNewsAPI 是 HN Firebase API 根地址。var 以便测试注入 mock 服务。
var hackerNewsAPI = "https://hacker-news.firebaseio.com/v0"

// hnItem 是返回给 LLM 的单条 HN 热榜条目。
type hnItem struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Score       int    `json:"score"`
	Comments    int    `json:"comments"`
	By          string `json:"by"`
	PublishedAt string `json:"published_at"`
}

// hnStory 是 HN API 的条目原始结构。
type hnStory struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Score       int    `json:"score"`
	By          string `json:"by"`
	Time        int64  `json:"time"`
	Type        string `json:"type"`
	Descendants int    `json:"descendants"`
}

// hnTool 持有 fetch_hackernews 工具的依赖。
type hnTool struct {
	client    *http.Client
	topN      int
	minPoints int
	window    time.Duration
	store     *SeenStore
	nowFn     func() time.Time
}

// NewHackerNewsTool 创建 fetch_hackernews 工具:拉取 Hacker News 首页热榜。
func NewHackerNewsTool(cfg *repository.HackerNewsConfig, collect *repository.CollectConfig, store *SeenStore) (tool.InvokableTool, error) {
	t := &hnTool{
		client:    &http.Client{Timeout: time.Duration(collect.HTTPTimeoutSec) * time.Second},
		topN:      cfg.TopN,
		minPoints: cfg.MinPoints,
		window:    time.Duration(collect.WindowHours) * time.Hour,
		store:     store,
		nowFn:     time.Now,
	}
	return utils.InferTool(
		"fetch_hackernews",
		"Fetch current Hacker News front-page stories (top stories with score and comment count). "+
			"Useful for tech news, launches and discussions.",
		t.invoke,
	)
}

// invoke 执行 HN 拉取。
func (t *hnTool) invoke(ctx context.Context, _ struct{}) (string, error) {
	now := t.nowFn()

	ids := make([]int, 0, t.topN*2)
	if err := t.getJSON(ctx, hackerNewsAPI+"/topstories.json", &ids); err != nil {
		return friendlyResult(map[string]any{"error": fmt.Sprintf("fetch topstories failed: %v", err)}), nil
	}
	if len(ids) > t.topN*3 {
		ids = ids[:t.topN*3] // 多取一些以留出分数/去重过滤余量
	}

	stories := fetchStories(ctx, t.client, ids, t.topN*3)

	items := make([]hnItem, 0, t.topN)
	links := make([]string, 0, t.topN)
	for _, s := range stories {
		if s.Score < t.minPoints {
			continue
		}
		link := s.URL
		if link == "" {
			// Ask HN 等无外链的讨论帖指向 HN 页面本身
			link = fmt.Sprintf("https://news.ycombinator.com/item?id=%d", s.ID)
		}
		pub := time.Unix(s.Time, 0)
		if now.Sub(pub) > t.window {
			continue
		}
		if len(t.store.Filter([]string{link})) == 0 {
			continue
		}
		links = append(links, link)
		items = append(items, hnItem{
			Title:       s.Title,
			URL:         link,
			Score:       s.Score,
			Comments:    s.Descendants,
			By:          s.By,
			PublishedAt: pub.Format(time.RFC3339),
		})
		if len(items) >= t.topN {
			break
		}
	}
	t.store.Mark(links, now)

	if len(items) == 0 {
		return friendlyResult(map[string]any{"items": []hnItem{}, "note": "no qualifying stories"}), nil
	}
	return friendlyResult(map[string]any{"items": items}), nil
}

// getJSON 请求 url 并解析 JSON 到 out。
func (t *hnTool) getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// fetchStories 并发拉取 story 详情(限制并发,避免对 API 压力过大)。
func fetchStories(ctx context.Context, client *http.Client, ids []int, limit int) []hnStory {
	var (
		mu    sync.Mutex
		wg    sync.WaitGroup
		sem   = make(chan struct{}, 8)
		out   = make([]hnStory, 0, limit)
		child context.Context
	)
	child, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	for _, id := range ids {
		if len(out) >= limit {
			break
		}
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-child.Done():
				return
			}
			defer func() { <-sem }()

			var s hnStory
			req, err := http.NewRequestWithContext(child, http.MethodGet,
				fmt.Sprintf("%s/item/%d.json", hackerNewsAPI, id), nil)
			if err != nil {
				return
			}
			resp, err := client.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return
			}
			raw, _ := io.ReadAll(resp.Body)
			if err := json.Unmarshal(raw, &s); err != nil || s.Type != "story" {
				return
			}
			mu.Lock()
			if len(out) < limit {
				out = append(out, s)
			}
			mu.Unlock()
		}(id)
	}
	wg.Wait()
	return out
}

// repoItem 是返回给 LLM 的单个 trending 仓库。
type repoItem struct {
	Repo        string `json:"repo"`
	Description string `json:"description"`
	Language    string `json:"language"`
	StarsToday  string `json:"stars_today"`
	URL         string `json:"url"`
}

// trendingToolParams 是 fetch_github_trending 工具的参数。
type trendingToolParams struct {
	// Language 是编程语言筛选;留空使用配置默认值;all 表示不过滤
	Language string `json:"language,omitempty" jsonschema_description:"programming language filter, e.g. go, python; omit for default; 'all' for no filter"`
}

// trendingTool 持有 fetch_github_trending 工具的依赖。
type trendingTool struct {
	client  *http.Client
	baseURL string // 可注入,便于测试
	defLang string
	topN    int
	store   *SeenStore
	nowFn   func() time.Time
}

// NewGitHubTrendingTool 创建 fetch_github_trending 工具:抓取 GitHub Trending 页面。
func NewGitHubTrendingTool(cfg *repository.GitHubTrendingConfig, collect *repository.CollectConfig, store *SeenStore) (tool.InvokableTool, error) {
	t := &trendingTool{
		client:  &http.Client{Timeout: time.Duration(collect.HTTPTimeoutSec) * time.Second},
		baseURL: "https://github.com/trending",
		defLang: cfg.Language,
		topN:    cfg.TopN,
		store:   store,
		nowFn:   time.Now,
	}
	return utils.InferTool(
		"fetch_github_trending",
		"Fetch today's GitHub trending repositories, optionally filtered by programming language. "+
			"Returns repo name, description, language and stars gained today.",
		t.invoke,
	)
}

// invoke 抓取并解析 GitHub Trending。
func (t *trendingTool) invoke(ctx context.Context, params *trendingToolParams) (string, error) {
	lang := t.defLang
	if params.Language != "" {
		lang = params.Language
	}

	u := t.baseURL + "?since=daily"
	if lang != "" && !strings.EqualFold(lang, "all") {
		u = t.baseURL + "/" + url.PathEscape(lang) + "?since=daily"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return friendlyResult(map[string]any{"error": fmt.Sprintf("build request: %v", err)}), nil
	}
	req.Header.Set("User-Agent", "InfoSentinel/0.1 (+github-trending)")
	resp, err := t.client.Do(req)
	if err != nil {
		return friendlyResult(map[string]any{"error": fmt.Sprintf("request failed: %v", err)}), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return friendlyResult(map[string]any{"error": fmt.Sprintf("HTTP %d from %s", resp.StatusCode, u)}), nil
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return friendlyResult(map[string]any{"error": fmt.Sprintf("parse html: %v", err)}), nil
	}

	now := t.nowFn()
	items := make([]repoItem, 0, t.topN)
	links := make([]string, 0, t.topN)
	doc.Find("article.Box").Each(func(_ int, art *goquery.Selection) {
		if len(items) >= t.topN {
			return
		}
		href, ok := art.Find("h2 a").Attr("href")
		if !ok {
			return
		}
		repoURL := "https://github.com" + href
		name := strings.TrimSpace(strings.TrimPrefix(href, "/"))
		desc := strings.TrimSpace(art.Find("p").First().Text())
		language := strings.TrimSpace(art.Find("[itemprop='programmingLanguage']").First().Text())
		starsToday := strings.TrimSpace(art.Find("span.d-inline-block.float-sm-right").First().Text())
		starsToday = strings.Join(strings.Fields(starsToday), " ")

		if len(t.store.Filter([]string{repoURL})) == 0 {
			return
		}
		links = append(links, repoURL)
		items = append(items, repoItem{
			Repo:        name,
			Description: desc,
			Language:    language,
			StarsToday:  starsToday,
			URL:         repoURL,
		})
	})
	t.store.Mark(links, now)

	if len(items) == 0 {
		return friendlyResult(map[string]any{"repos": []repoItem{}, "note": "no new trending repos"}), nil
	}
	return friendlyResult(map[string]any{"repos": items}), nil
}
