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

func TestSeenStoreMultiMergeAndSave(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.json")
	pathB := filepath.Join(dir, "b.json")
	now := time.Now()

	// 两个主题各自已有的状态:shared 两边都有(b 更早),另各有独占条目
	sa, err := NewSeenStore(pathA, 7)
	if err != nil {
		t.Fatalf("NewSeenStore(a): %v", err)
	}
	sa.Mark([]string{"shared", "only-a"}, now)
	if err := sa.Save(now); err != nil {
		t.Fatalf("Save(a): %v", err)
	}
	sb, err := NewSeenStore(pathB, 7)
	if err != nil {
		t.Fatalf("NewSeenStore(b): %v", err)
	}
	sb.Mark([]string{"shared", "only-b"}, now.Add(-time.Hour))
	if err := sb.Save(now); err != nil {
		t.Fatalf("Save(b): %v", err)
	}

	// 合并加载:任一文件见过的条目都视为已见
	merged, err := NewSeenStoreMulti([]string{pathA, pathB}, 7)
	if err != nil {
		t.Fatalf("NewSeenStoreMulti: %v", err)
	}
	if got := merged.Filter([]string{"shared", "only-a", "only-b", "new"}); len(got) != 1 || got[0] != "new" {
		t.Fatalf("合并后应只剩 new 未见,得到 %v", got)
	}
	merged.Mark([]string{"new"}, now)
	if err := merged.Save(now); err != nil {
		t.Fatalf("合并状态 Save: %v", err)
	}

	// 保存应写回全部文件:任一文件单独加载都持有完整合并状态
	for _, p := range []string{pathA, pathB} {
		s, err := NewSeenStore(p, 7)
		if err != nil {
			t.Fatalf("重新加载 %s: %v", p, err)
		}
		if got := s.Filter([]string{"shared", "only-a", "only-b", "new"}); len(got) != 0 {
			t.Errorf("%s 的合并状态未写回,未见: %v", p, got)
		}
	}

	// 含空路径 → 纯内存模式,Save 为空操作
	mm, err := NewSeenStoreMulti([]string{""}, 7)
	if err != nil {
		t.Fatalf("NewSeenStoreMulti(内存模式): %v", err)
	}
	mm.Mark([]string{"x"}, now)
	if err := mm.Save(now); err != nil {
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
