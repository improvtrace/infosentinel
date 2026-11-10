package storage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// localStore 是本地磁盘驱动:Root 为磁盘根目录(推荐绝对路径)。
type localStore struct {
	root string
}

// newLocal 创建本地磁盘驱动。
func newLocal(root string) (*localStore, error) {
	if root == "" {
		return nil, fmt.Errorf("本地存储根目录未配置(引导配置 storage.base_dir)")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("解析本地根目录 %s: %w", root, err)
	}
	return &localStore{root: abs}, nil
}

// resolve 拼接并校验本地路径(防穿越)。
func (s *localStore) resolve(p string) (string, error) {
	c, err := CleanRel(p)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.root, filepath.FromSlash(c)), nil
}

// Save 实现 Store:写入文件(自动创建父目录)。
func (s *localStore) Save(_ context.Context, p string, data []byte) error {
	path, err := s.resolve(p)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("创建目录 %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写入文件 %s: %w", path, err)
	}
	return nil
}

// Read 实现 Store。
func (s *localStore) Read(_ context.Context, p string) ([]byte, error) {
	path, err := s.resolve(p)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, p)
	}
	if err != nil {
		return nil, fmt.Errorf("读取文件 %s: %w", p, err)
	}
	return raw, nil
}

// List 实现 Store:列出一级条目。
func (s *localStore) List(_ context.Context, dir string) ([]Entry, error) {
	path, err := s.resolve(dir)
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, dir)
	}
	if err != nil {
		return nil, fmt.Errorf("列出目录 %s: %w", dir, err)
	}
	out := make([]Entry, 0, len(ents))
	for _, e := range ents {
		item := Entry{Name: e.Name(), Dir: e.IsDir()}
		if info, err := e.Info(); err == nil {
			item.Size = info.Size()
			item.ModTime = info.ModTime()
		}
		out = append(out, item)
	}
	return out, nil
}

// Delete 实现 Store:删除文件。
func (s *localStore) Delete(_ context.Context, p string) error {
	path, err := s.resolve(p)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrNotFound, p)
		}
		return fmt.Errorf("删除文件 %s: %w", p, err)
	}
	return nil
}

// DeleteDir 实现 Store:递归删除目录。
func (s *localStore) DeleteDir(_ context.Context, dir string) error {
	path, err := s.resolve(dir)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrNotFound, dir)
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("删除目录 %s: %w", dir, err)
	}
	return nil
}

var _ Store = (*localStore)(nil)
