// provider.go 提供 wire 依赖注入的应用组装 Provider:
// newConfig 从数据库加载生效配置,newApp 组装 serve 常驻应用(kratos app)。
package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/go-kratos/kratos/v2"
	klog "github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"

	"github.com/tomasgdl/infosentinel/backend/internal/agent"
	"github.com/tomasgdl/infosentinel/backend/internal/artifact"
	"github.com/tomasgdl/infosentinel/backend/internal/config"
	"github.com/tomasgdl/infosentinel/backend/internal/pusher"
	"github.com/tomasgdl/infosentinel/backend/internal/repository"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data"
	"github.com/tomasgdl/infosentinel/backend/internal/runner"
	"github.com/tomasgdl/infosentinel/backend/internal/scheduler"
	"github.com/tomasgdl/infosentinel/backend/internal/server"
)

// cliProviderSet 是 CLI 子命令的 wire ProviderSet。
var cliProviderSet = wire.NewSet(newCLIApp)

// appProviderSet 是 serve 应用的 wire ProviderSet。
var appProviderSet = wire.NewSet(newConfig, newApp)

// newCLIApp 聚合 CLI 子命令(run/list/migrate)所需依赖。
func newCLIApp(boot *config.Bootstrap, d *data.Data, lg *klog.Helper, secrets *config.Resolver) *cliApp {
	return &cliApp{Boot: boot, Data: d, Log: lg, Secrets: secrets}
}

// newConfig 从数据库加载当前生效配置(初始快照;热更新走 Deps.Apply)。
func newConfig(repo repository.ConfigRepository) (*repository.Config, error) {
	return repo.LoadConfig(context.Background())
}

