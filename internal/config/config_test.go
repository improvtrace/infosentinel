package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTmp 把内容写入临时配置文件,返回路径。
func writeTmp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写临时配置: %v", err)
	}
	return path
}

// minimalYAML 是一份合法的最小配置,供校验测试增删字段。
const minimalYAML = `
sources:
  search: {enabled: true}
topics:
  - name: t1
    prompt: "关注 AI 动态"
targets:
  - name: mail
    type: email
    to: "a@b.c"
tasks:
  - name: job1
    topic: t1
    targets: [mail]
    schedules: ["0 8 * * *"]
`

func TestLoadDefaults(t *testing.T) {
	path := writeTmp(t, minimalYAML)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// 采集默认值
	if cfg.Sources.Collect.WindowHours != 48 || cfg.Sources.Collect.StateDir != "out/state" {
		t.Errorf("collect 默认值不符: %+v", cfg.Sources.Collect)
	}
	// LLM 默认值
	if cfg.LLM.BaseURL != "https://api.deepseek.com" || cfg.LLM.Model != "deepseek-chat" {
		t.Errorf("LLM 默认值不符: %+v", cfg.LLM)
	}
	if cfg.LLM.APIKeyEnv != "DEEPSEEK_API_KEY" {
		t.Errorf("APIKeyEnv 默认值 = %q", cfg.LLM.APIKeyEnv)
	}
	// 目标默认值
	if cfg.Targets[0].SubjectPrefix != "[InfoSentinel]" {
		t.Errorf("SubjectPrefix 默认值 = %q", cfg.Targets[0].SubjectPrefix)
	}
	if cfg.Mailer.Type != "dryrun" || cfg.Mailer.OutputDir != "out" {
		t.Errorf("mailer 默认值不符: %+v", cfg.Mailer)
	}
}

func TestLookupHelpers(t *testing.T) {
	cfg, err := Load(writeTmp(t, minimalYAML))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TopicByName("t1") == nil || cfg.TopicByName("nope") != nil {
		t.Error("TopicByName 行为不符")
	}
	if cfg.TargetByName("mail") == nil || cfg.TargetByName("nope") != nil {
		t.Error("TargetByName 行为不符")
	}
	if cfg.TaskByName("job1") == nil || cfg.TaskByName("nope") != nil {
		t.Error("TaskByName 行为不符")
	}
}

func TestValidateErrors(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string // 期望错误信息包含的子串
	}{
		{"无主题", `
targets: [{name: m, type: email, to: "a@b.c"}]
tasks: [{name: j, topic: t, targets: [m], schedules: ["0 8 * * *"]}]
`, "topics"},
		{"无目标", `
topics: [{name: t, prompt: p}]
tasks: [{name: j, topic: t, targets: [m], schedules: ["0 8 * * *"]}]
`, "targets"},
		{"无任务", `
topics: [{name: t, prompt: p}]
targets: [{name: m, type: email, to: "a@b.c"}]
`, "tasks"},
		{"无数据源", `
topics: [{name: t, prompt: p}]
targets: [{name: m, type: email, to: "a@b.c"}]
tasks: [{name: j, topic: t, targets: [m], schedules: ["0 8 * * *"]}]
`, "未启用任何数据源"},
		{"主题引用不存在", `
sources: {search: {enabled: true}}
topics: [{name: t, prompt: p}]
targets: [{name: m, type: email, to: "a@b.c"}]
tasks: [{name: j, topic: ghost, targets: [m], schedules: ["0 8 * * *"]}]
`, "不存在的 topic"},
		{"目标引用不存在", `
sources: {search: {enabled: true}}
topics: [{name: t, prompt: p}]
targets: [{name: m, type: email, to: "a@b.c"}]
tasks: [{name: j, topic: t, targets: [ghost], schedules: ["0 8 * * *"]}]
`, "不存在的 target"},
		{"非法 cron", `
sources: {search: {enabled: true}}
topics: [{name: t, prompt: p}]
targets: [{name: m, type: email, to: "a@b.c"}]
tasks: [{name: j, topic: t, targets: [m], schedules: ["not-a-cron"]}]
`, "不合法"},
		{"主题重名", `
sources: {search: {enabled: true}}
topics:
  - {name: t, prompt: p1}
  - {name: t, prompt: p2}
targets: [{name: m, type: email, to: "a@b.c"}]
tasks: [{name: j, topic: t, targets: [m], schedules: ["0 8 * * *"]}]
`, "topic 名字重复"},
		{"email 缺 to", `
sources: {search: {enabled: true}}
topics: [{name: t, prompt: p}]
targets: [{name: m, type: email}]
tasks: [{name: j, topic: t, targets: [m], schedules: ["0 8 * * *"]}]
`, "缺少 to"},
		{"非法通道类型", `
sources: {search: {enabled: true}}
topics: [{name: t, prompt: p}]
targets: [{name: m, type: sms, to: "123"}]
tasks: [{name: j, topic: t, targets: [m], schedules: ["0 8 * * *"]}]
`, "仅支持 email"},
		{"任务无调度", `
sources: {search: {enabled: true}}
topics: [{name: t, prompt: p}]
targets: [{name: m, type: email, to: "a@b.c"}]
tasks: [{name: j, topic: t, targets: [m]}]
`, "未配置任何 schedules"},
		{"空 prompt 主题", `
sources: {search: {enabled: true}}
topics: [{name: t}]
targets: [{name: m, type: email, to: "a@b.c"}]
tasks: [{name: j, topic: t, targets: [m], schedules: ["0 8 * * *"]}]
`, "prompt 不能为空"},
		{"非法 mailer 类型", minimalYAML + `
mailer: {type: "carrier-pigeon"}
`, "dryrun / smtp"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(writeTmp(t, tc.yaml))
			if err == nil {
				t.Fatalf("期望校验失败,但通过了")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("错误信息 %q 未包含 %q", err.Error(), tc.want)
			}
		})
	}
}

func TestMultiScheduleAndTargetsValid(t *testing.T) {
	yaml := `
sources:
  rss:
    enabled: true
    feeds: [{name: f1, url: "https://example.com/rss"}]
topics: [{name: t, prompt: p}]
targets:
  - {name: m1, type: email, to: "a@b.c"}
  - {name: m2, type: email, to: "d@e.f"}
tasks:
  - name: j
    topic: t
    targets: [m1, m2]
    schedules: ["0 8 * * *", "30 20 * * *"]
`
	cfg, err := Load(writeTmp(t, yaml))
	if err != nil {
		t.Fatalf("多目标多调度应合法: %v", err)
	}
	if len(cfg.Tasks[0].Schedules) != 2 || len(cfg.Tasks[0].Targets) != 2 {
		t.Errorf("任务字段解析不符: %+v", cfg.Tasks[0])
	}
}
