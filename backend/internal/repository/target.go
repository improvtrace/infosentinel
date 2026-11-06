package repository

import "context"

// TargetRepository 推送目标表操作。
type TargetRepository interface {
	// ListTargets 返回全部推送目标(按名称排序)。
	ListTargets(ctx context.Context) ([]Target, error)
	// GetTarget 按名称查找;不存在返回 ErrNotFound。
	GetTarget(ctx context.Context, name string) (*Target, error)
	// UpsertTarget 新增或更新(按 name)。
	UpsertTarget(ctx context.Context, t Target) error
	// DeleteTarget 删除;被任务引用时返回错误并列出引用者。
	DeleteTarget(ctx context.Context, name string) error
	// UsedByTarget 返回引用该推送目标的任务数。
	UsedByTarget(ctx context.Context, name string) (int, error)
}