// newApp 组装 serve 常驻应用(kratos app 生命周期):
//   - kratos HTTP server 提供 Web 管理界面与 REST API
//   - cron 调度在 app 启动前注册、退出时优雅停止
//   - 配置变更(界面保存)→ Apply():从数据库重载并重建调度
//
// runner/scheduler/产物空间/pusher 随配置重建,属动态依赖,不进静态 wire 图。
func newApp(
	boot *config.Bootstrap,
	d *data.Data,
	repo repository.Repository,
	cfg *repository.Config,
	secrets *config.Resolver,
	lg *klog.Helper,
) (*kratos.App, func(), error) {
	if d.Seeded() {
		lg.Infow("database_seeded")
	}
	lg.Infow("startup",
		"version", version,
		"db", boot.Database.Path,
		"log", boot.Log.Path,
		"storage", boot.Storage.BaseDir,
		"subscriptions", len(cfg.Subscriptions),
		"schema_version", d.MigrationResult().To,
	)

	// buildArtifacts 按配置构建产物空间(热重载时随配置重建)。
	buildArtifacts := func(c *repository.Config) (*artifact.Manager, error) {
		pass, err := secrets.StoragePassword(context.Background(), "")
		if err != nil {
			lg.Warnf("storage_password_resolve_failed err=%v", err)
			pass = ""
		}
		return artifact.Build(&c.Storage, boot.Storage.BaseDir, pass)
	}

	arts, err := buildArtifacts(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("初始化产物空间失败: %w", err)
	}

	var (
		mu      sync.Mutex
		curCfg  = cfg
		curArts = arts
		curRun  = runner.New(cfg, secrets, arts, boot.StateDir, lg)
		sched   *scheduler.Scheduler
	)

	logNexts := func(nexts []scheduler.NextRun) {
		for _, nr := range nexts {
			lg.Infow("schedule_registered", "topic", nr.Topic, "cron", nr.Cron, "next", nr.Next.Format(time.RFC3339))
		}
	}

	startScheduler := func(c *repository.Config, rn *runner.Runner) (*scheduler.Scheduler, []scheduler.NextRun, error) {
		s := scheduler.New(time.Duration(c.LLM.TimeoutSec) * time.Second)
		jobs := make([]scheduler.Job, 0, len(c.Subscriptions))
		for _, sub := range c.Subscriptions {
			jobs = append(jobs, scheduler.Job{
				Name:     sub.Topic,
				Schedule: sub.Cron,
				Run:      rn.RecordedJob(sub, repo, repository.RunTriggerSchedule),
			})
		}
		nexts, err := s.Start(jobs)
		if err != nil {
			return nil, nil, err
		}
		return s, nexts, nil
	}

	stopScheduler := func() {
		if sched != nil {
			<-sched.Stop().Done()
		}
	}

	s0, nexts, err := startScheduler(cfg, curRun)
	if err != nil {
		return nil, nil, fmt.Errorf("启动调度失败: %w", err)
	}
	sched = s0
	lg.Infow("scheduler_started", "subscriptions", len(cfg.Subscriptions), "rules", len(nexts))
	logNexts(nexts)

	// 监听地址取当前生效配置(来自数据库);变更后需重启进程生效
	addr := cfg.Server.Addr
	webEnabled := cfg.Server.IsEnabled()
	if webEnabled && addr == "" {
		stopScheduler()
		return nil, nil, fmt.Errorf("server.addr 未配置:请在 Web 界面全局设置中填写监听地址")
	}

	deps := server.Deps{
		Version:   version,
		StartedAt: time.Now(),
		Config:    func() *repository.Config { mu.Lock(); defer mu.Unlock(); return curCfg },
		NextRuns: func() []scheduler.NextRun {
			mu.Lock()
			defer mu.Unlock()
			if sched == nil {
				return nil
			}
			return sched.NextRuns()
		},
		Artifacts: func() *artifact.Manager { mu.Lock(); defer mu.Unlock(); return curArts },
		IsRunning: func(name string) bool { mu.Lock(); defer mu.Unlock(); return curRun.IsRunning(name) },
		Trigger: func(topic string) error {
			mu.Lock()
			c, rn := curCfg, curRun
			mu.Unlock()
			sub := c.SubscriptionByTopic(topic)
			if sub == nil {
				return errSubscriptionNotFound(topic)
			}
			if rn.IsRunning(topic) {
				return errSubscriptionRunning(topic)
			}
			// 后台执行:立即返回,进度通过运行记录与订阅页查看
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.LLM.TimeoutSec)*time.Second)
				defer cancel()
				if err := rn.RecordedJob(*sub, repo, repository.RunTriggerManual)(ctx); err != nil {
					lg.Warnf("sub_run_failed topic=%s trigger=manual err=%v", topic, err)
				}
			}()
			return nil
		},
		TestTarget: func(name string) error {
			mu.Lock()
			c, arts := curCfg, curArts
			mu.Unlock()
			t := c.TargetByName(name)
			if t == nil {
				return errTargetNotFound(name)
			}
			pass, _ := secrets.SMTPPassword(context.Background())
			p, err := pusher.New(t, &c.Mailer, pass, arts)
			if err != nil {
				return err
			}
			_, err = p.Push(context.Background(), &pusher.Message{
				Topic:   "test",
				Subject: "InfoSentinel 测试邮件",
				Body:    "这是一封来自 InfoSentinel Web 管理界面的测试邮件。\n\n收到它说明该推送目标配置正确。",
			})
			return err
		},
		TestLLM: func(ctx context.Context, llm repository.LLMConfig, apiKey string) (string, error) {
			// 表单值不落库:用临时配置构建 ChatModel 并发送一次最小请求
			if llm.TimeoutSec <= 0 {
				llm.TimeoutSec = 120
			}
			key, err := secrets.ResolveWith(ctx, config.NameLLMAPIKey, "模型 API Key", apiKey, true)
			if err != nil {
				return "", err
			}
			cm, err := agent.NewChatModel(ctx, &repository.Config{LLM: llm}, key)
			if err != nil {
				return "", err
			}
			ctx, cancel := context.WithTimeout(ctx, time.Duration(llm.TimeoutSec)*time.Second)
			defer cancel()
			reply, err := cm.Generate(ctx, []*schema.Message{
				{Role: schema.User, Content: "这是连通性测试,请只回复:OK"},
			})
			if err != nil {
				return "", fmt.Errorf("模型请求失败: %w", err)
			}
			if reply == nil {
				return "", fmt.Errorf("模型未返回内容")
			}
			text := strings.TrimSpace(reply.Content)
			if runed := []rune(text); len(runed) > 120 {
				text = string(runed[:120]) + "…"
			}
			return fmt.Sprintf("模型连通正常,回复:%s", text), nil
		},
		TestMailer: func(ctx context.Context, mcfg repository.MailerConfig, password, to string) (string, error) {
			if err := mcfg.Validate(); err != nil {
				return "", err
			}
			mu.Lock()
			c, arts := curCfg, curArts
			mu.Unlock()
			if to == "" {
				for _, tg := range c.Targets {
					if tg.Type == "email" {
						to = tg.To
						break
					}
				}
			}
			if to == "" {
				return "", fmt.Errorf("未指定测试收件人:请填写收件邮箱或先配置一个 email 推送目标")
			}
			// 表单密钥优先,其次已保存密文;站内信模式不要求密码
			pass, err := secrets.ResolveWith(ctx, config.NameSMTPPassword, "SMTP 密码/授权码", password, mcfg.Type == repository.MailerTypeSMTP)
			if err != nil {
				return "", err
			}
			p, err := pusher.New(&repository.Target{Name: "test", Type: "email", To: to}, &mcfg, pass, arts)
			if err != nil {
				return "", err
			}
			return p.Push(ctx, &pusher.Message{
				Topic:   "test",
				Subject: "InfoSentinel 发送器测试邮件",
				Body:    "这是一封来自 InfoSentinel Web 管理界面的**发送器测试邮件**。\n\n收到它说明全局发送器配置正确。",
			})
		},
		Apply: func() error {
			c, err := repo.LoadConfig(context.Background())
			if err != nil {
				return err
			}
			arts, err := buildArtifacts(c)
			if err != nil {
				return fmt.Errorf("初始化产物空间失败: %w", err)
			}
			mu.Lock()
			old := sched
			mu.Unlock()
			if old != nil {
				<-old.Stop().Done() // 等待运行中的任务结束
			}
			rn := runner.New(c, secrets, arts, boot.StateDir, lg)
			newS, nx, err := startScheduler(c, rn)
			if err != nil {
				return err
			}
			mu.Lock()
			curCfg, curRun, curArts, sched = c, rn, arts, newS
			mu.Unlock()
			lg.Infow("scheduler_applied", "subscriptions", len(c.Subscriptions), "rules", len(nx))
			logNexts(nx)
			return nil
		},
		Repo: repo,
		Log:  lg,
	}

	opts := []kratos.Option{
		kratos.Name("infosentinel"),
		kratos.Version(version),
		kratos.Logger(lg.Logger()),
		kratos.AfterStop(func(context.Context) error {
			stopScheduler()
			lg.Infow("shutdown")
			return nil
		}),
	}
	if webEnabled {
		opts = append(opts, kratos.Server(server.New(deps, addr)))
		lg.Infow("web_started", "addr", addr)
	} else {
		// 无 HTTP 界面时仍保持进程常驻,仅调度任务;kratos 自带信号等待
		lg.Infow("web_disabled")
	}

	return kratos.New(opts...), func() {}, nil
}
