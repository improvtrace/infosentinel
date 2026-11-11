package runner

import (
	"errors"
	"sync"
	"testing"

	"github.com/tomasgdl/infosentinel/backend/internal/artifact"
	"github.com/tomasgdl/infosentinel/backend/internal/repository"
)

var configStub = repository.Config{}

func TestWithSlotGuard(t *testing.T) {
	r := New(&configStub, nil, artifact.NewSpaces(), "", nil)

	release := make(chan struct{})
	started := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = r.withSlot([]string{"s1", "t1"}, func() error {
			close(started)
			<-release
			return errors.New("done")
		})
	}()
	<-started

	// 运行中:同任务再次进入应被拒绝
	if !r.IsRunning("s1") {
		t.Fatal("IsRunning 应为 true")
	}
	err := r.withSlot([]string{"s1"}, func() error { t.Fatal("不应执行"); return nil })
	if err == nil {
		t.Fatal("运行中重复进入应报错")
	}

	// 主题键也被占用:共享主题的其他订阅应被拒绝(去重状态文件互斥)
	if err := r.withSlot([]string{"s2", "t1"}, func() error { t.Fatal("共享主题不应执行"); return nil }); err == nil {
		t.Fatal("共享主题的并发执行应报错")
	}

	// 释放后可再次进入
	close(release)
	wg.Wait()
	if r.IsRunning("s1") {
		t.Fatal("结束后 IsRunning 应为 false")
	}
	if err := r.withSlot([]string{"s1", "t1"}, func() error { return nil }); err != nil {
		t.Fatalf("释放后应可再次运行: %v", err)
	}
}
