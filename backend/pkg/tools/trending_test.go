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
func newHNToolForTest(t *testing.T) *hnTool {
	t.Helper()
	store, err := NewSeenStore("", DedupDays)
	if err != nil {
		t.Fatalf("NewSeenStore: %v", err)
	}
	return &hnTool{
		client: &http.Client{Timeout: 5 * time.Second},
		window: WindowHours * time.Hour,
		store:  store,
		nowFn:  time.Now,
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

	tool := newHNToolForTest(t)
	out, err := tool.invoke(context.Background(), &hnToolParams{})
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
		t.Fatalf("期望 2 条(低分被默认阈值过滤),得到 %d: %+v", len(res.Items), res.Items)
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

// TestHackerNewsParams 验证 min_points / top_n 参数由调用方(主题指令)控制筛选。
func TestHackerNewsParams(t *testing.T) {
	now := time.Now()
	mux := http.NewServeMux()
	mux.HandleFunc("/v0/topstories.json", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "[101,102,103]")
	})
	stories := map[string]hnStory{
		"101": {ID: 101, Type: "story", Title: "Eino v1 released", URL: "https://example.com/eino", Score: 300, By: "alice", Time: now.Add(-3 * time.Hour).Unix()},
		"102": {ID: 102, Type: "story", Title: "mid score", URL: "https://example.com/mid", Score: 120, By: "bob", Time: now.Add(-3 * time.Hour).Unix()},
		"103": {ID: 103, Type: "story", Title: "low score", URL: "https://example.com/low", Score: 10, By: "carol", Time: now.Add(-3 * time.Hour).Unix()},
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

	count := func(t *testing.T, params *hnToolParams) []hnItem {
		t.Helper()
		out, err := newHNToolForTest(t).invoke(context.Background(), params)
		if err != nil {
			t.Fatalf("invoke: %v", err)
		}
		var res struct {
			Items []hnItem `json:"items"`
		}
		if err := json.Unmarshal([]byte(out), &res); err != nil {
			t.Fatalf("解析 %s: %v", out, err)
		}
		return res.Items
	}

	// min_points=200:只留 300 分一条
	items := count(t, &hnToolParams{MinPoints: 200})
	if len(items) != 1 || items[0].Title != "Eino v1 released" {
		t.Fatalf("min_points=200 应只留高分一条,得到 %+v", items)
	}
	// top_n=1:默认阈值下最多一条
	if items := count(t, &hnToolParams{TopN: 1}); len(items) != 1 {
		t.Fatalf("top_n=1 应只返回一条,得到 %+v", items)
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
	var gotLangs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLangs = append(gotLangs, r.URL.Path) // /trending 或 /trending/<lang>
		if !strings.Contains(r.URL.RawQuery, "since=daily") {
			t.Errorf("应携带 since=daily,得到 %q", r.URL.RawQuery)
		}
		fmt.Fprint(w, trendingHTML())
	}))
	defer srv.Close()

	newTool := func(t *testing.T) *trendingTool {
		t.Helper()
		store, err := NewSeenStore("", DedupDays)
		if err != nil {
			t.Fatalf("NewSeenStore: %v", err)
		}
		return &trendingTool{
			client:  &http.Client{Timeout: 5 * time.Second},
			baseURL: srv.URL + "/trending",
			store:   store,
			nowFn:   time.Now,
		}
	}

	// 传语言:拼进路径
	tool := newTool(t)
	out, err := tool.invoke(context.Background(), &trendingToolParams{Language: "go"})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if want := "/trending/go"; gotLangs[0] != want {
		t.Errorf("语言应拼进路径: got %q want %q", gotLangs[0], want)
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

	// 第二次(同一工具):去重后为空
	out2, _ := tool.invoke(context.Background(), &trendingToolParams{Language: "all"})
	if !strings.Contains(out2, "no new trending repos") {
		t.Errorf("第二次应因去重返回空,得到 %s", out2)
	}

	// 不传语言:不拼语言路径(不过滤)
	if _, err := newTool(t).invoke(context.Background(), &trendingToolParams{}); err != nil {
		t.Fatalf("invoke(无语言): %v", err)
	}
	if want := "/trending"; gotLangs[len(gotLangs)-1] != want {
		t.Errorf("无语言参数不应拼接路径: got %q want %q", gotLangs[len(gotLangs)-1], want)
	}
}
