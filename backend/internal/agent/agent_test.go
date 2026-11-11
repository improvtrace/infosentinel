package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
)

func TestSplitSubjectBody(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		subject string
		body    string
	}{
		{
			name:    "标准输出",
			in:      "今日 AI 三件事\n\n## 模型\n- GPT-5 发布 [链接](https://x.com)",
			subject: "今日 AI 三件事",
			body:    "## 模型\n- GPT-5 发布 [链接](https://x.com)",
		},
		{
			name:    "标题带井号",
			in:      "# 今日看点\n正文",
			subject: "今日看点",
			body:    "正文",
		},
		{
			name:    "只有标题",
			in:      "只有标题",
			subject: "只有标题",
			body:    "",
		},
		{
			name:    "空输出",
			in:      "",
			subject: "今日资讯",
			body:    "本轮未产出内容。",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, b := splitSubjectBody(tc.in)
			if s != tc.subject {
				t.Errorf("subject = %q,期望 %q", s, tc.subject)
			}
			if b != tc.body {
				t.Errorf("body = %q,期望 %q", b, tc.body)
			}
		})
	}
}

func TestSystemPrompt(t *testing.T) {
	topic := repository.Topic{
		Name:   "ai-agent",
		Prompt: "关注 AI Agent 框架动态,忽略营销软文。",
	}
	got := systemPrompt(&Group{Name: topic.Name, Topics: []repository.Topic{topic}}, time.Date(2026, 11, 3, 8, 0, 0, 0, time.UTC))

	for _, want := range []string{
		"个人信息情报员",
		"ai-agent",
		"关注 AI Agent 框架动态,忽略营销软文。", // 主题 prompt 原文注入
		"fetch_rss",
		"fetch_webpage",
		"工作方法",
		"链接必须原样取自工具返回结果", // 链接真实性约束
		"输出格式",
		"2026-11-03",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("系统提示词缺少 %q", want)
		}
	}
	// 单主题组不出现多主题组专用段落
	for _, unwanted := range []string{"本组共同意图", "### 主题:"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("单主题组系统提示词不应包含 %q", unwanted)
		}
	}
}

func TestSystemPromptGroup(t *testing.T) {
	topics := []repository.Topic{
		{Name: "world-situation", Prompt: "关注全球局势与金融。"},
		{Name: "policy", Prompt: "关注各国政策变更与解读。"},
	}
	got := systemPrompt(&Group{Name: "世界与政策", Goal: "全球动态与政策联动观察", Topics: topics},
		time.Date(2026, 11, 11, 8, 0, 0, 0, time.UTC))

	for _, want := range []string{
		"本组共同意图",                 // 多主题组的组意图段
		"全球动态与政策联动观察",            // 组 goal 注入
		"### 主题:world-situation", // 主题逐一分列
		"关注全球局势与金融。",             // 各主题 prompt 原文注入
		"### 主题:policy",
		"关注各国政策变更与解读。",
		"同时覆盖全部主题", // 多主题合并采集要求
	} {
		if !strings.Contains(got, want) {
			t.Errorf("多主题组系统提示词缺少 %q", want)
		}
	}
}

func TestKickoffMessage(t *testing.T) {
	now := time.Date(2026, 11, 11, 8, 0, 0, 0, time.UTC)

	single := kickoffMessage(&Group{
		Name:   "ai-agent",
		Topics: []repository.Topic{{Name: "ai-agent", Prompt: "p"}},
	}, now)
	if !strings.Contains(single, "开始执行主题「ai-agent」") || strings.Contains(single, "覆盖主题") {
		t.Errorf("单主题组启动消息口径不符: %q", single)
	}

	multi := kickoffMessage(&Group{
		Name: "世界与政策",
		Topics: []repository.Topic{
			{Name: "world-situation", Prompt: "p"},
			{Name: "policy", Prompt: "p"},
		},
	}, now)
	if !strings.Contains(multi, "开始执行分组「世界与政策」") ||
		!strings.Contains(multi, "覆盖主题:world-situation、policy") {
		t.Errorf("多主题组启动消息应列出全部主题: %q", multi)
	}
}

func TestSynthesisSystemPrompt(t *testing.T) {
	got := synthesisSystemPrompt("按重要性排序合并", time.Date(2026, 11, 11, 8, 0, 0, 0, time.UTC))
	for _, want := range []string{
		"资讯编辑",
		"按重要性排序合并", // 订阅的合成提示词原文注入
		"输出格式",
		"HTML 正文片段", // HTML 输出契约
		"<script>",  // 危险标签禁令
		"代码围栏",      // 禁止围栏包裹全文
		"2026-11-11",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("合成系统提示词缺少 %q", want)
		}
	}
}

