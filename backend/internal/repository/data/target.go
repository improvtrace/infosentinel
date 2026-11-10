package data

import (
	"context"
	"fmt"
	"sort"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data/ent"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data/ent/target"
)

// ListTargets 实现 repository.TargetRepository。
func (d *Data) ListTargets(ctx context.Context) ([]repository.Target, error) {
	rows, err := d.client.Target.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询推送目标: %w", err)
	}
	out := make([]repository.Target, 0, len(rows))
	for _, t := range rows {
		out = append(out, repository.Target{
			Name: t.Name, Type: t.Type, To: t.To, SubjectPrefix: t.SubjectPrefix,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// GetTarget 实现 repository.TargetRepository。
func (d *Data) GetTarget(ctx context.Context, name string) (*repository.Target, error) {
	t, err := d.client.Target.Query().Where(target.Name(name)).First(ctx)
	if ent.IsNotFound(err) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询推送目标 %q: %w", name, err)
	}
	return &repository.Target{Name: t.Name, Type: t.Type, To: t.To, SubjectPrefix: t.SubjectPrefix}, nil
}

// UpsertTarget 实现 repository.TargetRepository。
func (d *Data) UpsertTarget(ctx context.Context, t repository.Target) error {
	err := d.client.Target.Create().
		SetName(t.Name).
		SetType(t.Type).
		SetTo(t.To).
		SetSubjectPrefix(t.SubjectPrefix).
		OnConflict().
		UpdateType().
		UpdateTo().
		UpdateSubjectPrefix().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("保存推送目标 %q: %w", t.Name, err)
	}
	return nil
}

// DeleteTarget 实现 repository.TargetRepository:被订阅引用时拒绝。
func (d *Data) DeleteTarget(ctx context.Context, name string) error {
	subs, err := d.subscriptionsUsingTarget(ctx, name)
	if err != nil {
		return err
	}
	if len(subs) > 0 {
		return fmt.Errorf("推送目标 %q 仍被订阅引用:%s;请先修改或删除这些订阅", name, joinNames(subs))
	}
	n, err := d.client.Target.Delete().Where(target.Name(name)).Exec(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("推送目标 %q 不存在", name)
	}
	return nil
}

// UsedByTarget 实现 repository.TargetRepository:引用该目标的订阅数。
func (d *Data) UsedByTarget(ctx context.Context, name string) (int, error) {
	subs, err := d.subscriptionsUsingTarget(ctx, name)
	if err != nil {
		return 0, err
	}
	return len(subs), nil
}

// subscriptionsUsingTarget 扫描订阅并统计 targets JSON 中引用该目标的订阅主题列表。
func (d *Data) subscriptionsUsingTarget(ctx context.Context, name string) ([]string, error) {
	subs, err := d.ListSubscriptions(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, s := range subs {
		for _, ref := range s.Targets {
			if ref == name {
				out = append(out, s.Topic)
				break
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

var _ repository.TargetRepository = (*Data)(nil)
