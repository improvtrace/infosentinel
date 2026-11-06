package agent

import (
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
	topic := &repository.Topic{
		Name:   "ai-agent",
		Prompt: "关注 AI Agent 框架动态,忽略营销软文。",
	}
	channels := []Channel{
		{Name: "fetch_rss", Hint: "拉取订阅源;可用源:CloudWeGo, HN"},
		{Name: "fetch_webpage", Hint: "抓取网页正文;常读页面清单:\n- Go Releases: https://go.dev/doc/devel/release\n"},
	}
	got := systemPrompt(topic, channels, time.Date(2026, 11, 3, 8, 0, 0, 0, time.UTC))

	for _, want := range []string{
		"个人信息情报员",
		"ai-agent",
		"关注 AI Agent 框架动态,忽略营销软文。", // 主题 prompt 原文注入
		"fetch_rss",
		"CloudWeGo, HN", // 渠道实例清单
		"fetch_webpage",
		"https://go.dev/doc/devel/release",
		"输出格式",
		"2026-11-03",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("系统提示词缺少 %q", want)
		}
	}
}