func TestRenderParts(t *testing.T) {
	parts := []*Digest{
		{Subject: "AI 周报", Body: "## 模型\n- A 发布"},
		{Subject: "Go 动态", Body: "## Go\n- B 发布"},
	}
	got := renderParts(parts)
	for _, want := range []string{
		"2 份主题报告",
		"AI 周报", // 每份报告的标题
		"## 模型", // 每份报告的正文
		"Go 动态",
		"--- 第 1 份报告 ---",
		"--- 第 2 份报告 ---",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("合成材料缺少 %q", want)
		}
	}
}

func TestSynthesizeSinglePartShortCircuits(t *testing.T) {
	// 单主题无需合并:直接返回该份报告(cfg 为零值,不应触发任何模型调用)
	part := &Digest{Subject: "唯一主题", Body: "正文"}
	got, err := Synthesize(context.Background(), &repository.Config{}, repository.DefaultSynthesis, "", []*Digest{part})
	if err != nil {
		t.Fatalf("单份报告不应报错: %v", err)
	}
	if got != part {
		t.Errorf("单份报告应原样返回,得到 %+v", got)
	}
	if _, err := Synthesize(context.Background(), &repository.Config{}, "", "", nil); err == nil {
		t.Error("空报告列表应报错")
	}
}

func TestPlanGroupsSingleTopicShortCircuits(t *testing.T) {
	// 单主题订阅直接成组,不触发任何模型调用(cfg 为零值)
	topics := []repository.Topic{{Name: "ai-agent", Prompt: "关注 AI"}}
	groups, err := PlanGroups(context.Background(), &repository.Config{}, "", topics)
	if err != nil {
		t.Fatalf("单主题分组不应报错: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != "ai-agent" || len(groups[0].Topics) != 1 {
		t.Errorf("单主题应自成一组,得到 %+v", groups)
	}
	if _, err := PlanGroups(context.Background(), &repository.Config{}, "", nil); err == nil {
		t.Error("空主题列表应报错")
	}
}

func TestParseIntents(t *testing.T) {
	topics := []repository.Topic{
		{Name: "world-situation", Prompt: "p1"},
		{Name: "policy", Prompt: "p2"},
		{Name: "security", Prompt: "p3"},
	}

	t.Run("合法分组", func(t *testing.T) {
		in := `{"groups":[
			{"name":"世界与政策","goal":"全球动态联动","topics":["world-situation","policy"]},
			{"name":"安全","goal":"威胁情报","topics":["security"]}
		]}`
		groups, err := parseIntents(in, topics)
		if err != nil {
			t.Fatalf("合法分组不应报错: %v", err)
		}
		if len(groups) != 2 {
			t.Fatalf("应得到 2 组,得到 %d 组", len(groups))
		}
		if groups[0].Name != "世界与政策" || groups[0].Goal != "全球动态联动" || len(groups[0].Topics) != 2 {
			t.Errorf("第一组内容不符: %+v", groups[0])
		}
		if len(groups[1].Topics) != 1 || groups[1].Topics[0].Name != "security" {
			t.Errorf("第二组内容不符: %+v", groups[1])
		}
	})

	t.Run("代码围栏包裹", func(t *testing.T) {
		in := "```json\n{\"groups\":[{\"name\":\"g\",\"goal\":\"\",\"topics\":[\"world-situation\",\"policy\",\"security\"]}]}\n```"
		groups, err := parseIntents(in, topics)
		if err != nil {
			t.Fatalf("围栏包裹的 JSON 应可解析: %v", err)
		}
		if len(groups) != 1 || len(groups[0].Topics) != 3 {
			t.Errorf("围栏解析结果不符: %+v", groups)
		}
	})

	t.Run("非法 JSON", func(t *testing.T) {
		if _, err := parseIntents("我不是 JSON", topics); err == nil {
			t.Error("非法 JSON 应报错")
		}
	})

	for _, tc := range []struct {
		name string
		in   string
	}{
		{"空分组", `{"groups":[]}`},
		{"组名为空", `{"groups":[{"name":"","goal":"","topics":["world-situation"]}]}`},
		{"组名为空且组非空但无主题", `{"groups":[{"name":"g","goal":"","topics":[]}]}`},
		{"未知主题", `{"groups":[{"name":"g","goal":"","topics":["no-such"]}]}`},
		{"主题重复分配", `{"groups":[
			{"name":"g1","goal":"","topics":["world-situation"]},
			{"name":"g2","goal":"","topics":["world-situation"]}]}`},
		{"组名重复", `{"groups":[
			{"name":"g","goal":"","topics":["world-situation"]},
			{"name":"g","goal":"","topics":["policy"]}]}`},
		{"遗漏主题", `{"groups":[{"name":"g","goal":"","topics":["world-situation","policy"]}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseIntents(tc.in, topics); err == nil {
				t.Errorf("%s 应校验失败", tc.name)
			}
		})
	}
}
