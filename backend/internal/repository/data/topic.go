package data

import (
	"context"
	"fmt"
	"sort"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data/ent"
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

// DeleteTopic 实现 repository.TopicRepository:已有订阅时拒绝。
func (d *Data) DeleteTopic(ctx context.Context, name string) error {
	subs, err := d.subscriptionsUsingTopic(ctx, name)
	if err != nil {
		return err
	}
	if len(subs) > 0 {
		return fmt.Errorf("主题 %q 已被订阅引用:%s;请先在订阅管理中移除关联或删除对应订阅", name, joinNames(subs))
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

// UsedByTopic 实现 repository.TopicRepository:引用该主题的订阅数(多对多,扫描数组)。
func (d *Data) UsedByTopic(ctx context.Context, name string) (int, error) {
	subs, err := d.subscriptionsUsingTopic(ctx, name)
	if err != nil {
		return 0, err
	}
	return len(subs), nil
}

// subscriptionsUsingTopic 扫描订阅并统计 topics JSON 中引用该主题的订阅名称列表。
func (d *Data) subscriptionsUsingTopic(ctx context.Context, name string) ([]string, error) {
	subs, err := d.ListSubscriptions(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, s := range subs {
		for _, ref := range s.Topics {
			if ref == name {
				out = append(out, s.Name)
				break
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

var _ repository.TopicRepository = (*Data)(nil)
