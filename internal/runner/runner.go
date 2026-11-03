// Package runner 编排单个任务的执行:加载任务级去重状态 →
// 从公用数据源构建工具 → 按主题创建 agent → 生成报告 → 推送到全部目标 → 持久化状态。
package runner

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/tomasgdl/infosentinel/internal/agent"
	"github.com/tomasgdl/infosentinel/internal/config"
	"github.com/tomasgdl/infosentinel/internal/pusher"
	"github.com/tomasgdl/infosentinel/internal/tools"
)

// Runner 是任务执行器,持有全局配置。
type Runner struct {
	cfg *config.Config
}

// New 创建任务执行器。
func New(cfg *config.Config) *Runner {
	return &Runner{cfg: cfg}
}

// TaskJob 返回任务的可调度执行函数(供 scheduler 与 run 子命令共用)。
func (r *Runner) TaskJob(task config.Task) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return r.RunTask(ctx, task)
	}
}

// RunTask 执行一个任务:采集 → 总结 → 推送到全部目标。
func (r *Runner) RunTask(ctx context.Context, task config.Task) error {
	start := time.Now()
	topic := r.cfg.TopicByName(task.Topic)
	if topic == nil {
		return fmt.Errorf("任务 %q 引用的主题 %q 不存在", task.Name, task.Topic)
	}

	// 任务级去重状态:任务之间互不影响
	statePath := filepath.Join(r.cfg.Sources.Collect.StateDir, task.Name+".json")
	store, err := tools.NewSeenStore(statePath, r.cfg.Sources.Collect.DedupDays)
	if err != nil {
		return fmt.Errorf("加载去重状态: %w", err)
	}

	toolList, channels, err := agent.BuildTools(&r.cfg.Sources, store)
	if err != nil {
		return fmt.Errorf("构建工具集: %w", err)
	}

	a, err := agent.New(ctx, r.cfg, topic, toolList, channels)
	if err != nil {
		return fmt.Errorf("创建 agent: %w", err)
	}
	digest, err := a.Run(ctx)
	if err != nil {
		return err
	}

	// 推送到全部目标:单个目标失败不影响其他目标,最后汇总报错
	var pushErrs []error
	for _, ref := range task.Targets {
		target := r.cfg.TargetByName(ref)
		if target == nil {
			pushErrs = append(pushErrs, fmt.Errorf("推送目标 %q 不存在", ref))
			continue
		}
		p, err := pusher.New(target, &r.cfg.Mailer)
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
			log.Printf("[task %s] 保存去重状态失败(不影响本轮推送): %v", task.Name, err)
		}
		log.Printf("[task %s] 完成,耗时 %s,推送到 %d 个目标", task.Name, time.Since(start).Round(time.Second), len(task.Targets))
		return nil
	}
	return fmt.Errorf("任务 %q 推送未全部成功: %w", task.Name, joinErrors(pushErrs))
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
