package repository

import "context"

// ConfigRepository 聚合读取:从各表组装完整配置并校验。
type ConfigRepository interface {
	// LoadConfig 组装完整配置并做完整校验(Finalize)。
	LoadConfig(ctx context.Context) (*Config, error)
}
