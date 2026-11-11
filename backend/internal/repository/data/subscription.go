package data

import (
	"context"
	"fmt"
	"sort"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data/ent"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data/ent/subscription"
)

// ListSubscriptions 实现 repository.SubscriptionRepository。
func (d *Data) ListSubscriptions(ctx context.Context) ([]repository.Subscription, error) {
	rows, err := d.client.Subscription.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询订阅: %w", err)
	}
	out := make([]repository.Subscription, 0, len(rows))
	for _, s := range rows {
		out = append(out, subscriptionOf(s))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// GetSubscription 实现 repository.SubscriptionRepository。
func (d *Data) GetSubscription(ctx context.Context, name string) (*repository.Subscription, error) {
	s, err := d.client.Subscription.Query().Where(subscription.Name(name)).First(ctx)
	if ent.IsNotFound(err) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询订阅 %q: %w", name, err)
	}
	sub := subscriptionOf(s)
	return &sub, nil
}

// UpsertSubscription 实现 repository.SubscriptionRepository(按名称)。
func (d *Data) UpsertSubscription(ctx context.Context, s repository.Subscription) error {
	err := d.client.Subscription.Create().
		SetName(s.Name).
		SetTopics(s.Topics).
		SetSynthesis(s.Synthesis).
		SetCron(s.Cron).
		SetTargets(s.Targets).
		SetSpace(s.Space).
		OnConflict().
		UpdateTopics().
		UpdateSynthesis().
		UpdateCron().
		UpdateTargets().
		UpdateSpace().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("保存订阅 %q: %w", s.Name, err)
	}
	return nil
}

// DeleteSubscription 实现 repository.SubscriptionRepository。
func (d *Data) DeleteSubscription(ctx context.Context, name string) error {
	n, err := d.client.Subscription.Delete().Where(subscription.Name(name)).Exec(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("订阅 %q 不存在", name)
	}
	return nil
}

// subscriptionOf 把 ent 实体映射为领域模型。
func subscriptionOf(s *ent.Subscription) repository.Subscription {
	return repository.Subscription{
		Name:      s.Name,
		Topics:    s.Topics,
		Synthesis: s.Synthesis,
		Cron:      s.Cron,
		Targets:   s.Targets,
		Space:     s.Space,
	}
}

var _ repository.SubscriptionRepository = (*Data)(nil)
