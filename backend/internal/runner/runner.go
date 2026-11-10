// Package runner 编排单个订阅的执行:加载订阅级去重状态 →
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
	"github.com/tomasgdl/infosentinel/backend/internal/artifact"
	"github.com/tomasgdl/infosentinel/backend/internal/config"
	"github.com/tomasgdl/infosentinel/backend/internal/pusher"
	"github.com/tomasgdl/infosentinel/backend/internal/repository"
	"github.com/tomasgdl/infosentinel/backend/pkg/logger"
	"github.com/tomasgdl/infosentinel/backend/pkg/tools"
)

// Runner 是订阅执行器,持有全局配置、敏感数据解析器、产物空间与日志。
type Runner struct {
	cfg       *repository.Config
	secrets   *config.Resolver
	artifacts *artifact.Manager
	stateDir  string // 去重状态目录(引导配置),每个订阅一个 <topic>.json
	log       *klog.Helper

	mu      sync.Mutex
	running map[string]struct{} // 正在执行的订阅主题(防并发守卫)
}

// New 创建订阅执行器;artifacts 为产物空间(站内信投递依赖,可为 nil)。
func New(cfg *repository.Config, secrets *config.Resolver, artifacts *artifact.Manager, stateDir string, log *klog.Helper) *Runner {
	if log == nil {
		log = logger.NewDiscard()
	}
	return &Runner{cfg: cfg, secrets: secrets, artifacts: artifacts, stateDir: stateDir, log: log, running: make(map[string]struct{})}
}

// IsRunning 返回订阅是否正在执行。
func (r *Runner) IsRunning(topic string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.running[topic]
	return ok
}

// withSlot 占用订阅的执行槽位后运行 fn;同订阅已有实例在跑时直接报错。
func (r *Runner) withSlot(topic string, fn func() error) error {
	r.mu.Lock()
	if _, ok := r.running[topic]; ok {
		r.mu.Unlock()
		return fmt.Errorf("订阅 %q 正在运行中,请稍后再试", topic)
	}
	r.running[topic] = struct{}{}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.running, topic)
		r.mu.Unlock()
	}()
	return fn()
}

// RecordedJob 返回带防并发守卫与运行记录的订阅执行函数,
// 供 scheduler 定时触发与 web 手动触发共用。
// runs 为 nil 时只做防并发守卫,不记录运行历史。
func (r *Runner) RecordedJob(sub repository.Subscription, runs repository.RunRepository, trigger string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return r.withSlot(sub.Topic, func() error {
			var runID int64
			if runs != nil {
				id, err := runs.StartRun(ctx, sub.Topic, trigger)
				if err != nil {
					r.log.Warnf("run_record_start_failed topic=%s err=%v", sub.Topic, err)
				} else {
					runID = id
				}
			}
			digest, err := r.RunTask(ctx, sub)
			if runs != nil && runID > 0 {
				var ferr error
				if err != nil {
					ferr = runs.FinishRun(ctx, runID, err, "", "")
				} else {
					ferr = runs.FinishRun(ctx, runID, nil, digest.Subject, digest.Body)
				}
				if ferr != nil {
					r.log.Warnf("run_record_finish_failed topic=%s err=%v", sub.Topic, ferr)
				}
			}
			return err
		})
	}
}

// RunTask 执行一个订阅:采集 → 总结 → 推送到全部目标,返回生成的报告。
func (r *Runner) RunTask(ctx context.Context, sub repository.Subscription) (*agent.Digest, error) {
	start := time.Now()
	topic := r.cfg.TopicByName(sub.Topic)
	if topic == nil {
		return nil, fmt.Errorf("订阅引用的主题 %q 不存在", sub.Topic)
	}

	// 订阅级去重状态:订阅之间互不影响
	statePath := filepath.Join(r.stateDir, sub.Topic+".json")
	store, err := tools.NewSeenStore(statePath, r.cfg.Sources.Collect.DedupDays)
	if err != nil {
		return nil, fmt.Errorf("加载去重状态: %w", err)
	}

	toolList, channels, err := agent.BuildTools(&r.cfg.Sources, store)
	if err != nil {
		return nil, fmt.Errorf("构建工具集: %w", err)
	}

	apiKey, err := r.secrets.LLMAPIKey(ctx)
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
	smtpPassword, _ := r.secrets.SMTPPassword(ctx) // 非 smtp 模式可为空
	var pushErrs []error
	for _, ref := range sub.Targets {
		target := r.cfg.TargetByName(ref)
		if target == nil {
			pushErrs = append(pushErrs, fmt.Errorf("推送目标 %q 不存在", ref))
			continue
		}
		p, err := pusher.New(target, &r.cfg.Mailer, smtpPassword, r.artifacts)
		if err != nil {
			pushErrs = append(pushErrs, err)
			continue
		}
		if _, err := p.Push(ctx, &pusher.Message{
			Topic:   sub.Topic,
			Subject: digest.Subject,
			Body:    digest.Body,
		}); err != nil {
			pushErrs = append(pushErrs, fmt.Errorf("推送到 %q: %w", ref, err))
		}
	}

	// 全部推送成功才持久化去重状态:失败重跑不会漏内容
	if len(pushErrs) == 0 {
		if err := store.Save(time.Now()); err != nil {
			r.log.Warnf("sub_state_save_failed err=%v", err)
		}
		r.log.Infow(
			"sub_run_done",
			logger.KeyTopic, sub.Topic,
			logger.KeyDuration, time.Since(start).Round(time.Millisecond).String(),
			logger.KeyTargets, len(sub.Targets),
			logger.KeyStatus, "success",
		)
		return digest, nil
	}
	return digest, fmt.Errorf("订阅 %q 推送未全部成功: %w", sub.Topic, joinErrors(pushErrs))
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
