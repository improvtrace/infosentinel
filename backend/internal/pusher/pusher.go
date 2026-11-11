// Package pusher 定义推送通道抽象及其实现。
// 一个推送目标(target)对应一个 Pusher 实例;email 通道按全局投递类型
// 二选一:站内信(写入产物空间,界面在线查看)或由内置 SMTP 发件邮箱
// (mailer.smtp 配置)真实投递。后续可扩展 webhook、IM 等通道类型。
package pusher

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gopkg.in/mail.v2"

	"github.com/tomasgdl/infosentinel/backend/internal/artifact"
	"github.com/tomasgdl/infosentinel/backend/internal/repository"
	"github.com/tomasgdl/infosentinel/backend/pkg/render"
)

// Message 是一条待推送的报告。
type Message struct {
	Subscription string // 产生该报告的订阅名,用于日志与产物命名
	Subject      string // 标题(不含通道前缀)
	Body         string // 报告正文(Markdown 或 HTML 片段,推送前统一渲染为完整 HTML 文档)
}

// Result 是一次推送的结果:描述文案与产物 DSN 列表(仅站内信模式非空)。
type Result struct {
	// Detail 是结果描述(投递摘要/产物 DSN),用于测试反馈与日志。
	Detail string
	// Artifacts 是本次推送写入的产物路径(DSN 格式:类型://空间名/分桶/文件名)。
	Artifacts []string
}

// Pusher 是推送通道抽象。
type Pusher interface {
	// Push 推送一条报告;失败返回错误。
	Push(ctx context.Context, msg *Message) (*Result, error)
}

// New 按目标配置、全局发送器配置、已解析的 SMTP 密码与产物空间集合创建通道实例。
// space 为订阅关联的产物空间名(站内信模式写入目标;可空);password 由调用方
// 通过 config.Resolver 解析(页面配置,加密存储)。
func New(target *repository.Target, mcfg *repository.MailerConfig, password string, spaces *artifact.Spaces, space string) (Pusher, error) {
	switch target.Type {
	case "email":
		return &EmailPusher{
			To:            target.To,
			SubjectPrefix: target.SubjectPrefix,
			Mode:          mcfg.Type,
			Smtp:          mcfg.Smtp,
			Password:      password,
			Spaces:        spaces,
			Space:         space,
		}, nil
	default:
		// repository.Validate 已拦截非法类型;此处兜底
		return nil, fmt.Errorf("不支持的推送通道类型: %q", target.Type)
	}
}

// EmailPusher 是 email 通道:站内信模式写入订阅关联的产物空间;smtp 走内置发件邮箱真实投递。
type EmailPusher struct {
	To            string
	SubjectPrefix string
	Mode          string // repository.MailerTypeInbox | repository.MailerTypeSMTP
	Smtp          repository.SmtpConfig
	Password      string // SMTP 授权码(已解析,不落库不明文)
	Spaces        *artifact.Spaces
	Space         string // 订阅关联的产物空间名(空 = 不写产物空间)
}

// Push 发送(或落盘)邮件。
func (p *EmailPusher) Push(ctx context.Context, msg *Message) (*Result, error) {
	switch p.Mode {
	case repository.MailerTypeInbox:
		return p.writeArtifact(ctx, msg)
	case repository.MailerTypeSMTP:
		return p.sendSMTP(msg)
	default:
		return nil, fmt.Errorf("未知 mailer.type: %q", p.Mode)
	}
}

// sendSMTP 通过内置 SMTP 发件邮箱投递 HTML 邮件。
func (p *EmailPusher) sendSMTP(msg *Message) (*Result, error) {
	if p.Smtp.Host == "" || p.Smtp.From == "" {
		return nil, fmt.Errorf("SMTP 发件邮箱未配置:请在 Web 界面“配置 → 全局设置 → 发送”中填写 host 与发件人地址")
	}
	password := p.Password
	if password == "" {
		return nil, fmt.Errorf("SMTP 密码/授权码未配置:请在 Web 界面“配置 → 全局设置”中填写并加密保存")
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
		return nil, fmt.Errorf("SMTP 发送失败(%s:%d): %w", p.Smtp.Host, port, err)
	}
	detail := fmt.Sprintf("email→%s 已通过 %s:%d 发送", p.To, p.Smtp.Host, port)
	fmt.Printf("[pusher] %s\n", detail)
	return &Result{Detail: detail}, nil
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
	m.SetBody("text/html; charset=utf-8", render.EmailHTML(p.applyPrefix(msg.Subject), msg.Body, time.Now()))
	return m
}

// writeArtifact 将报告渲染为 HTML 写入订阅关联的产物空间:
// <分桶>/<topic>-<时间戳>.html,返回带产物 DSN 的结果。
func (p *EmailPusher) writeArtifact(ctx context.Context, msg *Message) (*Result, error) {
	if p.Space == "" {
		return nil, fmt.Errorf("订阅未关联产物空间:请在订阅设置中选择,或改用 smtp 投递")
	}
	if p.Spaces == nil {
		return nil, fmt.Errorf("产物空间未就绪:请检查 storage 配置")
	}
	mgr, ok := p.Spaces.Manager(p.Space)
	if !ok {
		return nil, fmt.Errorf("产物空间 %q 不存在:请检查订阅的空间引用与全局设置", p.Space)
	}
	html := render.EmailHTML(p.applyPrefix(msg.Subject), msg.Body, time.Now())
	name := fmt.Sprintf("%s-%s.html", sanitize(msg.Subscription), time.Now().Format("20060102-150405"))
	bucket, err := mgr.Save(ctx, name, []byte(html))
	if err != nil {
		return nil, fmt.Errorf("写入产物 %s: %w", name, err)
	}
	dsn := artifact.DSN(p.Spaces.Type(p.Space), p.Space, bucket, name)
	detail := fmt.Sprintf("email→%s 站内信:产物已写入 %s", p.To, dsn)
	fmt.Printf("[pusher] %s\n", detail)
	return &Result{Detail: detail, Artifacts: []string{dsn}}, nil
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
