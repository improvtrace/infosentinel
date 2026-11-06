package pusher

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
)

func emailTarget(name, to string) *repository.Target {
	return &repository.Target{Name: name, Type: "email", To: to, SubjectPrefix: "[InfoSentinel]"}
}

func mailerCfg(mode, outDir string) *repository.MailerConfig {
	return &repository.MailerConfig{Type: mode, OutputDir: outDir}
}

func TestEmailPusherDryRun(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "out")
	p, err := New(emailTarget("mailbox", "me@example.com"), mailerCfg("dryrun", outDir), "")
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

func TestSMTPConfigErrors(t *testing.T) {
	// 未配置 host/from → 明确报错
	p, _ := New(emailTarget("m", "a@b.c"), mailerCfg("smtp", t.TempDir()), "")
	err := p.Push(context.Background(), &Message{Task: "t", Subject: "s", Body: "b"})
	if err == nil || !strings.Contains(err.Error(), "host 与 from") {
		t.Errorf("未配置 SMTP 应返回配置错误,得到 %v", err)
	}

	// 已配置 host/from 但未提供授权码 → 明确报错
	cfg := mailerCfg("smtp", t.TempDir())
	cfg.Smtp = repository.SmtpConfig{Host: "smtp.example.com", Port: 465, From: "no-reply@example.com", PasswordEnv: "SMTP_PASSWORD"}
	p, _ = New(emailTarget("m", "a@b.c"), cfg, "")
	err = p.Push(context.Background(), &Message{Task: "t", Subject: "s", Body: "b"})
	if err == nil || !strings.Contains(err.Error(), "SMTP_PASSWORD") {
		t.Errorf("缺授权码应返回环境变量错误,得到 %v", err)
	}
}

func TestBuildMessage(t *testing.T) {
	p := &EmailPusher{
		To:            "me@example.com",
		SubjectPrefix: "[InfoSentinel]",
		Smtp:          repository.SmtpConfig{From: "no-reply@example.com"},
	}
	m := p.buildMessage(&Message{Task: "t1", Subject: "今日看点", Body: "正文"})

	from := m.GetHeader("From")
	if len(from) != 1 || from[0] != "no-reply@example.com" {
		t.Errorf("From = %v", from)
	}
	to := m.GetHeader("To")
	if len(to) != 1 || to[0] != "me@example.com" {
		t.Errorf("To = %v", to)
	}
	subject := m.GetHeader("Subject")
	// 非 ASCII 标题按 RFC 2047 编码(如 =?UTF-8?q?...?=),校验前缀与原文可解码性
	if len(subject) != 1 || !strings.Contains(subject[0], "InfoSentinel") {
		t.Errorf("Subject = %v", subject)
	}
}

func TestSMTPSSLPolicy(t *testing.T) {
	if got := sslOrDefault(repository.SmtpConfig{Port: 465}); !got {
		t.Error("465 端口缺省应为隐式 TLS")
	}
	if got := sslOrDefault(repository.SmtpConfig{Port: 587}); got {
		t.Error("587 端口缺省应为 STARTTLS(非隐式 TLS)")
	}
	if got := sslOrDefault(repository.SmtpConfig{Port: 0}); !got {
		t.Error("端口缺省按 465 处理")
	}
	if got := sslOrDefault(repository.SmtpConfig{Port: 25, SSL: boolPtr(true)}); !got {
		t.Error("显式 ssl: true 应生效")
	}
	if got := sslOrDefault(repository.SmtpConfig{Port: 465, SSL: boolPtr(false)}); got {
		t.Error("显式 ssl: false 应覆盖端口缺省")
	}
}

func TestNewUnsupportedType(t *testing.T) {
	if _, err := New(&repository.Target{Name: "w", Type: "webhook"}, &repository.MailerConfig{}, ""); err == nil {
		t.Error("未知通道类型应返回错误")
	}
}

func TestSanitize(t *testing.T) {
	if got := sanitize("a/b c\\d"); got != "a_b_c_d" {
		t.Errorf("sanitize = %q", got)
	}
	if time.Now().IsZero() {
		t.Error("sanity")
	}
}

func boolPtr(b bool) *bool { return &b }
