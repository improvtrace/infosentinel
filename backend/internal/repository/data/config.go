package data

import (
	"context"
	"fmt"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
)

// 设置段 key 常量。
const (
	settingLLM     = "llm"
	settingMailer  = "mailer"
	settingServer  = "server"
	settingStorage = "storage"
)

// LoadConfig 实现 repository.ConfigRepository:从各表组装完整配置并校验。
func (d *Data) LoadConfig(ctx context.Context) (*repository.Config, error) {
	cfg := &repository.Config{}

	// 全局设置段(缺省时保持空值,由 Finalize 填默认)
	for key, dst := range map[string]any{
		settingLLM:     &cfg.LLM,
		settingMailer:  &cfg.Mailer,
		settingServer:  &cfg.Server,
		settingStorage: &cfg.Storage,
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
	if cfg.Subscriptions, err = d.ListSubscriptions(ctx); err != nil {
		return nil, err
	}

	if err := cfg.Finalize(); err != nil {
		return nil, fmt.Errorf("数据库配置不合法: %w", err)
	}
	return cfg, nil
}

var _ repository.ConfigRepository = (*Data)(nil)
