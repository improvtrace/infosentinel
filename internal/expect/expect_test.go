package expect

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"github.com/tomasgdl/infosentinel/internal/config"
)

// validYAML 是一份可通过 config.Parse 校验的最小配置。
const validYAML = `sources:
  search: {enabled: true}
topics:
  - name: t1
    prompt: "关注 AI"
targets:
  - name: m1
    type: email
    to: a@b.c
tasks:
  - name: j1
    topic: t1
    targets: [m1]
    schedules: ["0 8 * * *"]
`

func TestExtractYAML(t *testing.T) {
	valid := strings.TrimSpace(validYAML) // extractYAML 会去除首尾空白
	cases := []struct {
		name    string
		in      string
		want    string // 期望提取结果
		wantErr bool   // 期望提取结果不可通过校验时,测试再自行断言
	}{
		{
			name: "yaml 围栏",
			in:   "好的,以下是修改后的配置:\n```yaml\n" + validYAML + "\n```\n请确认。",
			want: valid,
		},
		{
			name: "无语言标记围栏",
			in:   "```\n" + validYAML + "\n```",
			want: valid,
		},
		{
			name: "多个围栏优先取可通过校验者",
			in:   "```yaml\nsources: [bad\n```\n中间解释\n```yaml\n" + validYAML + "\n```",
			want: valid,
		},
		{
			name: "无围栏全文即 YAML",
			in:   validYAML,
			want: valid,
		},
		{
			name:    "唯一围栏内容非法时原样返回(交由重试机制)",
			in:      "```yaml\nsources: [bad\n```",
			want:    "sources: [bad",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractYAML(tc.in)
			if got != tc.want {
				t.Errorf("extractYAML = %q,期望 %q", got, tc.want)
			}
			if tc.wantErr {
				if _, err := config.Parse([]byte(got)); err == nil {
					t.Errorf("期望 %q 不可解析,但通过了", got)
				}
			} else if _, err := config.Parse([]byte(got)); err != nil {
				t.Errorf("提取结果应可通过校验: %v", err)
			}
		})
	}
}

func TestDiff(t *testing.T) {
	if d := Diff("a\nb\nc\n", "a\nb\nc\n"); d != "" {
		t.Errorf("无差异应返回空串,得到 %q", d)
	}

	a := "sources:\n  search:\n    enabled: true\ntopics: []\n"
	b := "sources:\n  search:\n    enabled: false\ntopics: []\ntasks: []\n"
	d := Diff(a, b)

	// enabled 行被改("- " 前缀 + 原 4 空格缩进)
	if !strings.Contains(d, "-     enabled: true") || !strings.Contains(d, "+     enabled: false") {
		t.Errorf("差异未包含修改行:\n%s", d)
	}
	// 新增行
	if !strings.Contains(d, "+ tasks: []") {
		t.Errorf("差异未包含新增行:\n%s", d)
	}
	// 未变化行有上下文前缀
	if !strings.Contains(d, "  topics: []") {
		t.Errorf("未变化行缺少上下文标记:\n%s", d)
	}
	// 头部
	if !strings.Contains(d, "--- 当前配置") || !strings.Contains(d, "+++ 修改后") {
		t.Errorf("缺少 diff 头部:\n%s", d)
	}

	// 纯删除:不应出现任何新增行("+ " 前缀)
	d = Diff("a\nb\n", "a\n")
	if !strings.Contains(d, "- b") || strings.Contains(d, "+ b") || strings.Contains(d, "+ a") {
		t.Errorf("纯删除差异不符:\n%s", d)
	}
}

func TestSessionRememberCapped(t *testing.T) {
	s := &Session{}
	for i := 0; i < 20; i++ {
		s.remember("指令", validYAML)
	}
	if len(s.history) != maxHistoryMessages {
		t.Errorf("历史上限应为 %d,得到 %d", maxHistoryMessages, len(s.history))
	}
	if len(s.history)%2 != 0 {
		t.Error("历史应成对(用户+助手)")
	}
	// buildMessages = system + history + 本轮 user
	msgs := s.buildMessages("再加一个源")
	if len(msgs) != 1+maxHistoryMessages+1 {
		t.Errorf("消息数不符: %d", len(msgs))
	}
	if msgs[0].Role != schema.System || msgs[len(msgs)-1].Role != schema.User {
		t.Error("首条应为 system,末条应为 user")
	}
}
