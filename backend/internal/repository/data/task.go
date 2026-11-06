package data

import (
	"context"
	"fmt"
	"sort"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data/ent"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data/ent/task"
)

// ListTasks 实现 repository.TaskRepository。
func (d *Data) ListTasks(ctx context.Context) ([]repository.Task, error) {
	rows, err := d.client.Task.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询任务: %w", err)
	}
	out := make([]repository.Task, 0, len(rows))
	for _, t := range rows {
		out = append(out, repository.Task{
			Name: t.Name, Topic: t.Topic, Targets: t.Targets, Schedules: t.Schedules,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// GetTask 实现 repository.TaskRepository。
func (d *Data) GetTask(ctx context.Context, name string) (*repository.Task, error) {
	t, err := d.client.Task.Query().Where(task.Name(name)).First(ctx)
	if ent.IsNotFound(err) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询任务 %q: %w", name, err)
	}
	return &repository.Task{Name: t.Name, Topic: t.Topic, Targets: t.Targets, Schedules: t.Schedules}, nil
}

// UpsertTask 实现 repository.TaskRepository。
func (d *Data) UpsertTask(ctx context.Context, t repository.Task) error {
	err := d.client.Task.Create().
		SetName(t.Name).
		SetTopic(t.Topic).
		SetTargets(t.Targets).
		SetSchedules(t.Schedules).
		OnConflict().
		UpdateTopic().
		UpdateTargets().
		UpdateSchedules().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("保存任务 %q: %w", t.Name, err)
	}
	return nil
}

// DeleteTask 实现 repository.TaskRepository。
func (d *Data) DeleteTask(ctx context.Context, name string) error {
	n, err := d.client.Task.Delete().Where(task.Name(name)).Exec(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("任务 %q 不存在", name)
	}
	return nil
}

var _ repository.TaskRepository = (*Data)(nil)
