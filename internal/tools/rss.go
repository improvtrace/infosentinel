package tools

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/mmcdole/gofeed"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/tomasgdl/infosentinel/internal/config"
)

// RSSItem 是返回给 LLM 的单条 RSS 内容。
type RSSItem struct {
	Feed      string `json:"feed"`
	Title     string `json:"title"`
	Link      string `json:"link"`
	Published string `json:"published"` // RFC3339,可能为零值
	Summary   string `json:"summary"`
}

// rssToolParams 是 fetch_rss 工具的参数。
type rssToolParams struct {
	// Feed 是要拉取的源名称;留空表示依次拉取所有已配置的源
	Feed string `json:"feed,omitempty" jsonschema_description:"name of the RSS feed to fetch; omit to fetch all configured feeds"`
}

// rssTool 持有 fetch_rss 工具的依赖。
type rssTool struct {
	feeds    []config.RSSFeed
	client   *http.Client
	window   time.Duration
	maxItems int
	store    *SeenStore
	nowFn    func() time.Time
}

// NewRSSTool 创建 fetch_rss 工具:拉取配置的 RSS/Atom 订阅源,
// 按时间窗与去重状态过滤后返回条目列表。
func NewRSSTool(cfg *config.RSSConfig, collect *config.CollectConfig, store *SeenStore) (tool.InvokableTool, error) {
	t := &rssTool{
		feeds:    cfg.Feeds,
		client:   &http.Client{Timeout: time.Duration(collect.HTTPTimeoutSec) * time.Second},
		window:   time.Duration(collect.WindowHours) * time.Hour,
		maxItems: collect.MaxItemsPerSource,
		store:    store,
		nowFn:    time.Now,
	}
	return utils.InferTool(
		"fetch_rss",
		"Fetch recent articles from the configured RSS/Atom feeds. "+
			"Returns articles published within the time window that have not been reported before. "+
			"Pass feed name to fetch one specific feed, or omit to fetch all feeds.",
		t.invoke,
	)
}

// invoke 执行 RSS 拉取与过滤。
func (t *rssTool) invoke(ctx context.Context, params *rssToolParams) (string, error) {
	now := t.nowFn()
	targets := t.feeds
	if params.Feed != "" {
		targets = nil
		for _, f := range t.feeds {
			if f.Name == params.Feed {
				targets = []config.RSSFeed{f}
				break
			}
		}
		if targets == nil {
			return friendlyResult(map[string]any{
				"error": fmt.Sprintf("feed %q not found; call this tool without arguments to see all configured feeds", params.Feed),
			}), nil
		}
	}

	items := make([]RSSItem, 0, t.maxItems)
	for _, f := range targets {
		got, err := t.fetchOne(ctx, f, now)
		if err != nil {
			// 单个源失败不影响其他源:错误信息返回给 LLM,由其自行取舍
			items = append(items, RSSItem{
				Feed: f.Name, Link: f.URL,
				Summary: fmt.Sprintf("[feed error: %v]", err),
			})
			continue
		}
		items = append(items, got...)
		if len(items) >= t.maxItems {
			items = items[:t.maxItems]
			break
		}
	}
	if len(items) == 0 {
		return friendlyResult(map[string]any{"items": []RSSItem{}, "note": "no new articles within the time window"}), nil
	}
	return friendlyResult(map[string]any{"items": items}), nil
}

// fetchOne 拉取单个订阅源并完成时间窗与去重过滤。
func (t *rssTool) fetchOne(ctx context.Context, f config.RSSFeed, now time.Time) ([]RSSItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("构造请求: %w", err)
	}
	req.Header.Set("User-Agent", "InfoSentinel/0.1 (+rss-collector)")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	fp := gofeed.NewParser()
	feed, err := fp.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("解析 feed: %w", err)
	}

	cutoff := now.Add(-t.window)
	out := make([]RSSItem, 0, len(feed.Items))
	links := make([]string, 0, len(feed.Items))
	for _, it := range feed.Items {
		link := it.Link
		if link == "" && it.GUID != "" {
			link = it.GUID
		}
		// 时间窗过滤:发布时间已知且早于窗口的跳过;未知时间的保留(交由 LLM 判断)
		if it.PublishedParsed != nil && !it.PublishedParsed.IsZero() && it.PublishedParsed.Before(cutoff) {
			continue
		}
		// 去重:本轮已进入过邮件的链接不再返回
		if len(t.store.Filter([]string{link})) == 0 {
			continue
		}
		links = append(links, link)
		out = append(out, RSSItem{
			Feed:      f.Name,
			Title:     it.Title,
			Link:      link,
			Published: formatTimePtr(it.PublishedParsed),
			Summary:   truncate(stripHTML(it.Description), 500),
		})
		if len(out) >= t.maxItems {
			break
		}
	}
	t.store.Mark(links, now)
	return out, nil
}

// formatTimePtr 把 gofeed 的时间指针格式化为 RFC3339,nil/零值返回空串。
func formatTimePtr(ts *time.Time) string {
	if ts == nil || ts.IsZero() {
		return ""
	}
	return ts.Format(time.RFC3339)
}
