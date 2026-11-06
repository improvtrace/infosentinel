// Package scheduler 按 cron 表达式调度任务,支持一个任务关联多个表达式,优雅退出。
package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/robfig/cron/v3"
)

// Job 是一个可调度的任务。
type Job struct {
	// Name 是任务名(仅用于日志)。
	Name string
	// Schedules 是 cron 表达式列表(五段),每个表达式独立触发。
	Schedules []string
	// Run 是任务执行函数,由调度器包一层超时控制。
	Run func(ctx context.Context) error
}

// NextRun 描述某个 (任务, 表达式) 组合的下次触发时间。
type NextRun struct {
	Task     string    `json:"task"`
	Schedule string    `json:"schedule"`
	Next     time.Time `json:"next"`
}

// Scheduler 包装 robfig/cron。
type Scheduler struct {
	c       *cron.Cron
	timeout time.Duration // 单轮任务的超时上限
	regs    []regEntry    // 已注册的 (任务, 表达式) 条目
}

type regEntry struct {
	nr NextRun
	id cron.EntryID
}

// New 创建调度器;timeout 为每轮任务的执行超时(通常取 llm.timeout_sec)。
func New(timeout time.Duration) *Scheduler {
	return &Scheduler{
		// 标准五段表达式;按本地时区调度
		c:       cron.New(cron.WithLocation(time.Local)),
		timeout: timeout,
	}
}

// Start 注册全部 (任务, 表达式) 组合并启动调度,返回各组合的下次触发时间。
// 任一表达式非法即返回错误(不会部分启动)。
func (s *Scheduler) Start(jobs []Job) ([]NextRun, error) {
	s.regs = s.regs[:0]

	for _, j := range jobs {
		for _, sched := range j.Schedules {
			run, name := j.Run, j.Name
			wrapped := func() {
				// 每轮任务独立超时,避免慢轮询阻塞后续调度
				ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
				defer cancel()
				if err := run(ctx); err != nil {
					log.Printf("[scheduler] 任务 %s 执行失败: %v", name, err)
				}
			}
			id, err := s.c.AddFunc(sched, wrapped)
			if err != nil {
				s.c.Stop()
				return nil, fmt.Errorf("任务 %s 的 cron 表达式 %q 不合法: %w", j.Name, sched, err)
			}
			s.regs = append(s.regs, regEntry{nr: NextRun{Task: j.Name, Schedule: sched}, id: id})
		}
	}

	s.c.Start()
	return s.NextRuns(), nil
}

// NextRuns 返回全部 (任务, 表达式) 的下次触发时间(每次运行后自动更新)。
func (s *Scheduler) NextRuns() []NextRun {
	out := make([]NextRun, 0, len(s.regs))
	for _, r := range s.regs {
		nr := r.nr
		nr.Next = s.c.Entry(r.id).Next
		out = append(out, nr)
	}
	return out
}

// Stop 停止调度并等待运行中的任务结束。
func (s *Scheduler) Stop() context.Context {
	return s.c.Stop()
}
