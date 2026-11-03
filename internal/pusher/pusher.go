// Package pusher 定义推送通道抽象及其实现。
// 一个推送目标(target)对应一个 Pusher 实例;当前支持 email 通道
// (dry-run 落盘,SMTP 预留),后续可扩展 webhook、IM 等通道类型。
package pusher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tomasgdl/infosentinel/internal/config"
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

// New 按目标配置与全局发送器配置创建通道实例。
func New(target *config.Target, mcfg *config.MailerConfig) (Pusher, error) {
	switch target.Type {
	case "email":
		return &EmailPusher{
			To:            target.To,
			SubjectPrefix: target.SubjectPrefix,
			Mode:          mcfg.Type,
			OutDir:        mcfg.OutputDir,
		}, nil
	default:
		// config.Validate 已拦截非法类型;此处兜底
		return nil, fmt.Errorf("不支持的推送通道类型: %q", target.Type)
	}
}

// EmailPusher 是 email 通道:当前 dry-run 模式落盘,SMTP 模式为预留。
type EmailPusher struct {
	To            string
	SubjectPrefix string
	Mode          string // dryrun | smtp
	OutDir        string
}

// Push 发送(或落盘)邮件。
func (p *EmailPusher) Push(_ context.Context, msg *Message) error {
	switch p.Mode {
	case "dryrun":
		return p.writeDryRun(msg)
	case "smtp":
		return fmt.Errorf("mailer.type=smtp 尚未实现:当前版本请使用 dryrun(生成邮件文件);接入 SMTP 的步骤见 .docs/development.md")
	default:
		return fmt.Errorf("未知 mailer.type: %q", p.Mode)
	}
}

// writeDryRun 将邮件写入 OutDir/email-<task>-<timestamp>.md。
func (p *EmailPusher) writeDryRun(msg *Message) error {
	if err := os.MkdirAll(p.OutDir, 0o755); err != nil {
		return fmt.Errorf("创建输出目录 %s: %w", p.OutDir, err)
	}

	name := fmt.Sprintf("email-%s-%s.md", sanitize(msg.Task), time.Now().Format("20060102-150405"))
	path := filepath.Join(p.OutDir, name)

	subject := p.applyPrefix(msg.Subject)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("<!--\nTo: %s\nSubject: %s\nTask: %s\nGenerated: %s\n-->\n\n",
		p.To, subject, msg.Task, time.Now().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("# %s\n\n", subject))
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
