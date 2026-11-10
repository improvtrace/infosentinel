package migrate

import (
	"testing"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
)

// TestBuiltins 校验内置默认配置:完整合法,且运行时产物路径统一收在 .tests/ 下。
func TestBuiltins(t *testing.T) {
	cfg, err := Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}

	// 运行时产物路径约定(相对工作目录,产物空间/去重状态的根路径由引导配置决定)
	if cfg.Storage.Type != repository.StorageTypeLocal || cfg.Storage.PeriodDays != 1 || cfg.Storage.MaxDirs != 30 {
		t.Errorf("storage 默认配置不正确: %+v", cfg.Storage)
	}

	// 内置主题/推送目标/订阅
	if cfg.TopicByName("ai-agent") == nil {
		t.Error("内置主题 ai-agent 缺失")
	}
	if cfg.TargetByName("mailbox") == nil {
		t.Error("内置推送目标 mailbox 缺失")
	}
	sub := cfg.SubscriptionByTopic("ai-agent")
	if sub == nil {
		t.Error("内置订阅(主题 ai-agent)缺失")
	} else {
		if sub.Cron != "0 8 * * *" {
			t.Errorf("内置订阅 cron = %q", sub.Cron)
		}
		if len(sub.Targets) != 1 || sub.Targets[0] != "mailbox" {
			t.Errorf("内置订阅 targets = %v", sub.Targets)
		}
	}

	// 模型与 Web 服务
	if cfg.LLM.BaseURL == "" || cfg.LLM.Model == "" {
		t.Errorf("LLM 配置不完整: %+v", cfg.LLM)
	}
	if !cfg.Server.IsEnabled() || cfg.Server.Addr == "" {
		t.Errorf("server 配置不完整: %+v", cfg.Server)
	}
	if cfg.Mailer.Type != repository.MailerTypeInbox {
		t.Errorf("mailer.type = %q,期望 %q", cfg.Mailer.Type, repository.MailerTypeInbox)
	}

	// 二次调用返回独立副本,外部修改不影响后续取值
	cfg.Topics[0].Name = "mutated"
	again, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	if again.Topics[0].Name == "mutated" {
		t.Error("Builtins 应返回独立副本")
	}
}
