// Package tools 实现 agent 可调用的内容采集工具(Eino InvokableTool)。
package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// SeenStore 记录已经采集过的 URL,用于跨轮去重,避免邮件内容每天重复。
// 状态持久化为 JSON 文件,保留最近 retainDays 天的记录。
type SeenStore struct {
	path       string
	retainDays int

	mu   sync.Mutex
	seen map[string]time.Time // url -> 首次见到的时间
}

type seenStateOnDisk struct {
	Seen map[string]time.Time `json:"seen"`
}

// NewSeenStore 创建去重状态存储。retainDays 天之前的记录会在 Save 时清理。
// path 为空时表示纯内存模式(主要用于测试)。
func NewSeenStore(path string, retainDays int) (*SeenStore, error) {
	s := &SeenStore{
		path: path,
		seen: make(map[string]time.Time),
	}
	if s.path == "" {
		return s, nil
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("读取去重状态文件 %s: %w", s.path, err)
	}
	var st seenStateOnDisk
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, fmt.Errorf("解析去重状态文件 %s: %w", s.path, err)
	}
	if st.Seen != nil {
		s.seen = st.Seen
	}
	s.retainDays = retainDays
	return s, nil
}

// Filter 返回 urls 中尚未见过的子集(不改变状态)。
func (s *SeenStore) Filter(urls []string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]string, 0, len(urls))
	for _, u := range urls {
		if _, ok := s.seen[u]; !ok {
			out = append(out, u)
		}
	}
	return out
}

// Mark 将 urls 标记为已见。
func (s *SeenStore) Mark(urls []string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range urls {
		if _, ok := s.seen[u]; !ok {
			s.seen[u] = now
		}
	}
}

// Save 将状态写回磁盘并清理过期记录。path 为空时是空操作。
func (s *SeenStore) Save(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.path == "" {
		return nil
	}
	if s.retainDays > 0 {
		cutoff := now.AddDate(0, 0, -s.retainDays)
		for u, t := range s.seen {
			if t.Before(cutoff) {
				delete(s.seen, u)
			}
		}
	}
	raw, err := json.MarshalIndent(&seenStateOnDisk{Seen: s.seen}, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化去重状态: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("创建状态目录: %w", err)
	}
	if err := os.WriteFile(s.path, raw, 0o644); err != nil {
		return fmt.Errorf("写去重状态文件 %s: %w", s.path, err)
	}
	return nil
}
