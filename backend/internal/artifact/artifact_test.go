package artifact

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/tomasgdl/infosentinel/backend/pkg/storage"
)

// newLocalManager 在临时目录上构建本地产物空间。
func newLocalManager(t *testing.T, opts Options) *Manager {
	t.Helper()
	if opts.Store == nil {
		store, err := storage.New(storage.Config{Type: storage.TypeLocal, Root: filepath.Join(t.TempDir(), "root")})
		if err != nil {
			t.Fatalf("创建本地存储: %v", err)
		}
		opts.Store = store
	}
	m, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return m
}

// TestBucketOf 校验分桶命名:按周期起始日期(UTC)。
func TestBucketOf(t *testing.T) {
	utc := func(s string) time.Time {
		v, _ := time.Parse(time.RFC3339, s)
		return v
	}
	cases := []struct {
		now    time.Time
		period int
		want   string
	}{
		{utc("2026-11-09T08:00:00Z"), 1, "2026-11-09"},
		{utc("2026-11-09T08:00:00Z"), 7, "2026-11-05"},  // 11-05 起的一周
		{utc("2026-11-09T08:00:00Z"), 30, "2026-11-03"}, // 11-03 起的 30 天
		{utc("2026-11-09T08:00:00Z"), 0, "2026-11-09"},  // 非法周期按 1 天
	}
	for _, c := range cases {
		if got := BucketOf(c.now, c.period); got != c.want {
			t.Errorf("BucketOf(%s, %d) = %q,期望 %q", c.now, c.period, got, c.want)
		}
	}
}

// TestSaveListRead 校验保存/列表/读取的往返与分桶归位。
func TestSaveListRead(t *testing.T) {
	m := newLocalManager(t, Options{Path: "products", PeriodDays: 1})
	ctx := context.Background()

	bucket, err := m.Save(ctx, "ai-agent-daily-20261109-080000.html", []byte("<h1>ok</h1>"))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	wantBucket := BucketOf(time.Now(), 1)
	if bucket != wantBucket {
		t.Errorf("Save 返回分桶 %q,期望 %q", bucket, wantBucket)
	}

	files, err := m.List(ctx)
	if err != nil || len(files) != 1 {
		t.Fatalf("List = %+v err=%v", files, err)
	}
	if files[0].Bucket != wantBucket || files[0].Name != "ai-agent-daily-20261109-080000.html" {
		t.Errorf("List 条目不符: %+v", files[0])
	}

	raw, err := m.Read(ctx, wantBucket, "ai-agent-daily-20261109-080000.html")
	if err != nil || string(raw) != "<h1>ok</h1>" {
		t.Errorf("Read = %q err=%v", raw, err)
	}

	// 读取不存在的产物 → ErrNotFound
	if _, err := m.Read(ctx, wantBucket, "ghost.html"); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("读取不存在产物应 ErrNotFound,得到 %v", err)
	}
	// 非法分桶/文件名
	if _, err := m.Read(ctx, "../evil", "x.html"); err == nil {
		t.Error("非法分桶应报错")
	}
	if _, err := m.Save(ctx, "../evil.html", []byte("x")); err == nil {
		t.Error("穿越文件名应报错")
	}
	if _, err := m.Save(ctx, "a/b.html", []byte("x")); err == nil {
		t.Error("多级产物名应报错")
	}
}

// TestPruneOldestBuckets 校验保留策略:超过 maxDirs 时删除最旧分桶。
func TestPruneOldestBuckets(t *testing.T) {
	store, err := storage.New(storage.Config{Type: storage.TypeLocal, Root: filepath.Join(t.TempDir(), "root")})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// 人工构造 3 个历史分桶(均在今天之前)
	for _, b := range []string{"2026-01-01", "2026-01-02", "2026-01-03"} {
		if err := store.Save(ctx, "products/"+b+"/old.html", []byte("x")); err != nil {
			t.Fatal(err)
		}
	}

	// 写入今天的分桶且 maxDirs=2:首次保存即触发清理,
	// 按名称倒序保留最新 2 个(今天与 2026-01-03),删除更旧的
	today := BucketOf(time.Now(), 1)
	m := newLocalManager(t, Options{Store: store, Path: "products", PeriodDays: 1, MaxDirs: 2})
	if today == "2026-01-01" || today == "2026-01-02" || today == "2026-01-03" {
		t.Fatalf("测试前提失效:今天 %q 与预设分桶冲突", today)
	}
	if _, err := m.Save(ctx, "cur.html", []byte("y")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	dirs, err := store.List(ctx, "products")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, d := range dirs {
		if d.Dir {
			names[d.Name] = true
		}
	}
	if names["2026-01-01"] || names["2026-01-02"] {
		t.Errorf("最旧分桶应被清理,实际保留 %v", names)
	}
	if !names["2026-01-03"] || !names[today] {
		t.Errorf("应保留 2026-01-03 与当前分桶 %s,实际 %v", today, names)
	}
}

// TestNewValidation 校验构造参数校验。
func TestNewValidation(t *testing.T) {
	if _, err := New(Options{}); err == nil {
		t.Error("缺存储驱动应报错")
	}
	if _, err := New(Options{Store: &fakeStore{}, Path: "../evil"}); err == nil {
		t.Error("穿越产物路径应报错")
	}
}

// TestDSNRoundTrip 校验产物 DSN 的构造与解析往返,以及非法格式拦截。
func TestDSNRoundTrip(t *testing.T) {
	dsn := DSN("local", "默认空间", "2026-11-11", "ai-agent-20261111-080000.html")
	space, bucket, name, err := ParseDSN(dsn)
	if err != nil {
		t.Fatalf("ParseDSN: %v", err)
	}
	if space != "默认空间" || bucket != "2026-11-11" || name != "ai-agent-20261111-080000.html" {
		t.Errorf("ParseDSN(%q) = (%q,%q,%q)", dsn, space, bucket, name)
	}

	for _, bad := range []string{
		"",
		"no-scheme",
		"://空间/2026-11-11/a.html",    // 缺类型
		"local://空间/a.html",          // 缺文件名段
		"local:///2026-11-11/a.html", // 缺空间名
	} {
		if _, _, _, err := ParseDSN(bad); err == nil {
			t.Errorf("非法 DSN %q 应解析失败", bad)
		}
	}
}

// fakeStore 是占位存储驱动(仅用于构造参数校验)。
type fakeStore struct{}

func (fakeStore) Save(context.Context, string, []byte) error            { return nil }
func (fakeStore) Read(context.Context, string) ([]byte, error)          { return nil, nil }
func (fakeStore) List(context.Context, string) ([]storage.Entry, error) { return nil, nil }
func (fakeStore) Delete(context.Context, string) error                  { return nil }
func (fakeStore) DeleteDir(context.Context, string) error               { return nil }
