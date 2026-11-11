package pusher

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tomasgdl/infosentinel/backend/internal/artifact"
	"github.com/tomasgdl/infosentinel/backend/internal/repository"
	"github.com/tomasgdl/infosentinel/backend/pkg/storage"
)

func emailTarget(name, to string) *repository.Target {
	return &repository.Target{Name: name, Type: "email", To: to, SubjectPrefix: "[InfoSentinel]"}
}

func mailerCfg(mode string) *repository.MailerConfig {
	return &repository.MailerConfig{Type: mode}
}

// newTestArtifacts 在临时目录上构建含单个本地产物空间的集合。
func newTestArtifacts(t *testing.T) *artifact.Spaces {
	t.Helper()
	store, err := storage.New(storage.Config{Type: storage.TypeLocal, Root: t.TempDir()})
	if err != nil {
		t.Fatalf("创建本地存储: %v", err)
	}
	m, err := artifact.New(artifact.Options{Store: store})
	if err != nil {
		t.Fatalf("创建产物空间: %v", err)
	}
	spaces := artifact.NewSpaces()
	space := &repository.StorageSpace{Name: "默认空间", Type: repository.StorageTypeLocal}
	spaces.Add(space, m)
	return spaces
}

func TestEmailPusherInbox(t *testing.T) {
	arts := newTestArtifacts(t)
	p, err := New(emailTarget("mailbox", "me@example.com"), mailerCfg(repository.MailerTypeInbox), "", arts, "默认空间")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	res, err := p.Push(context.Background(), &Message{
		Subscription: "ai-agent",
		Subject:      "今日看点",
		Body:         "## Go\n- 新版本发布\n",
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	// 产物写入分桶子目录,文件名为 <订阅名>-<时间戳>.html
	files, err := arts.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("应生成 1 个产物文件,得到 %d", len(files))
	}
	f := files[0]
	// 结果应携带产物 DSN(类型://空间名/分桶/文件名),描述包含 DSN
	if len(res.Artifacts) != 1 {
		t.Fatalf("站内信推送应返回 1 条产物 DSN,得到 %v", res.Artifacts)
	}
	dsn := res.Artifacts[0]
	want := artifact.DSN(repository.StorageTypeLocal, "默认空间", f.Bucket, f.Name)
	if dsn != want {
		t.Errorf("DSN = %q,期望 %q", dsn, want)
	}
	space, bucket, name, perr := artifact.ParseDSN(dsn)
	if perr != nil || space != "默认空间" || bucket != f.Bucket || name != f.Name {
		t.Errorf("ParseDSN(%q) = (%q,%q,%q,%v)", dsn, space, bucket, name, perr)
	}
	if !strings.Contains(res.Detail, dsn) {
		t.Errorf("Push 描述 %q 应包含 DSN %q", res.Detail, dsn)
	}
	if !strings.HasPrefix(f.Name, "ai-agent-") || !strings.HasSuffix(f.Name, ".html") {
		t.Errorf("文件名应含订阅名且为 html: %q", f.Name)
	}

	raw, err := arts.Read(context.Background(), "默认空间", f.Bucket, f.Name)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	content := string(raw)
	for _, want := range []string{
		"今日看点",                // 标题(head.subject 与 <title>)
		"[InfoSentinel] 今日看点", // 前缀已拼接
		"<h2>Go</h2>",         // Markdown 已渲染为 HTML
		"新版本发布",
		"<!DOCTYPE html>",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("产物 HTML 缺少 %q", want)
		}
	}
}

func TestEmailPusherInboxRequiresSpace(t *testing.T) {
	arts := newTestArtifacts(t)
	// 未关联产物空间 → 明确报错
	p, _ := New(emailTarget("m", "a@b.c"), mailerCfg(repository.MailerTypeInbox), "", arts, "")
	_, err := p.Push(context.Background(), &Message{Subscription: "t", Subject: "s", Body: "b"})
	if err == nil || !strings.Contains(err.Error(), "未关联产物空间") {
		t.Errorf("未关联空间应返回配置引导错误,得到 %v", err)
	}

	// 关联了不存在的空间 → 明确报错
	p, _ = New(emailTarget("m", "a@b.c"), mailerCfg(repository.MailerTypeInbox), "", arts, "ghost")
	_, err = p.Push(context.Background(), &Message{Subscription: "t", Subject: "s", Body: "b"})
	if err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Errorf("引用不存在空间应报错,得到 %v", err)
	}
}

func TestSMTPConfigErrors(t *testing.T) {
	arts := newTestArtifacts(t)
	// 未配置 host/from → 明确报错
	p, _ := New(emailTarget("m", "a@b.c"), mailerCfg(repository.MailerTypeSMTP), "", arts, "")
	_, err := p.Push(context.Background(), &Message{Subscription: "t", Subject: "s", Body: "b"})
	if err == nil || !strings.Contains(err.Error(), "host 与发件人地址") {
		t.Errorf("未配置 SMTP 应返回配置错误,得到 %v", err)
	}

	// 已配置 host/from 但未提供授权码 → 明确报错
	cfg := mailerCfg(repository.MailerTypeSMTP)
	cfg.Smtp = repository.SmtpConfig{Host: "smtp.example.com", Port: 465, From: "no-reply@example.com"}
	p, _ = New(emailTarget("m", "a@b.c"), cfg, "", arts, "")
	_, err = p.Push(context.Background(), &Message{Subscription: "t", Subject: "s", Body: "b"})
	if err == nil || !strings.Contains(err.Error(), "授权码") {
		t.Errorf("缺授权码应返回配置引导错误,得到 %v", err)
	}
}

func TestBuildMessage(t *testing.T) {
	p := &EmailPusher{
		To:            "me@example.com",
		SubjectPrefix: "[InfoSentinel]",
		Smtp:          repository.SmtpConfig{From: "no-reply@example.com"},
	}
	m := p.buildMessage(&Message{Subscription: "t1", Subject: "今日看点", Body: "正文"})

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

	// 写出 MIME 报文,正文应为 HTML
	var buf bytes.Buffer
	if _, err := m.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if !strings.Contains(buf.String(), "Content-Type: text/html") {
		t.Error("正文应为 text/html")
	}
	if !strings.Contains(buf.String(), "<!DOCTYPE html>") {
		t.Error("正文应为渲染后的完整 HTML 文档")
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
	if _, err := New(&repository.Target{Name: "w", Type: "webhook"}, &repository.MailerConfig{}, "", nil, ""); err == nil {
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
