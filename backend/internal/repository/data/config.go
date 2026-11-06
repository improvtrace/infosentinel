package data

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data/ent"
)

// 设置段 key 常量。
const (
	settingSources = "sources"
	settingLLM     = "llm"
	settingMailer  = "mailer"
	settingServer  = "server"
)

// LoadConfig 实现 repository.ConfigRepository:从各表组装完整配置并校验。
func (d *Data) LoadConfig(ctx context.Context) (*repository.Config, error) {
	cfg := &repository.Config{}

	// 全局设置段(缺省时保持空值,由 Finalize 填默认)
	for key, dst := range map[string]any{
		settingSources: &cfg.Sources,
		settingLLM:     &cfg.LLM,
		settingMailer:  &cfg.Mailer,
		settingServer:  &cfg.Server,
	} {
		err := d.GetSetting(ctx, key, dst)
		if err != nil && err != repository.ErrNotFound {
			return nil, err
		}
	}

	var err error
	if cfg.Topics, err = d.ListTopics(ctx); err != nil {
		return nil, err
	}
	if cfg.Targets, err = d.ListTargets(ctx); err != nil {
		return nil, err
	}
	if cfg.Tasks, err = d.ListTasks(ctx); err != nil {
		return nil, err
	}

	if err := cfg.Finalize(); err != nil {
		return nil, fmt.Errorf("数据库配置不合法: %w", err)
	}
	return cfg, nil
}

// IsEmpty 实现 repository.ConfigRepository:业务表是否全部为空。
func (d *Data) IsEmpty(ctx context.Context) (bool, error) {
	counts := []func(context.Context) (int, error){
		d.client.Topic.Query().Count,
		d.client.Target.Query().Count,
		d.client.Task.Query().Count,
		d.client.Setting.Query().Count,
	}
	for _, count := range counts {
		n, err := count(ctx)
		if err != nil {
			return false, fmt.Errorf("统计表行数: %w", err)
		}
		if n > 0 {
			return false, nil
		}
	}
	return true, nil
}

// SeedFromConfig 实现 repository.ConfigRepository:用 c 的业务配置整体替换数据库内容。
func (d *Data) SeedFromConfig(ctx context.Context, c *repository.Config) error {
	tx, err := d.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("开启事务: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Topic.Delete().Exec(ctx); err != nil {
		return fmt.Errorf("清空主题: %w", err)
	}
	if _, err := tx.Target.Delete().Exec(ctx); err != nil {
		return fmt.Errorf("清空推送目标: %w", err)
	}
	if _, err := tx.Task.Delete().Exec(ctx); err != nil {
		return fmt.Errorf("清空任务: %w", err)
	}
	if _, err := tx.Setting.Delete().Exec(ctx); err != nil {
		return fmt.Errorf("清空设置: %w", err)
	}

	for _, t := range c.Topics {
		if err := tx.Topic.Create().SetName(t.Name).SetPrompt(t.Prompt).Exec(ctx); err != nil {
			return fmt.Errorf("导入主题 %q: %w", t.Name, err)
		}
	}
	for _, t := range c.Targets {
		if err := tx.Target.Create().
			SetName(t.Name).SetType(t.Type).SetTo(t.To).SetSubjectPrefix(t.SubjectPrefix).
			Exec(ctx); err != nil {
			return fmt.Errorf("导入推送目标 %q: %w", t.Name, err)
		}
	}
	for _, t := range c.Tasks {
		if err := tx.Task.Create().
			SetName(t.Name).SetTopic(t.Topic).SetTargets(t.Targets).SetSchedules(t.Schedules).
			Exec(ctx); err != nil {
			return fmt.Errorf("导入任务 %q: %w", t.Name, err)
		}
	}

	sections := map[string]any{
		settingSources: c.Sources,
		settingLLM:     c.LLM,
		settingMailer:  c.Mailer,
		settingServer:  c.Server,
	}
	for key, v := range sections {
		if err := putSettingTx(ctx, tx, key, v); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事务: %w", err)
	}
	return nil
}

// putSettingTx 在事务内写入设置段。
func putSettingTx(ctx context.Context, tx *ent.Tx, key string, v any) error {
	raw, err := jsonMarshal(v)
	if err != nil {
		return fmt.Errorf("序列化设置 %s: %w", key, err)
	}
	if err := tx.Setting.Create().SetKey(key).SetValue(string(raw)).Exec(ctx); err != nil {
		return fmt.Errorf("导入设置 %s: %w", key, err)
	}
	return nil
}

var _ repository.ConfigRepository = (*Data)(nil)

// jsonMarshal 是 JSON 序列化的内部薄封装(便于统一替换实现)。
func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }
