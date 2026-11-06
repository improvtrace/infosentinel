package data

import (
	"context"
	"fmt"
	"sort"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data/ent"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data/ent/task"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data/ent/topic"
)

// List 实现 repository.TopicRepository。
func (d *Data) ListTopics(ctx context.Context) ([]repository.Topic, error) {
	rows, err := d.client.Topic.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询主题: %w", err)
	}
	out := make([]repository.Topic, 0, len(rows))
	for _, t := range rows {
		out = append(out, repository.Topic{Name: t.Name, Prompt: t.Prompt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// GetTopic 实现 repository.TopicRepository。
func (d *Data) GetTopic(ctx context.Context, name string) (*repository.Topic, error) {
	t, err := d.client.Topic.Query().Where(topic.Name(name)).First(ctx)
	if ent.IsNotFound(err) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询主题 %q: %w", name, err)
	}
	return &repository.Topic{Name: t.Name, Prompt: t.Prompt}, nil
}

// UpsertTopic 实现 repository.TopicRepository。
func (d *Data) UpsertTopic(ctx context.Context, t repository.Topic) error {
	err := d.client.Topic.Create().
		SetName(t.Name).
		SetPrompt(t.Prompt).
		OnConflict().
		UpdatePrompt().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("保存主题 %q: %w", t.Name, err)
	}
	return nil
}

// DeleteTopic 实现 repository.TopicRepository:被任务引用时拒绝。
func (d *Data) DeleteTopic(ctx context.Context, name string) error {
	tasks, err := d.tasksUsingTopic(ctx, name)
	if err != nil {
		return err
	}
	if len(tasks) > 0 {
		return fmt.Errorf("主题 %q 仍被任务引用:%s;请先修改或删除这些任务", name, joinNames(tasks))
	}
	n, err := d.client.Topic.Delete().Where(topic.Name(name)).Exec(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("主题 %q 不存在", name)
	}
	return nil
}

// UsedByTopic 实现 repository.TopicRepository。
func (d *Data) UsedByTopic(ctx context.Context, name string) (int, error) {
	return d.client.Task.Query().Where(task.Topic(name)).Count(ctx)
}

// tasksUsingTopic 返回引用某主题的任务名列表。
func (d *Data) tasksUsingTopic(ctx context.Context, topicName string) ([]string, error) {
	rows, err := d.client.Task.Query().Where(task.Topic(topicName)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询主题引用: %w", err)
	}
	out := make([]string, 0, len(rows))
	for _, t := range rows {
		out = append(out, t.Name)
	}
	sort.Strings(out)
	return out, nil
}

var _ repository.TopicRepository = (*Data)(nil)
