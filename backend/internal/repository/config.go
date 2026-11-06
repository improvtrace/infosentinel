package repository

import "context"

// ConfigRepository 聚合读取与种子导入:从各表组装完整配置并校验。
type ConfigRepository interface {
	// LoadConfig 组装完整配置并做完整校验(Finalize)。
	LoadConfig(ctx context.Context) (*Config, error)
	// IsEmpty 判断业务表是否全空(用于内置种子初始化)。
	IsEmpty(ctx context.Context) (bool, error)
	// SeedFromConfig 用 c 的业务配置整体替换数据库内容(仅用于空库初始化)。
	SeedFromConfig(ctx context.Context, c *Config) error
}
