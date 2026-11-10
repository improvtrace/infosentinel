package data

import (
	"context"
	"fmt"
	"time"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data/ent"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data/ent/subscriptionrun"
)

// StartRun 实现 repository.RunRepository:创建一条 running 状态的运行记录。
func (d *Data) StartRun(ctx context.Context, topic, trigger string) (int64, error) {
	row, err := d.client.SubscriptionRun.Create().
		SetTopic(topic).
		SetTrigger(trigger).
		SetStatus(string(repository.RunStatusRunning)).
		SetStartedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("创建运行记录 topic=%s: %w", topic, err)
	}
	return int64(row.ID), nil
}

// FinishRun 实现 repository.RunRepository:成功写 subject/body,失败写错误信息;
// 记录不存在时静默返回。
func (d *Data) FinishRun(ctx context.Context, id int64, runErr error, subject, body string) error {
	row, err := d.client.SubscriptionRun.Get(ctx, int(id))
	if ent.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取运行记录 %d: %w", id, err)
	}
	now := time.Now()
	up := d.client.SubscriptionRun.UpdateOne(row).
		SetFinishedAt(now).
		SetDuration(int64(now.Sub(row.StartedAt))).
		SetSubject(subject).
		SetBody(body)
	if runErr != nil {
		up = up.SetStatus(string(repository.RunStatusFailed)).SetError(runErr.Error())
	} else {
		up = up.SetStatus(string(repository.RunStatusSuccess))
	}
	if err := up.Exec(ctx); err != nil {
		return fmt.Errorf("收尾运行记录 %d: %w", id, err)
	}
	return nil
}

// ListRuns 实现 repository.RunRepository:按开始时间倒序;topic 非空时过滤。
func (d *Data) ListRuns(ctx context.Context, topic string, limit int) ([]repository.RunRecord, error) {
	q := d.client.SubscriptionRun.Query().Order(ent.Desc(subscriptionrun.FieldStartedAt, subscriptionrun.FieldID))
	if topic != "" {
		q = q.Where(subscriptionrun.Topic(topic))
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("查询运行记录: %w", err)
	}
	out := make([]repository.RunRecord, 0, len(rows))
	for _, r := range rows {
		out = append(out, runRecord(r))
	}
	return out, nil
}

// GetRun 实现 repository.RunRepository,不存在返回 ErrNotFound。
func (d *Data) GetRun(ctx context.Context, id int64) (repository.RunRecord, error) {
	row, err := d.client.SubscriptionRun.Get(ctx, int(id))
	if ent.IsNotFound(err) {
		return repository.RunRecord{}, repository.ErrNotFound
	}
	if err != nil {
		return repository.RunRecord{}, fmt.Errorf("读取运行记录 %d: %w", id, err)
	}
	return runRecord(row), nil
}

// CountRuns 实现 repository.RunRepository。
func (d *Data) CountRuns(ctx context.Context) (int, error) {
	n, err := d.client.SubscriptionRun.Query().Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("统计运行记录: %w", err)
	}
	return n, nil
}

// runRecord 把 ent 实体映射为领域模型。
func runRecord(r *ent.SubscriptionRun) repository.RunRecord {
	return repository.RunRecord{
		ID:         int64(r.ID),
		Topic:      r.Topic,
		Trigger:    r.Trigger,
		Status:     repository.RunStatus(r.Status),
		StartedAt:  r.StartedAt,
		FinishedAt: r.FinishedAt,
		Duration:   time.Duration(r.Duration),
		Error:      r.Error,
		Subject:    r.Subject,
		Body:       r.Body,
	}
}

var _ repository.RunRepository = (*Data)(nil)
