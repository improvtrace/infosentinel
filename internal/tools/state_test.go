package tools

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSeenStoreFilterMarkSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	now := time.Now()

	s1, err := NewSeenStore(path, 7)
	if err != nil {
		t.Fatalf("NewSeenStore: %v", err)
	}
	fresh := s1.Filter([]string{"a", "b", "c"})
	if len(fresh) != 3 {
		t.Fatalf("初始应全部未见,得到 %v", fresh)
	}
	s1.Mark([]string{"a", "b"}, now)
	if got := s1.Filter([]string{"a", "b", "c"}); len(got) != 1 || got[0] != "c" {
		t.Fatalf("Mark 后应只剩 c,得到 %v", got)
	}
	if err := s1.Save(now); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// 重新加载应保留状态
	s2, err := NewSeenStore(path, 7)
	if err != nil {
		t.Fatalf("重新加载: %v", err)
	}
	if got := s2.Filter([]string{"a", "b", "c"}); len(got) != 1 || got[0] != "c" {
		t.Fatalf("持久化后应只剩 c,得到 %v", got)
	}

	// 过期清理:8 天前的记录应被清掉
	s2.Mark([]string{"old"}, now.AddDate(0, 0, -8))
	if err := s2.Save(now); err != nil {
		t.Fatalf("Save 清理: %v", err)
	}
	s3, err := NewSeenStore(path, 7)
	if err != nil {
		t.Fatalf("第三次加载: %v", err)
	}
	if got := s3.Filter([]string{"old"}); len(got) != 1 {
		t.Fatalf("超过保留天数的记录应被清理,得到 %v", got)
	}
}

func TestSeenStoreMemoryMode(t *testing.T) {
	s, err := NewSeenStore("", 7) // path 为空 → 纯内存
	if err != nil {
		t.Fatalf("NewSeenStore(内存模式): %v", err)
	}
	s.Mark([]string{"x"}, time.Now())
	if err := s.Save(time.Now()); err != nil {
		t.Fatalf("内存模式 Save 应为空操作: %v", err)
	}
}

func TestTruncateAndStripHTML(t *testing.T) {
	if got := stripHTML("<p>Hello <b>世界</b>!\n</p>"); got != "Hello 世界!" {
		t.Errorf("stripHTML = %q", got)
	}
	long := make([]rune, 100)
	for i := range long {
		long[i] = '字'
	}
	got := truncate(string(long), 10)
	if len([]rune(got)) != 11 { // 10 字 + 省略号
		t.Errorf("truncate 长度 = %d", len([]rune(got)))
	}
	if got := truncate("short", 10); got != "short" {
		t.Errorf("短文本不应截断: %q", got)
	}
}

func TestFriendlyResult(t *testing.T) {
	got := friendlyResult(map[string]any{"items": []string{}})
	if got != `{"items":[]}` {
		t.Errorf("friendlyResult = %q", got)
	}
	if _, err := os.Stat("no-such-file"); !os.IsNotExist(err) {
		t.Error("sanity")
	}
}
