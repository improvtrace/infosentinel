package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebpageTool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>Go 1.26 released</title></head>
<body><nav>menu</nav><main><p>Go 1.26 发布,带来多个运行时改进。</p></main></body></html>`)
	}))
	defer srv.Close()

	tk, err := NewWebpageTool()
	if err != nil {
		t.Fatalf("NewWebpageTool: %v", err)
	}

	// 工具元信息由 InferTool 生成
	info, err := tk.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "fetch_webpage" {
		t.Errorf("工具名 = %q", info.Name)
	}

	// 通过 InvokableRun 走完整链路:JSON 参数 → invoke → JSON 结果
	out, err := tk.InvokableRun(context.Background(), fmt.Sprintf(`{"url":%q}`, srv.URL))
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	var res struct {
		URL     string `json:"url"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("解析 %s: %v", out, err)
	}
	if !strings.Contains(res.Content, "Go 1.26 发布") {
		t.Errorf("正文应包含页面文本,得到 %q", res.Content)
	}
	if strings.Contains(res.Content, "<p>") {
		t.Errorf("正文不应包含 HTML 标签: %q", res.Content)
	}

	// 空 URL:结构化错误而非异常
	out, err = tk.InvokableRun(context.Background(), `{"url":""}`)
	if err != nil {
		t.Fatalf("空 URL 调用: %v", err)
	}
	if !strings.Contains(out, "must not be empty") {
		t.Errorf("空 URL 应返回错误说明,得到 %s", out)
	}
}
