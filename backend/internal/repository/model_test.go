package repository

import (
	"strings"
	"testing"
)

// validConfig 返回一份合法的最小配置,供校验测试增删字段。
func validConfig() *Config {
	return &Config{
		Topics:  []Topic{{Name: "t1", Prompt: "关注 AI 动态"}},
		Targets: []Target{{Name: "mail", Type: "email", To: "a@b.c"}},
		Subscriptions: []Subscription{{
			Name:      "订阅A",
			Topics:    []string{"t1"},
			Synthesis: DefaultSynthesis,
			Cron:      "0 8 * * *",
			Targets:   []string{"mail"},
		}},
		LLM:    LLMConfig{BaseURL: "https://api.example.com", Model: "test-model", TimeoutSec: 60, MaxStep: 10},
		Mailer: MailerConfig{Type: MailerTypeInbox},
	}
}

func TestValidateOK(t *testing.T) {
	if err := validConfig().Finalize(); err != nil {
		t.Fatalf("合法配置不应报错: %v", err)
	}
}

func TestValidateAllowsNoSubscription(t *testing.T) {
	cfg := validConfig()
	cfg.Subscriptions = nil // 全部退订是合法状态
	if err := cfg.Finalize(); err != nil {
		t.Fatalf("空订阅列表不应报错: %v", err)
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
	if cfg.SubscriptionByName("订阅A") == nil || cfg.SubscriptionByName("nope") != nil {
		t.Error("SubscriptionByName 行为不符")
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
		{"订阅引用不存在的主题", func(c *Config) { c.Subscriptions[0].Topics = []string{"ghost"} }, "不存在的 topic"},
		{"目标引用不存在", func(c *Config) { c.Subscriptions[0].Targets = []string{"ghost"} }, "不存在的 target"},
		{"非法 cron", func(c *Config) { c.Subscriptions[0].Cron = "not-a-cron" }, "不合法"},
		{"订阅重名", func(c *Config) {
			c.Subscriptions = append(c.Subscriptions, Subscription{Name: "订阅A", Topics: []string{"t1"}, Synthesis: "s", Cron: "0 9 * * *", Targets: []string{"mail"}})
		}, "subscription 名字重复"},
		{"主题重名", func(c *Config) {
			c.Topics = append(c.Topics, Topic{Name: "t1", Prompt: "p2"})
		}, "topic 名字重复"},
		{"email 缺 to", func(c *Config) { c.Targets[0].To = "" }, "缺少 to"},
		{"非法通道类型", func(c *Config) { c.Targets[0].Type = "sms" }, "仅支持 email"},
		{"订阅无主题", func(c *Config) { c.Subscriptions[0].Topics = nil }, "未关联任何主题"},
		{"订阅主题重复", func(c *Config) { c.Subscriptions[0].Topics = []string{"t1", "t1"} }, "topics 中"},
		{"订阅无合成提示词", func(c *Config) { c.Subscriptions[0].Synthesis = "" }, "synthesis 不能为空"},
		{"订阅无目标", func(c *Config) { c.Subscriptions[0].Targets = nil }, "未配置任何 targets"},
		{"订阅引用不存在的空间", func(c *Config) { c.Subscriptions[0].Space = "ghost" }, "不存在的产物空间"},
		{"空 prompt 主题", func(c *Config) { c.Topics[0].Prompt = "" }, "prompt 不能为空"},
		{"非法 mailer 类型", func(c *Config) { c.Mailer.Type = "carrier-pigeon" }, "站内信 / smtp"},
		{"smtp 缺 host/from", func(c *Config) { c.Mailer.Type = MailerTypeSMTP }, "mailer.smtp.host"},
		{"llm 缺 base_url", func(c *Config) { c.LLM.BaseURL = "" }, "llm.base_url"},
		{"llm 缺 model", func(c *Config) { c.LLM.Model = "" }, "llm.model"},
		{"llm 超时非正数", func(c *Config) { c.LLM.TimeoutSec = 0 }, "timeout_sec"},
		{"llm 步数非正数", func(c *Config) { c.LLM.MaxStep = 0 }, "max_step"},
		{"空间重名", func(c *Config) {
			c.Storage.Spaces = []StorageSpace{
				{Name: "sp", Type: StorageTypeLocal},
				{Name: "sp", Type: StorageTypeLocal},
			}
		}, "storage.space 名字重复"},
		{"空间非法类型", func(c *Config) {
			c.Storage.Spaces = []StorageSpace{{Name: "sp", Type: "nas"}}
		}, "仅支持 local"},
		{"空间 sftp 缺 host", func(c *Config) {
			c.Storage.Spaces = []StorageSpace{{Name: "sp", Type: StorageTypeSFTP}}
		}, "sftp.host"},
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

func TestMultiTopicsSharedAcrossSubscriptions(t *testing.T) {
	// 同一主题可被多个订阅引用(多对多);订阅可关联多个主题
	cfg := validConfig()
	cfg.Topics = append(cfg.Topics, Topic{Name: "t2", Prompt: "p2"})
	cfg.Subscriptions = append(cfg.Subscriptions, Subscription{
		Name: "订阅B", Topics: []string{"t1", "t2"}, Synthesis: DefaultSynthesis,
		Cron: "0 9 * * *", Targets: []string{"mail"},
	})
	if err := cfg.Finalize(); err != nil {
		t.Fatalf("多对多订阅应合法: %v", err)
	}
}

func TestMultiTargetsValid(t *testing.T) {
	cfg := &Config{
		Topics:  []Topic{{Name: "t", Prompt: "p"}},
		Targets: []Target{{Name: "m1", Type: "email", To: "a@b.c"}, {Name: "m2", Type: "email", To: "d@e.f"}},
		Subscriptions: []Subscription{{
			Name: "订阅A", Topics: []string{"t"}, Synthesis: DefaultSynthesis,
			Cron: "0 8 * * *", Targets: []string{"m1", "m2"},
		}},
		LLM:    LLMConfig{BaseURL: "https://api.example.com", Model: "test-model", TimeoutSec: 60, MaxStep: 10},
		Mailer: MailerConfig{Type: MailerTypeInbox},
	}
	if err := cfg.Finalize(); err != nil {
		t.Fatalf("多目标订阅应合法: %v", err)
	}
	if len(cfg.Subscriptions[0].Targets) != 2 {
		t.Errorf("订阅字段不符: %+v", cfg.Subscriptions[0])
	}
}

func TestFinalizeNormalizesLegacyMailerType(t *testing.T) {
	cfg := validConfig()
	cfg.Mailer.Type = "dryrun" // 旧库中的历史值
	if err := cfg.Finalize(); err != nil {
		t.Fatalf("旧值归一化不应报错: %v", err)
	}
	if cfg.Mailer.Type != MailerTypeInbox {
		t.Errorf("mailer.type = %q,期望归一化为 %q", cfg.Mailer.Type, MailerTypeInbox)
	}
}

func TestFinalizeBackfillsDefaultSpace(t *testing.T) {
	cfg := validConfig()
	cfg.Storage = StorageConfig{} // 未配置任何空间
	if err := cfg.Finalize(); err != nil {
		t.Fatalf("空空间配置不应报错: %v", err)
	}
	if len(cfg.Storage.Spaces) != 1 || cfg.Storage.Spaces[0].Name != DefaultSpaceName || cfg.Storage.Spaces[0].Type != StorageTypeLocal {
		t.Errorf("应回填一个本地默认空间,得到 %+v", cfg.Storage.Spaces)
	}
}

func TestFinalizeWrapsLegacyFlatStorage(t *testing.T) {
	// 旧库单空间扁平 JSON 反序列化后的形态
	cfg := validConfig()
	cfg.Storage = StorageConfig{Type: StorageTypeLocal, Path: "reports", PeriodDays: 7, MaxDirs: 10}
	if err := cfg.Finalize(); err != nil {
		t.Fatalf("旧扁平配置不应报错: %v", err)
	}
	if len(cfg.Storage.Spaces) != 1 {
		t.Fatalf("应包装为单个空间,得到 %+v", cfg.Storage.Spaces)
	}
	sp := cfg.Storage.Spaces[0]
	if sp.Name != DefaultSpaceName || sp.Type != StorageTypeLocal || sp.Path != "reports" || sp.PeriodDays != 7 || sp.MaxDirs != 10 {
		t.Errorf("包装后的空间字段不符: %+v", sp)
	}
	// 兼容扁平字段应被清空,持久化时只保留 Spaces
	if cfg.Storage.Type != "" || cfg.Storage.Path != "" {
		t.Errorf("扁平字段应清空,得到 %+v", cfg.Storage)
	}
}

func TestDefaultSynthesisWithin200Chars(t *testing.T) {
	if n := len([]rune(DefaultSynthesis)); n > 200 {
		t.Errorf("默认合成提示词 %d 字,超出 200 字上限", n)
	}
}
