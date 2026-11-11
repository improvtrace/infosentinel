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
)

// hackerNewsAPI 是 HN Firebase API 根地址。var 以便测试注入 mock 服务。
var hackerNewsAPI = "https://hacker-news.firebaseio.com/v0"

// HN 工具内置参数(无全局配置):min_points/top_n 由 LLM 按主题指令传入。
const (
	// hnDefaultTopN 是默认返回的热榜条数。
	hnDefaultTopN = 15
	// hnMaxTopN 是单次返回条数上限,控制上下文长度。
	hnMaxTopN = 30
	// hnDefaultMinPoints 是默认最低分阈值。
	hnDefaultMinPoints = 50
)

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

// hnToolParams 是 fetch_hackernews 工具的参数。
type hnToolParams struct {
	// MinPoints 是最低分阈值,过滤低热度条目;留空用默认值
	MinPoints int `json:"min_points,omitempty" jsonschema_description:"minimum score threshold; omit for default (50)"`
	// TopN 是返回条数上限;留空用默认值
	TopN int `json:"top_n,omitempty" jsonschema_description:"max stories to return; omit for default (15)"`
}

// hnTool 持有 fetch_hackernews 工具的依赖。
type hnTool struct {
	client *http.Client
	window time.Duration
	store  *SeenStore
	nowFn  func() time.Time
}

// NewHackerNewsTool 创建 fetch_hackernews 工具:拉取 Hacker News 首页热榜。
func NewHackerNewsTool(store *SeenStore) (tool.InvokableTool, error) {
	t := &hnTool{
		client: &http.Client{Timeout: HTTPTimeoutSec * time.Second},
		window: WindowHours * time.Hour,
		store:  store,
		nowFn:  time.Now,
	}
	return utils.InferTool(
		"fetch_hackernews",
		"Fetch current Hacker News front-page stories (top stories with score and comment count). "+
			"Useful for tech news, launches and discussions. "+
			"Pass min_points to filter low-score stories, top_n to limit the result count.",
		t.invoke,
	)
}

// invoke 执行 HN 拉取。
func (t *hnTool) invoke(ctx context.Context, params *hnToolParams) (string, error) {
	now := t.nowFn()

	topN := hnDefaultTopN
	if params.TopN > 0 {
		topN = min(params.TopN, hnMaxTopN)
	}
	minPoints := hnDefaultMinPoints
	if params.MinPoints > 0 {
		minPoints = params.MinPoints
	}

	ids := make([]int, 0, topN*2)
	if err := t.getJSON(ctx, hackerNewsAPI+"/topstories.json", &ids); err != nil {
		return friendlyResult(map[string]any{"error": fmt.Sprintf("fetch topstories failed: %v", err)}), nil
	}
	if len(ids) > topN*3 {
		ids = ids[:topN*3] // 多取一些以留出分数/去重过滤余量
	}

	stories := fetchStories(ctx, t.client, ids, topN*3)

	items := make([]hnItem, 0, topN)
	links := make([]string, 0, topN)
	for _, s := range stories {
		if s.Score < minPoints {
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
		if len(items) >= topN {
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

// trendingTopN 是单次返回的 trending 仓库数上限(无全局配置)。
const trendingTopN = 10

// trendingToolParams 是 fetch_github_trending 工具的参数。
type trendingToolParams struct {
	// Language 是编程语言筛选;留空不过滤;all 表示不过滤
	Language string `json:"language,omitempty" jsonschema_description:"programming language filter, e.g. go, python; omit or 'all' for no filter"`
}

// trendingTool 持有 fetch_github_trending 工具的依赖。
type trendingTool struct {
	client  *http.Client
	baseURL string // 可注入,便于测试
	store   *SeenStore
	nowFn   func() time.Time
}

// NewGitHubTrendingTool 创建 fetch_github_trending 工具:抓取 GitHub Trending 页面。
func NewGitHubTrendingTool(store *SeenStore) (tool.InvokableTool, error) {
	t := &trendingTool{
		client:  &http.Client{Timeout: HTTPTimeoutSec * time.Second},
		baseURL: "https://github.com/trending",
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
	lang := params.Language

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
	items := make([]repoItem, 0, trendingTopN)
	links := make([]string, 0, trendingTopN)
	doc.Find("article.Box").Each(func(_ int, art *goquery.Selection) {
		if len(items) >= trendingTopN {
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
