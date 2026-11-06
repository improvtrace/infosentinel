// provider.go 提供 wire 依赖注入的应用组装 Provider:
// newConfig 从数据库加载生效配置,newApp 组装 serve 常驻应用(kratos app)。
package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2"
	klog "github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"

	"github.com/tomasgdl/infosentinel/backend/internal/config"
	"github.com/tomasgdl/infosentinel/backend/internal/history"
	"github.com/tomasgdl/infosentinel/backend/internal/pusher"
	"github.com/tomasgdl/infosentinel/backend/internal/repository"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data"
	"github.com/tomasgdl/infosentinel/backend/internal/runner"
	"github.com/tomasgdl/infosentinel/backend/internal/scheduler"
	"github.com/tomasgdl/infosentinel/backend/internal/secret"
	"github.com/tomasgdl/infosentinel/backend/internal/server"
)

// cliProviderSet 是 CLI 子命令的 wire ProviderSet。
var cliProviderSet = wire.NewSet(newCLIApp)

// appProviderSet 是 serve 应用的 wire ProviderSet。
var appProviderSet = wire.NewSet(newConfig, newApp)

// newCLIApp 聚合 CLI 子命令(run/list/migrate)所需依赖。
func newCLIApp(boot *config.Bootstrap, d *data.Data, lg *klog.Helper, secrets *secret.Resolver) *cliApp {
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
// runner/scheduler/pusher 随配置重建,属动态依赖,不进静态 wire 图。
func newApp(
	boot *config.Bootstrap,
	d *data.Data,
	repo repository.Repository,
	cfg *repository.Config,
	secrets *secret.Resolver,
	lg *klog.Helper,
) (*kratos.App, func(), error) {
	if d.Seeded() {
		lg.Infow("database_seeded")
	}
	lg.Infow("startup",
		"version", version,
		"db", boot.Database.Path,
		"log", boot.Log.Path,
		"tasks", len(cfg.Tasks),
		"schema_version", d.MigrationResult().To,
	)

	hist := history.New(0)

	var (
		mu     sync.Mutex
		curCfg = cfg
		curRun = runner.New(cfg, secrets, lg)
		sched  *scheduler.Scheduler
	)

	logNexts := func(nexts []scheduler.NextRun) {
		for _, nr := range nexts {
			lg.Infow("schedule_registered", "task", nr.Task, "cron", nr.Schedule, "next", nr.Next.Format(time.RFC3339))
		}
	}

	startScheduler := func(c *repository.Config, rn *runner.Runner) (*scheduler.Scheduler, []scheduler.NextRun, error) {
		s := scheduler.New(time.Duration(c.LLM.TimeoutSec) * time.Second)
		jobs := make([]scheduler.Job, 0, len(c.Tasks))
		for _, t := range c.Tasks {
			jobs = append(jobs, scheduler.Job{
				Name:      t.Name,
				Schedules: t.Schedules,
				Run:       rn.RecordedJob(t, hist, history.TriggerSchedule),
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
	lg.Infow("scheduler_started", "tasks", len(cfg.Tasks), "rules", len(nexts))
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
		History:   hist,
		IsRunning: func(name string) bool { mu.Lock(); defer mu.Unlock(); return curRun.IsRunning(name) },
		Trigger: func(name string) error {
			mu.Lock()
			c, rn := curCfg, curRun
			mu.Unlock()
			t := c.TaskByName(name)
			if t == nil {
				return errTaskNotFound(name)
			}
			if rn.IsRunning(name) {
				return errTaskRunning(name)
			}
			// 后台执行:立即返回,进度通过历史记录与任务页查看
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.LLM.TimeoutSec)*time.Second)
				defer cancel()
				if err := rn.RecordedJob(*t, hist, history.TriggerManual)(ctx); err != nil {
					lg.Warnf("task_run_failed task=%s trigger=manual err=%v", name, err)
				}
			}()
			return nil
		},
		TestTarget: func(name string) error {
			mu.Lock()
			c := curCfg
			mu.Unlock()
			t := c.TargetByName(name)
			if t == nil {
				return errTargetNotFound(name)
			}
			pass, _ := secrets.SMTPPassword(context.Background(), c)
			p, err := pusher.New(t, &c.Mailer, pass)
			if err != nil {
				return err
			}
			return p.Push(context.Background(), &pusher.Message{
				Task:    "test",
				Subject: "InfoSentinel 测试邮件",
				Body:    "这是一封来自 InfoSentinel Web 管理界面的测试邮件。\n\n收到它说明该推送目标配置正确。",
			})
		},
		Apply: func() error {
			c, err := repo.LoadConfig(context.Background())
			if err != nil {
				return err
			}
			mu.Lock()
			old := sched
			mu.Unlock()
			if old != nil {
				<-old.Stop().Done() // 等待运行中的任务结束
			}
			rn := runner.New(c, secrets, lg)
			newS, nx, err := startScheduler(c, rn)
			if err != nil {
				return err
			}
			mu.Lock()
			curCfg, curRun, sched = c, rn, newS
			mu.Unlock()
			lg.Infow("scheduler_applied", "tasks", len(c.Tasks), "rules", len(nx))
			logNexts(nx)
			return nil
		},
		Repo:      repo,
		EmailsDir: func() string { mu.Lock(); defer mu.Unlock(); return curCfg.Mailer.OutputDir },
		Log:       lg,
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
