// Package pusher 定义推送通道抽象及其实现。
// 一个推送目标(target)对应一个 Pusher 实例;email 通道由服务内置的
// SMTP 发件邮箱(mailer.smtp 配置)投递,dryrun 模式落盘预览。
// 后续可扩展 webhook、IM 等通道类型。
package pusher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/mail.v2"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
)

// Message 是一条待推送的报告。
type Message struct {
	Task    string // 产生该报告的任务名,用于日志与落盘命名
	Subject string // 标题(不含通道前缀)
	Body    string // Markdown 正文
}

// Pusher 是推送通道抽象。
type Pusher interface {
	// Push 推送一条报告。
	Push(ctx context.Context, msg *Message) error
}

// New 按目标配置、全局发送器配置与已解析的 SMTP 授权码创建通道实例。
// password 由调用方通过 secret.Resolver 解析(环境变量优先,其次数据库密文)。
func New(target *repository.Target, mcfg *repository.MailerConfig, password string) (Pusher, error) {
	switch target.Type {
	case "email":
		return &EmailPusher{
			To:            target.To,
			SubjectPrefix: target.SubjectPrefix,
			Mode:          mcfg.Type,
			OutDir:        mcfg.OutputDir,
			Smtp:          mcfg.Smtp,
			Password:      password,
		}, nil
	default:
		// repository.Validate 已拦截非法类型;此处兜底
		return nil, fmt.Errorf("不支持的推送通道类型: %q", target.Type)
	}
}

// EmailPusher 是 email 通道:dry-run 落盘预览;smtp 走内置发件邮箱真实投递。
type EmailPusher struct {
	To            string
	SubjectPrefix string
	Mode          string // dryrun | smtp
	OutDir        string
	Smtp          repository.SmtpConfig
	Password      string // SMTP 授权码(已解析,不落库不明文)
}

// Push 发送(或落盘)邮件。
func (p *EmailPusher) Push(_ context.Context, msg *Message) error {
	switch p.Mode {
	case "dryrun":
		return p.writeDryRun(msg)
	case "smtp":
		return p.sendSMTP(msg)
	default:
		return fmt.Errorf("未知 mailer.type: %q", p.Mode)
	}
}

// sendSMTP 通过内置 SMTP 发件邮箱投递邮件。
func (p *EmailPusher) sendSMTP(msg *Message) error {
	if p.Smtp.Host == "" || p.Smtp.From == "" {
		return fmt.Errorf("SMTP 发件邮箱未配置:请在 mailer.smtp 中填写 host 与 from")
	}
	password := p.Password
	if password == "" {
		return fmt.Errorf("SMTP 密码/授权码未配置:请设置环境变量 %s,或在 Web 界面“全局设置”中填写(加密存储)", p.Smtp.PasswordEnv)
	}

	m := p.buildMessage(msg)

	port := p.Smtp.Port
	if port == 0 {
		port = 465
	}
	d := mail.NewDialer(p.Smtp.Host, port, p.Smtp.Username, password)
	d.Timeout = 30 * time.Second
	d.SSL = sslOrDefault(p.Smtp)

	if err := d.DialAndSend(m); err != nil {
		return fmt.Errorf("SMTP 发送失败(%s:%d): %w", p.Smtp.Host, port, err)
	}
	fmt.Printf("[pusher] email→%s 已通过 %s:%d 发送\n", p.To, p.Smtp.Host, port)
	return nil
}

// sslOrDefault 返回是否使用隐式 TLS:显式 ssl 配置优先;缺省按端口判断(465 → true)。
func sslOrDefault(c repository.SmtpConfig) bool {
	if c.SSL != nil {
		return *c.SSL
	}
	port := c.Port
	if port == 0 {
		port = 465
	}
	return port == 465
}

// buildMessage 组装 MIME 邮件(独立出来便于单测头部编码)。
func (p *EmailPusher) buildMessage(msg *Message) *mail.Message {
	m := mail.NewMessage()
	m.SetHeader("From", p.Smtp.From)
	m.SetHeader("To", p.To)
	m.SetHeader("Subject", p.applyPrefix(msg.Subject))
	m.SetDateHeader("Date", time.Now())
	m.SetBody("text/plain; charset=utf-8", msg.Body)
	return m
}

// writeDryRun 将邮件写入 OutDir/email-<task>-<timestamp>.md。
func (p *EmailPusher) writeDryRun(msg *Message) error {
	if err := os.MkdirAll(p.OutDir, 0o755); err != nil {
		return fmt.Errorf("创建输出目录 %s: %w", p.OutDir, err)
	}

	name := fmt.Sprintf("email-%s-%s.md", sanitize(msg.Task), time.Now().Format("20060102-150405"))
	path := filepath.Join(p.OutDir, name)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("<!--\nTo: %s\nSubject: %s\nTask: %s\nGenerated: %s\n-->\n\n",
		p.To, p.applyPrefix(msg.Subject), msg.Task, time.Now().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("# %s\n\n", p.applyPrefix(msg.Subject)))
	b.WriteString(msg.Body)
	b.WriteString("\n")

	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("写邮件文件 %s: %w", path, err)
	}
	fmt.Printf("[pusher] email→%s dry-run:邮件已写入 %s\n", p.To, path)
	return nil
}

// applyPrefix 拼接标题前缀。
func (p *EmailPusher) applyPrefix(subject string) string {
	if p.SubjectPrefix == "" {
		return subject
	}
	return fmt.Sprintf("%s %s", p.SubjectPrefix, subject)
}

// sanitize 清理文件名中的路径分隔符等不安全字符。
func sanitize(s string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", " ", "_")
	return r.Replace(s)
}
