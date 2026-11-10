// run.go 定义订阅运行记录仓库:持久化的运行历史(替代旧内存 history.Store)。
package repository

import (
	"context"
	"time"
)

// RunStatus 是运行记录状态。
type RunStatus string

// 运行状态取值(与前端 RunStatus 一致)。
const (
	RunStatusRunning RunStatus = "running"
	RunStatusSuccess RunStatus = "success"
	RunStatusFailed  RunStatus = "failed"
)

// 运行触发方式取值。
const (
	RunTriggerSchedule = "schedule" // 定时调度触发
	RunTriggerManual   = "manual"   // Web 界面手动触发
)

// RunRecord 是一条订阅运行记录。
// JSON 字段名与前端 RunRecord 类型一一对应。
type RunRecord struct {
	ID         int64         `json:"id"`
	Topic      string        `json:"topic"` // 订阅主题(订阅身份)
	Trigger    string        `json:"trigger"`
	Status     RunStatus     `json:"status"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt time.Time     `json:"finished_at"`
	Duration   time.Duration `json:"duration"`
	Error      string        `json:"error"`
	Subject    string        `json:"subject"`
	Body       string        `json:"body"`
}

// RunRepository 是订阅运行记录仓库。
type RunRepository interface {
	// StartRun 创建一条 running 状态的记录,返回记录 ID。
	StartRun(ctx context.Context, topic, trigger string) (int64, error)
	// FinishRun 收尾一条记录:成功时写入 subject/body,失败时写入错误信息;
	// 记录不存在时静默返回(与旧内存历史行为一致)。
	FinishRun(ctx context.Context, id int64, runErr error, subject, body string) error
	// ListRuns 按开始时间倒序列出运行记录;topic 非空时按订阅(主题)过滤;
	// limit <= 0 时不限制。
	ListRuns(ctx context.Context, topic string, limit int) ([]RunRecord, error)
	// GetRun 按 ID 查询单条记录,不存在返回 ErrNotFound。
	GetRun(ctx context.Context, id int64) (RunRecord, error)
	// CountRuns 返回记录总数。
	CountRuns(ctx context.Context) (int, error)
}
