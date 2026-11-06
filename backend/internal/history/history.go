// Package history 维护 serve 模式下任务运行历史的内存环形存储,
// 供 Web 管理界面展示最近运行与手动触发后的状态跟踪。重启进程后清空;
// 持久化的运行产物(邮件文件)见 mailer.output_dir。
package history

import (
	"sync"
	"time"
)

// Status 是一次任务运行的状态。
type Status string

const (
	Running Status = "running"
	Success Status = "success"
	Failed  Status = "failed"
)

// 触发来源。
const (
	TriggerSchedule = "schedule"
	TriggerManual   = "manual"
)

// Record 是一次任务运行的记录。
type Record struct {
	ID         int64         `json:"id"`
	Task       string        `json:"task"`
	Trigger    string        `json:"trigger"`
	Status     Status        `json:"status"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt time.Time     `json:"finished_at"`
	Duration   time.Duration `json:"duration"`
	Error      string        `json:"error"`
	Subject    string        `json:"subject"`
	Body       string        `json:"body"` // 成功时的报告 Markdown 正文
}

// Store 是容量受控的运行历史存储,并发安全。
type Store struct {
	mu   sync.Mutex
	seq  int64
	cap  int
	recs []*Record // 按 ID 升序;展示时倒序(最新在前)
}

// New 创建存储;cap <= 0 时使用默认容量 200。
func New(capacity int) *Store {
	if capacity <= 0 {
		capacity = 200
	}
	return &Store{cap: capacity}
}

// Start 记录一次运行开始,返回该记录(运行中状态)。
func (s *Store) Start(task, trigger string) *Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	r := &Record{
		ID:        s.seq,
		Task:      task,
		Trigger:   trigger,
		Status:    Running,
		StartedAt: time.Now(),
	}
	s.recs = append(s.recs, r)
	if len(s.recs) > s.cap {
		s.recs = s.recs[len(s.recs)-s.cap:]
	}
	return r
}

// Finish 结束一次运行:失败时记录错误;成功时记录报告标题与正文。
func (s *Store) Finish(id int64, runErr error, subject, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.recs {
		if r.ID != id {
			continue
		}
		r.FinishedAt = time.Now()
		r.Duration = r.FinishedAt.Sub(r.StartedAt)
		if runErr != nil {
			r.Status = Failed
			r.Error = runErr.Error()
		} else {
			r.Status = Success
			r.Subject = subject
			r.Body = body
		}
		return
	}
}

// Snapshot 返回全部记录的副本,最新在前。
func (s *Store) Snapshot() []Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Record, 0, len(s.recs))
	for i := len(s.recs) - 1; i >= 0; i-- {
		out = append(out, *s.recs[i])
	}
	return out
}

// Get 按 ID 查找记录副本。
func (s *Store) Get(id int64) (Record, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.recs {
		if r.ID == id {
			return *r, true
		}
	}
	return Record{}, false
}

// Len 返回当前记录条数。
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.recs)
}
