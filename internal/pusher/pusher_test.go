package pusher

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tomasgdl/infosentinel/internal/config"
)

func emailTarget(name, to string) *config.Target {
	return &config.Target{Name: name, Type: "email", To: to, SubjectPrefix: "[InfoSentinel]"}
}

func TestEmailPusherDryRun(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "out")
	p, err := New(emailTarget("mailbox", "me@example.com"), &config.MailerConfig{Type: "dryrun", OutputDir: outDir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = p.Push(context.Background(), &Message{
		Task:    "ai-agent-daily",
		Subject: "今日看点",
		Body:    "## Go\n- 新版本发布\n",
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("读取输出目录: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("应生成 1 个邮件文件,得到 %d", len(entries))
	}
	name := entries[0].Name()
	if !strings.HasPrefix(name, "email-ai-agent-daily-") || !strings.HasSuffix(name, ".md") {
		t.Errorf("文件名应含任务名: %q", name)
	}

	raw, err := os.ReadFile(filepath.Join(outDir, name))
	if err != nil {
		t.Fatalf("读取邮件文件: %v", err)
	}
	content := string(raw)
	for _, want := range []string{
		"To: me@example.com",
		"Subject: [InfoSentinel] 今日看点", // 前缀已拼接
		"Task: ai-agent-daily",
		"## Go",
		"新版本发布",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("邮件内容缺少 %q", want)
		}
	}
}

func TestNewUnsupportedType(t *testing.T) {
	if _, err := New(&config.Target{Name: "w", Type: "webhook"}, &config.MailerConfig{}); err == nil {
		t.Error("未知通道类型应返回错误")
	}
}

func TestEmailPusherSMTPNotImplemented(t *testing.T) {
	p, err := New(emailTarget("m", "a@b.c"), &config.MailerConfig{Type: "smtp", OutputDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = p.Push(context.Background(), &Message{Task: "t", Subject: "s", Body: "b"})
	if err == nil || !strings.Contains(err.Error(), "尚未实现") {
		t.Errorf("smtp 应返回未实现错误,得到 %v", err)
	}
}

func TestSanitize(t *testing.T) {
	if got := sanitize("a/b c\\d"); got != "a_b_c_d" {
		t.Errorf("sanitize = %q", got)
	}
}
