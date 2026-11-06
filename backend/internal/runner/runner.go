// Package runner 编排单个任务的执行:加载任务级去重状态 →
// 从公用数据源构建工具 → 按主题创建 agent → 生成报告 → 推送到全部目标 → 持久化状态。
package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	klog "github.com/go-kratos/kratos/v2/log"

	"github.com/tomasgdl/infosentinel/backend/internal/agent"
	"github.com/tomasgdl/infosentinel/backend/internal/history"
	"github.com/tomasgdl/infosentinel/backend/internal/logger"
	"github.com/tomasgdl/infosentinel/backend/internal/pusher"
	"github.com/tomasgdl/infosentinel/backend/internal/repository"
	"github.com/tomasgdl/infosentinel/backend/internal/secret"
	"github.com/tomasgdl/infosentinel/backend/internal/tools"
)

// Runner 是任务执行器,持有全局配置、敏感数据解析器与日志。
type Runner struct {
	cfg     *repository.Config
	secrets *secret.Resolver
	log     *klog.Helper

	mu      sync.Mutex
	running map[string]struct{} // 正在执行的任务名(防并发守卫)
}

// New 创建任务执行器。
func New(cfg *repository.Config, secrets *secret.Resolver, log *klog.Helper) *Runner {
	if log == nil {
		log = logger.NewDiscard()
	}
	return &Runner{cfg: cfg, secrets: secrets, log: log, running: make(map[string]struct{})}
}

// IsRunning 返回任务是否正在执行。
func (r *Runner) IsRunning(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.running[name]
	return ok
}

// withSlot 占用任务的执行槽位后运行 fn;同任务已有实例在跑时直接报错。
func (r *Runner) withSlot(name string, fn func() error) error {
	r.mu.Lock()
	if _, ok := r.running[name]; ok {
		r.mu.Unlock()
		return fmt.Errorf("任务 %q 正在运行中,请稍后再试", name)
	}
	r.running[name] = struct{}{}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.running, name)
		r.mu.Unlock()
	}()
	return fn()
}

// RecordedJob 返回带防并发守卫与历史记录的任务执行函数,
// 供 scheduler 定时触发与 web 手动触发共用。
// store 为 nil 时只做防并发守卫,不记录历史。
func (r *Runner) RecordedJob(task repository.Task, store *history.Store, trigger string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return r.withSlot(task.Name, func() error {
			var rec *history.Record
			if store != nil {
				rec = store.Start(task.Name, trigger)
			}
			digest, err := r.RunTask(ctx, task)
			if rec != nil {
				if err != nil {
					store.Finish(rec.ID, err, "", "")
				} else {
					store.Finish(rec.ID, nil, digest.Subject, digest.Body)
				}
			}
			return err
		})
	}
}

// RunTask 执行一个任务:采集 → 总结 → 推送到全部目标,返回生成的报告。
func (r *Runner) RunTask(ctx context.Context, task repository.Task) (*agent.Digest, error) {
	start := time.Now()
	topic := r.cfg.TopicByName(task.Topic)
	if topic == nil {
		return nil, fmt.Errorf("任务 %q 引用的主题 %q 不存在", task.Name, task.Topic)
	}

	// 任务级去重状态:任务之间互不影响
	statePath := filepath.Join(r.cfg.Sources.Collect.StateDir, task.Name+".json")
	store, err := tools.NewSeenStore(statePath, r.cfg.Sources.Collect.DedupDays)
	if err != nil {
		return nil, fmt.Errorf("加载去重状态: %w", err)
	}

	toolList, channels, err := agent.BuildTools(&r.cfg.Sources, store)
	if err != nil {
		return nil, fmt.Errorf("构建工具集: %w", err)
	}

	apiKey, err := r.secrets.LLMAPIKey(ctx, r.cfg)
	if err != nil {
		return nil, err
	}
	a, err := agent.New(ctx, r.cfg, topic, apiKey, toolList, channels)
	if err != nil {
		return nil, fmt.Errorf("创建 agent: %w", err)
	}
	digest, err := a.Run(ctx)
	if err != nil {
		return nil, err
	}

	// 推送到全部目标:单个目标失败不影响其他目标,最后汇总报错
	smtpPassword, _ := r.secrets.SMTPPassword(ctx, r.cfg) // 非 smtp 模式可为空
	var pushErrs []error
	for _, ref := range task.Targets {
		target := r.cfg.TargetByName(ref)
		if target == nil {
			pushErrs = append(pushErrs, fmt.Errorf("推送目标 %q 不存在", ref))
			continue
		}
		p, err := pusher.New(target, &r.cfg.Mailer, smtpPassword)
		if err != nil {
			pushErrs = append(pushErrs, err)
			continue
		}
		if err := p.Push(ctx, &pusher.Message{
			Task:    task.Name,
			Subject: digest.Subject,
			Body:    digest.Body,
		}); err != nil {
			pushErrs = append(pushErrs, fmt.Errorf("推送到 %q: %w", ref, err))
		}
	}

	// 全部推送成功才持久化去重状态:失败重跑不会漏内容
	if len(pushErrs) == 0 {
		if err := store.Save(time.Now()); err != nil {
			r.log.Warnf("task_state_save_failed err=%v", err)
		}
		r.log.Infow(
			"task_run_done",
			logger.KeyTask, task.Name,
			logger.KeyDuration, time.Since(start).Round(time.Millisecond).String(),
			logger.KeyTargets, len(task.Targets),
			logger.KeyStatus, "success",
		)
		return digest, nil
	}
	return digest, fmt.Errorf("任务 %q 推送未全部成功: %w", task.Name, joinErrors(pushErrs))
}

// joinErrors 合并多个错误。
func joinErrors(errs []error) error {
	if len(errs) == 1 {
		return errs[0]
	}
	msg := ""
	for i, e := range errs {
		if i > 0 {
			msg += "; "
		}
		msg += e.Error()
	}
	return fmt.Errorf("%s", msg)
}
