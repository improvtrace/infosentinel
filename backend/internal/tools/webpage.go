package tools

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cloudwego/eino-ext/components/document/loader/url"
	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
)

// webpageToolParams 是 fetch_webpage 工具的参数。
type webpageToolParams struct {
	// URL 是要抓取的页面地址
	URL string `json:"url" jsonschema_description:"full URL of the web page to fetch"`
}

// webpageTool 持有 fetch_webpage 工具的依赖。
type webpageTool struct {
	loader   *url.Loader
	defaults []repository.Webpage
	maxChars int
}

// NewWebpageTool 创建 fetch_webpage 工具:抓取网页并提取正文纯文本。
// 底层使用 Eino 扩展组件 document/loader/url(HTML → 文本)。
// defaults 为配置文件中的固定网页列表,LLM 可通过 list_default_webpages 结果选用。
func NewWebpageTool(pages []repository.Webpage, collect *repository.CollectConfig) (tool.InvokableTool, error) {
	loader, err := url.NewLoader(context.Background(), &url.LoaderConfig{
		Client: &http.Client{Timeout: time.Duration(collect.HTTPTimeoutSec) * time.Second},
	})
	if err != nil {
		return nil, fmt.Errorf("初始化 url loader: %w", err)
	}
	t := &webpageTool{
		loader:   loader,
		defaults: pages,
		maxChars: collect.MaxBodyChars,
	}
	return utils.InferTool(
		"fetch_webpage",
		"Fetch a web page by URL and return its readable text content (HTML stripped, truncated). "+
			"Use it to read the full content of an article found via search or feeds.",
		t.invoke,
	)
}

// invoke 抓取单个网页正文。
func (t *webpageTool) invoke(ctx context.Context, params *webpageToolParams) (string, error) {
	if params.URL == "" {
		return friendlyResult(map[string]any{"error": "url must not be empty"}), nil
	}

	docs, err := t.loader.Load(ctx, document.Source{URI: params.URL})
	if err != nil {
		return friendlyResult(map[string]any{"error": fmt.Sprintf("load %s failed: %v", params.URL, err)}), nil
	}
	if len(docs) == 0 {
		return friendlyResult(map[string]any{"error": fmt.Sprintf("no content returned for %s", params.URL)}), nil
	}

	content := truncate(stripHTML(docs[0].Content), t.maxChars)
	return friendlyResult(map[string]any{
		"url":     params.URL,
		"content": content,
	}), nil
}

// DefaultWebpages 返回配置的固定网页列表,供组装系统提示词时注入。
func DefaultWebpages(pages []repository.Webpage) string {
	out := ""
	for _, p := range pages {
		name := p.Name
		if name == "" {
			name = p.URL
		}
		out += fmt.Sprintf("- %s: %s\n", name, p.URL)
	}
	return out
}
