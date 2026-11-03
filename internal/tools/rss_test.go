package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tomasgdl/infosentinel/internal/config"
)

// rssFixture 返回一份含新/旧两条目与 HTML 描述的 RSS。
func rssFixture(now time.Time) string {
	fresh := now.Add(-2 * time.Hour).Format(time.RFC1123Z)
	stale := now.Add(-72 * time.Hour).Format(time.RFC1123Z)
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Test Feed</title>
<item>
  <title>新鲜事:Eino 发布新版</title>
  <link>https://example.com/eino-new</link>
  <pubDate>%s</pubDate>
  <description>&lt;p&gt;Eino v0.9.21 发布,包含&lt;b&gt;重要修复&lt;/b&gt;。&lt;/p&gt;</description>
</item>
<item>
  <title>旧消息:应被时间窗过滤</title>
  <link>https://example.com/stale</link>
  <pubDate>%s</pubDate>
  <description>三天前的旧内容</description>
</item>
</channel></rss>`, fresh, stale)
}

func newRSSToolForTest(t *testing.T, feedURL string) *rssTool {
	t.Helper()
	store, err := NewSeenStore("", 7)
	if err != nil {
		t.Fatalf("NewSeenStore: %v", err)
	}
	return &rssTool{
		feeds:    []config.RSSFeed{{Name: "测试源", URL: feedURL}},
		client:   &http.Client{Timeout: 5 * time.Second},
		window:   48 * time.Hour,
		maxItems: 15,
		store:    store,
		nowFn:    time.Now,
	}
}

func TestRSSInvoke(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, rssFixture(time.Now()))
	}))
	defer srv.Close()

	tool := newRSSToolForTest(t, srv.URL)

	out, err := tool.invoke(context.Background(), &rssToolParams{})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	var res struct {
		Items []RSSItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("解析结果 %q: %v", out, err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("应只返回 1 条新条目(旧条目被时间窗过滤),得到 %d: %+v", len(res.Items), res.Items)
	}
	it := res.Items[0]
	if it.Title != "新鲜事:Eino 发布新版" {
		t.Errorf("Title = %q", it.Title)
	}
	if it.Summary != "Eino v0.9.21 发布,包含重要修复。" {
		t.Errorf("Summary 应去除 HTML 标签,得到 %q", it.Summary)
	}
	if it.Published == "" {
		t.Error("Published 不应为空")
	}

	// 第二次调用:去重后应为空,并带 note 说明
	out2, err := tool.invoke(context.Background(), &rssToolParams{})
	if err != nil {
		t.Fatalf("invoke(第二次): %v", err)
	}
	if !strings.Contains(out2, "no new articles") {
		t.Errorf("第二次调用应因去重返回空列表,得到 %s", out2)
	}
}

func TestRSSUnknownFeed(t *testing.T) {
	tool := newRSSToolForTest(t, "http://127.0.0.1:1/feed")
	out, err := tool.invoke(context.Background(), &rssToolParams{Feed: "不存在的源"})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("未知源应返回错误说明,得到 %s", out)
	}
}

func TestRSSFeedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tool := newRSSToolForTest(t, srv.URL)
	out, err := tool.invoke(context.Background(), &rssToolParams{})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !strings.Contains(out, "feed error") {
		t.Errorf("源失败应以 feed error 形式回传,得到 %s", out)
	}
}
