package runner

import (
	"errors"
	"sync"
	"testing"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
)

var configStub = repository.Config{}

func TestWithSlotGuard(t *testing.T) {
	r := New(&configStub, nil, nil, "", nil)

	release := make(chan struct{})
	started := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = r.withSlot("t1", func() error {
			close(started)
			<-release
			return errors.New("done")
		})
	}()
	<-started

	// 运行中:同任务再次进入应被拒绝
	if !r.IsRunning("t1") {
		t.Fatal("IsRunning 应为 true")
	}
	err := r.withSlot("t1", func() error { t.Fatal("不应执行"); return nil })
	if err == nil {
		t.Fatal("运行中重复进入应报错")
	}

	// 释放后可再次进入
	close(release)
	wg.Wait()
	if r.IsRunning("t1") {
		t.Fatal("结束后 IsRunning 应为 false")
	}
	if err := r.withSlot("t1", func() error { return nil }); err != nil {
		t.Fatalf("释放后应可再次运行: %v", err)
	}
}
