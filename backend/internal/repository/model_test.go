package repository

import (
	"strings"
	"testing"
)

// validConfig 返回一份合法的最小配置,供校验测试增删字段。
func validConfig() *Config {
	return &Config{
		Sources: Sources{Search: SearchConfig{Enabled: true}},
		Topics:  []Topic{{Name: "t1", Prompt: "关注 AI 动态"}},
		Targets: []Target{{Name: "mail", Type: "email", To: "a@b.c"}},
		Tasks:   []Task{{Name: "job1", Topic: "t1", Targets: []string{"mail"}, Schedules: []string{"0 8 * * *"}}},
		LLM:     LLMConfig{BaseURL: "https://api.example.com", Model: "test-model", TimeoutSec: 60, MaxStep: 10},
		Mailer:  MailerConfig{Type: "dryrun"},
	}
}

func TestValidateOK(t *testing.T) {
	if err := validConfig().Finalize(); err != nil {
		t.Fatalf("合法配置不应报错: %v", err)
	}
}

func TestLookupHelpers(t *testing.T) {
	cfg := validConfig()
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
		name   string
		mutate func(*Config)
		want   string // 期望错误信息包含的子串
	}{
		{"无主题", func(c *Config) { c.Topics = nil }, "topics"},
		{"无目标", func(c *Config) { c.Targets = nil }, "targets"},
		{"无任务", func(c *Config) { c.Tasks = nil }, "tasks"},
		{"无数据源", func(c *Config) { c.Sources = Sources{} }, "未启用任何数据源"},
		{"主题引用不存在", func(c *Config) { c.Tasks[0].Topic = "ghost" }, "不存在的 topic"},
		{"目标引用不存在", func(c *Config) { c.Tasks[0].Targets = []string{"ghost"} }, "不存在的 target"},
		{"非法 cron", func(c *Config) { c.Tasks[0].Schedules = []string{"not-a-cron"} }, "不合法"},
		{"主题重名", func(c *Config) {
			c.Topics = append(c.Topics, Topic{Name: "t1", Prompt: "p2"})
		}, "topic 名字重复"},
		{"email 缺 to", func(c *Config) { c.Targets[0].To = "" }, "缺少 to"},
		{"非法通道类型", func(c *Config) { c.Targets[0].Type = "sms" }, "仅支持 email"},
		{"任务无调度", func(c *Config) { c.Tasks[0].Schedules = nil }, "未配置任何 schedules"},
		{"空 prompt 主题", func(c *Config) { c.Topics[0].Prompt = "" }, "prompt 不能为空"},
		{"非法 mailer 类型", func(c *Config) { c.Mailer.Type = "carrier-pigeon" }, "dryrun / smtp"},
		{"smtp 缺 host/from", func(c *Config) { c.Mailer.Type = "smtp" }, "mailer.smtp.host"},
		{"llm 缺 base_url", func(c *Config) { c.LLM.BaseURL = "" }, "llm.base_url"},
		{"llm 缺 model", func(c *Config) { c.LLM.Model = "" }, "llm.model"},
		{"llm 超时非正数", func(c *Config) { c.LLM.TimeoutSec = 0 }, "timeout_sec"},
		{"llm 步数非正数", func(c *Config) { c.LLM.MaxStep = 0 }, "max_step"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validConfig()
			tc.mutate(cfg)
			err := cfg.Finalize()
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
	cfg := &Config{
		Sources: Sources{RSS: RSSConfig{Enabled: true, Feeds: []RSSFeed{{Name: "f1", URL: "https://example.com/rss"}}}},
		Topics:  []Topic{{Name: "t", Prompt: "p"}},
		Targets: []Target{
			{Name: "m1", Type: "email", To: "a@b.c"},
			{Name: "m2", Type: "email", To: "d@e.f"},
		},
		Tasks: []Task{{
			Name:      "j",
			Topic:     "t",
			Targets:   []string{"m1", "m2"},
			Schedules: []string{"0 8 * * *", "30 20 * * *"},
		}},
		LLM:    LLMConfig{BaseURL: "https://api.example.com", Model: "test-model", TimeoutSec: 60, MaxStep: 10},
		Mailer: MailerConfig{Type: "dryrun"},
	}
	if err := cfg.Finalize(); err != nil {
		t.Fatalf("多目标多调度应合法: %v", err)
	}
	if len(cfg.Tasks[0].Schedules) != 2 || len(cfg.Tasks[0].Targets) != 2 {
		t.Errorf("任务字段不符: %+v", cfg.Tasks[0])
	}
}
