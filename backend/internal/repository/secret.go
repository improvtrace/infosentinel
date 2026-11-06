package repository

import "context"

// SecretRepository 敏感数据操作(静态加密存储)。
type SecretRepository interface {
	// SetSecret 加密后写入(按 name upsert);未配置加密主密钥时报错。
	SetSecret(ctx context.Context, name, plaintext string) error
	// GetSecret 解密读取;不存在返回 ErrNotFound。
	GetSecret(ctx context.Context, name string) (string, error)
	// DeleteSecret 删除。
	DeleteSecret(ctx context.Context, name string) error
	// ListSecrets 返回已设置的名称列表(不返回值)。
	ListSecrets(ctx context.Context) ([]string, error)
}
