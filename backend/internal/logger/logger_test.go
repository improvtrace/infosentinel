package logger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestJSONLogFormat 验证 Infow(event, k, v) 输出为规范 JSON 行。
func TestJSONLogFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "test.log")
	h, closer, err := New(Options{Path: path, Level: "DEBUG"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer closer.Close()

	h.Infow("task_run_done", KeyTask, "demo", KeyStatus, "success")
	h.Warnf("something_odd %d", 1)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取日志: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) < 2 {
		t.Fatalf("日志行数 = %d", len(lines))
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatalf("首行不是合法 JSON: %q", lines[0])
	}
	if m["msg"] != "task_run_done" || m["task"] != "demo" || m["status"] != "success" {
		t.Errorf("字段不符: %v", m)
	}
	if m["level"] != "INFO" || m["ts"] == nil {
		t.Errorf("level/ts 缺失: %v", m)
	}

	var w map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &w); err != nil {
		t.Fatalf("第二行不是合法 JSON: %q", lines[1])
	}
	if w["level"] != "WARN" {
		t.Errorf("Warnf 级别不符: %v", w)
	}
}

func TestParseLevel(t *testing.T) {
	cases := map[string]string{"debug": "DEBUG", "WARN": "WARN", "error": "ERROR", "": "INFO", "x": "INFO"}
	for in, want := range cases {
		if got := parseLevel(in).String(); got != want {
			t.Errorf("parseLevel(%q) = %s, want %s", in, got, want)
		}
	}
}
