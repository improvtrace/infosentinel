package repository

import "context"

// TaskRepository 任务表操作。
type TaskRepository interface {
	// ListTasks 返回全部任务(按名称排序)。
	ListTasks(ctx context.Context) ([]Task, error)
	// GetTask 按名称查找;不存在返回 ErrNotFound。
	GetTask(ctx context.Context, name string) (*Task, error)
	// UpsertTask 新增或更新(按 name)。
	UpsertTask(ctx context.Context, t Task) error
	// DeleteTask 删除;不存在返回错误。
	DeleteTask(ctx context.Context, name string) error
}
