package repository

import "context"

// SettingRepository 全局设置段操作(key ∈ llm/mailer/server/storage)。
type SettingRepository interface {
	// GetSetting 读取 key 对应的 JSON 段并反序列化到 out;不存在返回 ErrNotFound。
	GetSetting(ctx context.Context, key string, out any) error
	// PutSetting 序列化 v 并覆盖保存。
	PutSetting(ctx context.Context, key string, v any) error
}
