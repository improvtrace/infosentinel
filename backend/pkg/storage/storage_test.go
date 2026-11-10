package storage

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// TestCleanRel 校验相对路径清洗与穿越拦截。
func TestCleanRel(t *testing.T) {
	cases := []struct {
		in   string
		want string
		err  bool
	}{
		{in: "", want: ""},
		{in: "a/b.html", want: "a/b.html"},
		{in: "/a/b/", want: "a/b"},
		{in: "a//b/./c", want: "a/b/c"},
		{in: `a\ b.txt`, want: "a/ b.txt"}, // 反斜杠视为分隔;空格是合法文件名字符
		{in: "../a", err: true},
		{in: "a/../../b", err: true},
		{in: "..", err: true},
	}
	for _, c := range cases {
		got, err := CleanRel(c.in)
		if c.err {
			if err == nil {
				t.Errorf("CleanRel(%q) 应报错,得到 %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("CleanRel(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("CleanRel(%q) = %q,期望 %q", c.in, got, c.want)
		}
	}
}

// TestLocalStoreCRUD 校验本地驱动:写(自动建目录)/读/列/删/递归删与不存在语义。
func TestLocalStoreCRUD(t *testing.T) {
	ctx := context.Background()
	s, err := New(Config{Type: TypeLocal, Root: filepath.Join(t.TempDir(), "root")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// 写入(含缺失父目录)
	if err := s.Save(ctx, "2026-01-05/a/b.html", []byte("<h1>hi</h1>")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := s.Read(ctx, "2026-01-05/a/b.html")
	if err != nil || string(raw) != "<h1>hi</h1>" {
		t.Fatalf("Read = %q err=%v", raw, err)
	}

	// 列表
	ents, err := s.List(ctx, "2026-01-05/a")
	if err != nil || len(ents) != 1 || ents[0].Name != "b.html" || ents[0].Dir {
		t.Fatalf("List = %+v err=%v", ents, err)
	}

	// 路径穿越被拦截
	if err := s.Save(ctx, "../escape.txt", []byte("x")); err == nil {
		t.Error("穿越路径应报错")
	}
	if err := s.Save(ctx, "a/../../escape.txt", []byte("x")); err == nil {
		t.Error("间接穿越路径应报错")
	}

	// 删除与不存在语义
	if err := s.Delete(ctx, "2026-01-05/a/b.html"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Read(ctx, "2026-01-05/a/b.html"); !errors.Is(err, ErrNotFound) {
		t.Errorf("删除后 Read 应 ErrNotFound,得到 %v", err)
	}
	if err := s.Delete(ctx, "2026-01-05/a/b.html"); !errors.Is(err, ErrNotFound) {
		t.Errorf("删除不存在文件应 ErrNotFound,得到 %v", err)
	}

	// 递归删目录
	if err := s.Save(ctx, "2026-01-05/a/x.html", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteDir(ctx, "2026-01-05"); err != nil {
		t.Fatalf("DeleteDir: %v", err)
	}
	if _, err := s.List(ctx, "2026-01-05"); !errors.Is(err, ErrNotFound) {
		t.Errorf("删除目录后 List 应 ErrNotFound,得到 %v", err)
	}
}

// TestNewValidatesConfig 校验驱动工厂的类型与必填校验。
func TestNewValidatesConfig(t *testing.T) {
	if _, err := New(Config{Type: "webdav"}); err == nil {
		t.Error("未知类型应报错")
	}
	if _, err := New(Config{Type: TypeLocal}); err == nil {
		t.Error("local 缺根目录应报错")
	}
	if _, err := New(Config{Type: TypeSFTP}); err == nil {
		t.Error("sftp 缺 host 应报错")
	}
	if _, err := New(Config{Type: TypeFTP}); err == nil {
		t.Error("ftp 缺 host 应报错")
	}
	if _, err := New(Config{Type: TypeSFTP, Host: "h", HostKey: "not-a-key"}); err == nil {
		t.Error("非法 host_key 应报错")
	}
}

// TestJoinRoot 校验根目录拼接:绝对根保留,空根回退相对路径。
func TestJoinRoot(t *testing.T) {
	cases := []struct{ root, rel, want string }{
		{"", "a/b", "a/b"},
		{"/srv/artifacts", "2026-01-05/x.html", "/srv/artifacts/2026-01-05/x.html"},
		{"uploads", "x.html", "uploads/x.html"},
		{"/srv/artifacts/", "", "/srv/artifacts"},
	}
	for _, c := range cases {
		if got := joinRoot(c.root, c.rel); got != c.want {
			t.Errorf("joinRoot(%q, %q) = %q,期望 %q", c.root, c.rel, got, c.want)
		}
	}
}

// TestTimeoutOf 校验超时缺省。
func TestTimeoutOf(t *testing.T) {
	if timeoutOf(0).String() != "15s" {
		t.Errorf("timeoutOf(0) = %s", timeoutOf(0))
	}
	if timeoutOf(30).String() != "30s" {
		t.Errorf("timeoutOf(30) = %s", timeoutOf(30))
	}
	if !strings.HasPrefix(timeoutOf(-1).String(), "15") {
		t.Errorf("负值应取默认超时")
	}
}
