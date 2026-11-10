// Package logger 基于 kratos log 构建结构化日志:JSON 行写入文件并同步输出到 stdout。
//
// 埋点约定(调用方以 keyvals 附加结构化字段):
//   - 任务:task_run_done(task, trigger, duration, status, targets)
//   - 推送:secret_saved(name) / 任务级失败 task_run_failed
//   - 调度:scheduler_started / scheduler_applied / shutdown
//   - 接入:api_request(method, path, code, duration) 由 server 访问过滤器统一写入
package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	klog "github.com/go-kratos/kratos/v2/log"
)

// Options 是日志初始化参数(来自引导配置 config.yaml 的 log 段)。
type Options struct {
	// Path 是日志文件路径;目录不存在会自动创建。
	Path string
	// Level 是日志级别:DEBUG / INFO / WARN / ERROR(不区分大小写,默认 INFO)。
	Level string
}

// 常用埋点键名,保持调用方字段命名一致。
const (
	KeyTopic    = "topic"
	KeyTrigger  = "trigger"
	KeyDuration = "duration"
	KeyStatus   = "status"
	KeyTargets  = "targets"
	KeyTo       = "to"
	KeyMode     = "mode"
	KeyPath     = "path"
	KeyMethod   = "method"
	KeyCode     = "code"
	KeyCount    = "count"
)

// jsonLogger 把 kratos 的 keyvals 序列化为单行 JSON。
type jsonLogger struct {
	mu sync.Mutex
	w  io.Writer
}

// Log 实现 kratos log.Logger:输出 {"ts","level",...keyvals,"msg"}。
func (l *jsonLogger) Log(level klog.Level, keyvals ...any) error {
	m := map[string]any{
		"ts":    time.Now().Format(time.RFC3339),
		"level": level.String(),
	}
	// 约定:Infow(event, k1, v1, ...) —— 奇数个参数时首个为事件名(msg)
	n := len(keyvals)
	start := 0
	if n%2 == 1 {
		m["msg"] = fmt.Sprint(keyvals[0])
		start = 1
	}
	for i := start; i+1 < n; i += 2 {
		key := fmt.Sprint(keyvals[i])
		if key == "" {
			key = "key"
		}
		m[key] = keyvals[i+1]
	}

	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err = fmt.Fprintln(l.w, string(raw))
	return err
}

type fileCloser struct{ f *os.File }

func (c *fileCloser) Close() error { return c.f.Close() }

// New 初始化 kratos 日志:JSON 行写入 Path 并同步 stdout。
// 返回的 Helper 供全服务使用;Closer 在进程退出时关闭文件(可为 nil)。
func New(opts Options) (*klog.Helper, io.Closer, error) {
	level := parseLevel(opts.Level)

	if err := os.MkdirAll(filepath.Dir(opts.Path), 0o755); err != nil {
		return nil, nil, fmt.Errorf("创建日志目录: %w", err)
	}
	f, err := os.OpenFile(opts.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("打开日志文件 %s: %w", opts.Path, err)
	}

	out := io.MultiWriter(f, os.Stdout)
	filtered := klog.NewFilter(&jsonLogger{w: out}, klog.FilterLevel(level))
	helper := klog.NewHelper(filtered, klog.WithMessageKey("msg"))

	return helper, &fileCloser{f: f}, nil
}

// NewDiscard 返回丢弃日志的 Helper(测试/工具场景)。
func NewDiscard() *klog.Helper {
	return klog.NewHelper(klog.NewStdLogger(io.Discard))
}

// parseLevel 解析日志级别字符串。
func parseLevel(s string) klog.Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return klog.LevelDebug
	case "WARN", "WARNING":
		return klog.LevelWarn
	case "ERROR":
		return klog.LevelError
	default:
		return klog.LevelInfo
	}
}
