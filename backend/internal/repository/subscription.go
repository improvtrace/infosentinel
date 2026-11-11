package repository

import "context"

// SubscriptionRepository 订阅表操作。
type SubscriptionRepository interface {
	// ListSubscriptions 返回全部订阅(按名称排序)。
	ListSubscriptions(ctx context.Context) ([]Subscription, error)
	// GetSubscription 按名称查找;不存在返回 ErrNotFound。
	GetSubscription(ctx context.Context, name string) (*Subscription, error)
	// UpsertSubscription 新增或更新(按名称)。
	UpsertSubscription(ctx context.Context, s Subscription) error
	// DeleteSubscription 删除;不存在返回错误。
	DeleteSubscription(ctx context.Context, name string) error
}
