package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/tool/duckduckgo/ddgsearch"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
)

// searchResultItem 是返回给 LLM 的单条搜索结果。
type searchResultItem struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

// searchToolParams 是 search_web 工具的参数。
type searchToolParams struct {
	// Query 是搜索关键词
	Query string `json:"query" jsonschema_description:"search query keywords"`
	// TimeRange 限定时间范围:d(一天)/ w(一周)/ m(一月),默认 w
	TimeRange string `json:"time_range,omitempty" jsonschema_description:"result time range: d=day, w=week, m=month; default w"`
}

// searchTool 持有 search_web 工具的依赖。
type searchTool struct {
	ddgs       *ddgsearch.DDGS
	region     string
	maxResults int
	store      *SeenStore
	nowFn      func() time.Time
}

// NewSearchTool 创建 search_web 工具:基于 DuckDuckGo 的网络搜索(无需 API Key)。
func NewSearchTool(cfg *repository.SearchConfig, store *SeenStore) (tool.InvokableTool, error) {
	ddgs, err := ddgsearch.New(&ddgsearch.Config{
		// Region/SafeSearch 等筛选在每次 Search 的 SearchParams 中传入
		Timeout: 0, // 走库默认;外层 agent 请求带 ctx 超时
	})
	if err != nil {
		return nil, fmt.Errorf("初始化 duckduckgo 搜索: %w", err)
	}
	t := &searchTool{
		ddgs:       ddgs,
		region:     cfg.Region,
		maxResults: cfg.MaxResults,
		store:      store,
		nowFn:      time.Now,
	}
	return utils.InferTool(
		"search_web",
		"Search the web with DuckDuckGo for recent news, articles and releases about a topic. "+
			"Returns a list of results with title, url and short description.",
		t.invoke,
	)
}

// invoke 执行搜索并做去重过滤。
func (t *searchTool) invoke(ctx context.Context, params *searchToolParams) (string, error) {
	if strings.TrimSpace(params.Query) == "" {
		return friendlyResult(map[string]any{"error": "query must not be empty"}), nil
	}
	tr := ddgsearch.TimeRange(params.TimeRange)
	switch tr {
	case ddgsearch.TimeRangeDay, ddgsearch.TimeRangeWeek, ddgsearch.TimeRangeMonth:
	case "":
		tr = ddgsearch.TimeRangeWeek
	default:
		return friendlyResult(map[string]any{"error": "time_range must be one of: d, w, m"}), nil
	}

	resp, err := t.ddgs.Search(ctx, &ddgsearch.SearchParams{
		Query:      params.Query,
		Region:     ddgsearch.Region(t.region),
		SafeSearch: ddgsearch.SafeSearchModerate,
		MaxResults: t.maxResults,
		TimeRange:  tr,
	})
	if err != nil {
		return friendlyResult(map[string]any{"error": fmt.Sprintf("search failed: %v", err)}), nil
	}

	now := t.nowFn()
	items := make([]searchResultItem, 0, t.maxResults)
	links := make([]string, 0, t.maxResults)
	for _, r := range resp.Results {
		if len(t.store.Filter([]string{r.URL})) == 0 {
			continue
		}
		links = append(links, r.URL)
		items = append(items, searchResultItem{
			Title:       r.Title,
			URL:         r.URL,
			Description: truncate(r.Description, 300),
		})
	}
	t.store.Mark(links, now)

	if len(items) == 0 {
		return friendlyResult(map[string]any{"results": []searchResultItem{}, "note": "no new results (all filtered as already reported)"}), nil
	}
	return friendlyResult(map[string]any{"results": items}), nil
}
