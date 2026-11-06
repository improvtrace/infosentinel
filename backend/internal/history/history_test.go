package history

import (
	"errors"
	"testing"
)

func TestStoreLifecycle(t *testing.T) {
	s := New(0)

	r1 := s.Start("t1", TriggerSchedule)
	if r1.Status != Running || r1.ID != 1 {
		t.Fatalf("首条记录状态不符: %+v", r1)
	}
	if s.Len() != 1 {
		t.Fatalf("Len = %d", s.Len())
	}

	// 运行中快照应保持 running
	snap := s.Snapshot()
	if len(snap) != 1 || snap[0].Status != Running {
		t.Fatalf("运行中快照不符: %+v", snap)
	}

	s.Finish(r1.ID, nil, "今日看点", "正文内容")
	got, ok := s.Get(r1.ID)
	if !ok || got.Status != Success || got.Subject != "今日看点" || got.Body != "正文内容" {
		t.Fatalf("成功记录不符: %+v", got)
	}
	if got.FinishedAt.IsZero() || got.Duration <= 0 {
		t.Errorf("完成时间与耗时未记录: %+v", got)
	}

	// 失败路径
	r2 := s.Start("t2", TriggerManual)
	s.Finish(r2.ID, errors.New("boom"), "", "")
	got, _ = s.Get(r2.ID)
	if got.Status != Failed || got.Error != "boom" || got.Body != "" {
		t.Fatalf("失败记录不符: %+v", got)
	}

	// 最新在前
	snap = s.Snapshot()
	if snap[0].ID != r2.ID || snap[1].ID != r1.ID {
		t.Errorf("快照应最新在前: %v", snap)
	}

	// 不存在的 ID
	if _, ok := s.Get(999); ok {
		t.Error("不存在的 ID 不应命中")
	}
}

func TestStoreCapacity(t *testing.T) {
	s := New(3)
	for i := 0; i < 5; i++ {
		r := s.Start("t", TriggerSchedule)
		s.Finish(r.ID, nil, "", "")
	}
	if s.Len() != 3 {
		t.Fatalf("环形容量应为 3,得到 %d", s.Len())
	}
	snap := s.Snapshot()
	// 最早的 1、2 被淘汰,保留 3、4、5
	if snap[len(snap)-1].ID != 3 {
		t.Errorf("最旧保留记录应为 #3,得到 #%d", snap[len(snap)-1].ID)
	}
}
