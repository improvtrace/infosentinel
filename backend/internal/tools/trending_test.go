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
)

// newHNToolForTest 构建指向 mock API 的 hnTool。
func newHNToolForTest(t *testing.T, baseURL string) *hnTool {
	t.Helper()
	store, err := NewSeenStore("", 7)
	if err != nil {
		t.Fatalf("NewSeenStore: %v", err)
	}
	return &hnTool{
		client:    &http.Client{Timeout: 5 * time.Second},
		topN:      5,
		minPoints: 50,
		window:    48 * time.Hour,
		store:     store,
		nowFn:     time.Now,
	}
}

// hackHNAPI 将包级 API 常量替换为 mock 地址(hnTool.getJSON 与 fetchStories 均引用它)。
func hackHNAPI(t *testing.T, baseURL string) {
	t.Helper()
	old := hackerNewsAPI
	hackerNewsAPI = baseURL
	t.Cleanup(func() { hackerNewsAPI = old })
}

func TestHackerNewsInvoke(t *testing.T) {
	now := time.Now()
	mux := http.NewServeMux()
	mux.HandleFunc("/v0/topstories.json", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "[101,102,103]")
	})
	stories := map[string]hnStory{
		"101": {ID: 101, Type: "story", Title: "Eino v1 released", URL: "https://example.com/eino", Score: 300, By: "alice", Time: now.Add(-3 * time.Hour).Unix(), Descendants: 42},
		"102": {ID: 102, Type: "story", Title: "low score, filtered", URL: "https://example.com/low", Score: 10, By: "bob", Time: now.Add(-3 * time.Hour).Unix()},
		"103": {ID: 103, Type: "story", Title: "Ask HN: no link", URL: "", Score: 120, By: "carol", Time: now.Add(-5 * time.Hour).Unix(), Descendants: 7},
	}
	mux.HandleFunc("/v0/item/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v0/item/"), ".json")
		s, ok := stories[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(s)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	hackHNAPI(t, srv.URL+"/v0")

	tool := newHNToolForTest(t, srv.URL+"/v0")
	out, err := tool.invoke(context.Background(), struct{}{})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}

	var res struct {
		Items []hnItem `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("解析 %s: %v", out, err)
	}
	if len(res.Items) != 2 {
		t.Fatalf("期望 2 条(低分被过滤),得到 %d: %+v", len(res.Items), res.Items)
	}
	byTitle := map[string]hnItem{}
	for _, it := range res.Items {
		byTitle[it.Title] = it
	}
	if it, ok := byTitle["Eino v1 released"]; !ok || it.Score != 300 {
		t.Errorf("高分条目缺失或分数错误: %+v", it)
	}
	if it, ok := byTitle["Ask HN: no link"]; !ok || !strings.HasPrefix(it.URL, "https://news.ycombinator.com/item") {
		t.Errorf("无外链条目应指向 HN 页面: %+v", it)
	}
}

// trendingHTML 模拟 GitHub Trending 页面结构(仅保留工具解析所需的关键元素)。
func trendingHTML() string {
	return `<!DOCTYPE html><html><body><main>
<article class="Box">
  <h2 class="h3 lh-condensed"><a href="/cloudwego/eino">cloudwego / <b>eino</b></a></h2>
  <p class="col-9 color-fg-muted my-1 pr-4">The ultimate LLM app framework in Go</p>
  <div><span itemprop="programmingLanguage">Go</span></div>
  <span class="d-inline-block float-sm-right">1,234 stars today</span>
</article>
<article class="Box">
  <h2 class="h3 lh-condensed"><a href="/golang/go">golang / <b>go</b></a></h2>
  <p class="col-9 color-fg-muted my-1 pr-4">The Go programming language</p>
  <div><span itemprop="programmingLanguage">Go</span></div>
  <span class="d-inline-block float-sm-right">567 stars today</span>
</article>
</main></body></html>`
}

func TestGitHubTrendingInvoke(t *testing.T) {
	var gotLang string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLang = r.URL.Path // /trending 或 /trending/go
		if !strings.Contains(r.URL.RawQuery, "since=daily") {
			t.Errorf("应携带 since=daily,得到 %q", r.URL.RawQuery)
		}
		fmt.Fprint(w, trendingHTML())
	}))
	defer srv.Close()

	store, _ := NewSeenStore("", 7)
	tool := &trendingTool{
		client:  &http.Client{Timeout: 5 * time.Second},
		baseURL: srv.URL + "/trending",
		defLang: "go",
		topN:    10,
		store:   store,
		nowFn:   time.Now,
	}

	out, err := tool.invoke(context.Background(), &trendingToolParams{})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if want := "/trending/go"; gotLang != want {
		t.Errorf("默认语言应拼进路径: got %q want %q", gotLang, want)
	}

	var res struct {
		Repos []repoItem `json:"repos"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("解析 %s: %v", out, err)
	}
	if len(res.Repos) != 2 {
		t.Fatalf("期望 2 个仓库,得到 %d", len(res.Repos))
	}
	first := res.Repos[0]
	if first.Repo != "cloudwego/eino" || first.Language != "Go" {
		t.Errorf("首个仓库解析错误: %+v", first)
	}
	if !strings.Contains(first.StarsToday, "stars today") {
		t.Errorf("StarsToday 解析错误: %q", first.StarsToday)
	}
	if first.URL != "https://github.com/cloudwego/eino" {
		t.Errorf("URL = %q", first.URL)
	}

	// 第二次:去重后为空
	out2, _ := tool.invoke(context.Background(), &trendingToolParams{Language: "all"})
	if !strings.Contains(out2, "no new trending repos") {
		t.Errorf("第二次应因去重返回空,得到 %s", out2)
	}
}
