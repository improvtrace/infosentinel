package repository

import "context"

// TopicRepository 主题表操作。
type TopicRepository interface {
	// ListTopics 返回全部主题(按名称排序)。
	ListTopics(ctx context.Context) ([]Topic, error)
	// GetTopic 按名称查找;不存在返回 ErrNotFound。
	GetTopic(ctx context.Context, name string) (*Topic, error)
	// UpsertTopic 新增或更新(按 name)。
	UpsertTopic(ctx context.Context, t Topic) error
	// DeleteTopic 删除;被任务引用时返回错误并列出引用者。
	DeleteTopic(ctx context.Context, name string) error
	// UsedByTopic 返回引用该主题的任务数。
	UsedByTopic(ctx context.Context, name string) (int, error)
}
