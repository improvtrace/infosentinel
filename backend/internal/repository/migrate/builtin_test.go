package migrate

import "testing"

// TestBuiltins 校验内置默认配置:完整合法,且运行时产物路径统一收在 .tests/ 下。
func TestBuiltins(t *testing.T) {
	cfg, err := Builtins()
	if err != nil {
		t.Fatalf("Builtins: %v", err)
	}

	// 运行时产物路径约定(相对工作目录)
	if cfg.Sources.Collect.StateDir != ".tests/data/state" {
		t.Errorf("collect.state_dir = %q,期望 .tests/data/state", cfg.Sources.Collect.StateDir)
	}
	if cfg.Mailer.OutputDir != ".tests/output" {
		t.Errorf("mailer.output_dir = %q,期望 .tests/output", cfg.Mailer.OutputDir)
	}

	// 内置主题/推送目标/任务
	if cfg.TopicByName("ai-agent") == nil {
		t.Error("内置主题 ai-agent 缺失")
	}
	if cfg.TargetByName("mailbox") == nil {
		t.Error("内置推送目标 mailbox 缺失")
	}
	if cfg.TaskByName("ai-agent-daily") == nil {
		t.Error("内置任务 ai-agent-daily 缺失")
	}

	// 模型与 Web 服务
	if cfg.LLM.BaseURL == "" || cfg.LLM.Model == "" || cfg.LLM.APIKeyEnv == "" {
		t.Errorf("LLM 配置不完整: %+v", cfg.LLM)
	}
	if !cfg.Server.IsEnabled() || cfg.Server.Addr == "" {
		t.Errorf("server 配置不完整: %+v", cfg.Server)
	}
	if cfg.Mailer.Type != "dryrun" {
		t.Errorf("mailer.type = %q,期望 dryrun", cfg.Mailer.Type)
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
