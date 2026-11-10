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
		out = append(out, repository.Subscription{Topic: s.Topic, Cron: s.Cron, Targets: s.Targets})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Topic < out[j].Topic })
	return out, nil
}

// GetSubscription 实现 repository.SubscriptionRepository。
func (d *Data) GetSubscription(ctx context.Context, topic string) (*repository.Subscription, error) {
	s, err := d.client.Subscription.Query().Where(subscription.Topic(topic)).First(ctx)
	if ent.IsNotFound(err) {
		return nil, repository.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询订阅 %q: %w", topic, err)
	}
	return &repository.Subscription{Topic: s.Topic, Cron: s.Cron, Targets: s.Targets}, nil
}

// UpsertSubscription 实现 repository.SubscriptionRepository。
func (d *Data) UpsertSubscription(ctx context.Context, s repository.Subscription) error {
	err := d.client.Subscription.Create().
		SetTopic(s.Topic).
		SetCron(s.Cron).
		SetTargets(s.Targets).
		OnConflict().
		UpdateCron().
		UpdateTargets().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("保存订阅 %q: %w", s.Topic, err)
	}
	return nil
}

// DeleteSubscription 实现 repository.SubscriptionRepository。
func (d *Data) DeleteSubscription(ctx context.Context, topic string) error {
	n, err := d.client.Subscription.Delete().Where(subscription.Topic(topic)).Exec(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("订阅 %q 不存在", topic)
	}
	return nil
}

var _ repository.SubscriptionRepository = (*Data)(nil)
