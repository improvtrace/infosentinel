// Package runner 编排单个订阅的执行:整体意图分组 → 逐组加载去重状态并采集
// (每组一个子任务)→ 按合成提示词把各组报告合并为一份 → 推送到全部目标 → 持久化状态。
package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
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

// Runner 是订阅执行器,持有全局配置、敏感数据解析器、产物空间集合与日志。
type Runner struct {
	cfg      *repository.Config
	secrets  *config.Resolver
	spaces   *artifact.Spaces
	stateDir string // 去重状态目录(引导配置),每个主题一个 <topic>.json
	log      *klog.Helper

	mu      sync.Mutex
	running map[string]struct{} // 正在执行的订阅与主题键(防并发守卫)
}

// New 创建订阅执行器;spaces 为产物空间集合(站内信投递依赖,可为空集合)。
func New(cfg *repository.Config, secrets *config.Resolver, spaces *artifact.Spaces, stateDir string, log *klog.Helper) *Runner {
	if log == nil {
		log = logger.NewDiscard()
	}
	return &Runner{cfg: cfg, secrets: secrets, spaces: spaces, stateDir: stateDir, log: log, running: make(map[string]struct{})}
}

// IsRunning 返回订阅是否正在执行。
func (r *Runner) IsRunning(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.running[name]
	return ok
}

// withSlot 占用全部键对应的执行槽位后运行 fn;任一键已有实例在跑时直接报错。
// 键 = 订阅名 + 关联主题名:同订阅防重入,同主题跨订阅互斥(去重状态文件共享)。
func (r *Runner) withSlot(keys []string, fn func() error) error {
	r.mu.Lock()
	for _, k := range keys {
		if _, ok := r.running[k]; ok {
			r.mu.Unlock()
			return fmt.Errorf("%q 正在运行中,请稍后再试", k)
		}
	}
	for _, k := range keys {
		r.running[k] = struct{}{}
	}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		for _, k := range keys {
			delete(r.running, k)
		}
		r.mu.Unlock()
	}()
	return fn()
}

// RecordedJob 返回带防并发守卫与运行记录的订阅执行函数,
// 供 scheduler 定时触发与 web 手动触发共用。
// runs 为 nil 时只做防并发守卫,不记录运行历史。
func (r *Runner) RecordedJob(sub repository.Subscription, runs repository.RunRepository, trigger string) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		return r.withSlot(slotKeys(sub), func() error {
			var runID int64
			if runs != nil {
				id, err := runs.StartRun(ctx, sub.Name, trigger)
				if err != nil {
					r.log.Warnf("run_record_start_failed subscription=%s err=%v", sub.Name, err)
				} else {
					runID = id
				}
			}
			digest, artifacts, err := r.RunTask(ctx, sub)
			if runs != nil && runID > 0 {
				var ferr error
				if err != nil {
					ferr = runs.FinishRun(ctx, runID, err, "", "", nil)
				} else {
					ferr = runs.FinishRun(ctx, runID, nil, digest.Subject, digest.Body, artifacts)
				}
				if ferr != nil {
					r.log.Warnf("run_record_finish_failed subscription=%s err=%v", sub.Name, ferr)
				}
			}
			return err
		})
	}
}

// slotKeys 返回订阅执行的防并发键:订阅名 + 全部关联主题名。
func slotKeys(sub repository.Subscription) []string {
	keys := make([]string, 0, len(sub.Topics)+1)
	keys = append(keys, sub.Name)
	keys = append(keys, sub.Topics...)
	return keys
}

// RunTask 执行一个订阅:整体意图分组 → 逐组采集 → 合成 → 推送到全部目标,
// 返回最终报告与写入的产物路径(DSN 列表)。
func (r *Runner) RunTask(ctx context.Context, sub repository.Subscription) (*agent.Digest, []string, error) {
	start := time.Now()

	apiKey, err := r.secrets.LLMAPIKey(ctx)
	if err != nil {
		return nil, nil, err
	}

	// 解析订阅引用的全部主题(引用不完整直接失败)
	topics := make([]repository.Topic, 0, len(sub.Topics))
	for _, ref := range sub.Topics {
		topic := r.cfg.TopicByName(ref)
		if topic == nil {
			return nil, nil, fmt.Errorf("订阅引用的主题 %q 不存在", ref)
		}
		topics = append(topics, *topic)
	}

	// 意图分组:按采集意图相近程度把主题划为若干子任务组;分析不可用时
	// 回退为一主题一组(逐主题采集),保证流程总能走通
	groups, err := agent.PlanGroups(ctx, r.cfg, apiKey, topics)
	if err != nil {
		r.log.Warnf("intent_plan_failed subscription=%s err=%v;回退为逐主题采集", sub.Name, err)
		groups = singleTopicGroups(topics)
	}
	for _, g := range groups {
		r.log.Infof("intent_group subscription=%s group=%q topics=%s",
			sub.Name, g.Name, strings.Join(topicNamesOf(g), ","))
	}

	// 逐组采集(子任务):组内主题共享一次采集与合并的去重状态,
	// 一组失败则本轮失败(重跑不会漏内容)
	stores := make([]*tools.SeenStore, 0, len(groups))
	parts := make([]*agent.Digest, 0, len(groups))
	for _, g := range groups {
		groupStart := time.Now()

		// 组级去重状态:合并组内全部主题的状态文件,同一主题跨订阅共享
		store, err := tools.NewSeenStoreMulti(groupStatePaths(r.stateDir, g), tools.DedupDays)
		if err != nil {
			return nil, nil, fmt.Errorf("加载去重状态: %w", err)
		}

		toolList, err := agent.BuildTools(store)
		if err != nil {
			return nil, nil, fmt.Errorf("构建工具集: %w", err)
		}
		a, err := agent.New(ctx, r.cfg, &g, apiKey, toolList)
		if err != nil {
			return nil, nil, fmt.Errorf("创建 agent: %w", err)
		}
		digest, err := a.Run(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("分组 %q(主题:%s)采集失败: %w",
				g.Name, strings.Join(topicNamesOf(g), ","), err)
		}
		stores = append(stores, store)
		parts = append(parts, digest)
		r.log.Infow(
			"group_run_done",
			logger.KeyTopic, sub.Name,
			"group", g.Name,
			logger.KeyDuration, time.Since(groupStart).Round(time.Millisecond).String(),
			logger.KeyCount, len(g.Topics),
			logger.KeyStatus, "success",
		)
	}

	// 按订阅的合成提示词把各组报告合并为一份(单组时原样返回)
	digest, err := agent.Synthesize(ctx, r.cfg, apiKey, sub.Synthesis, parts)
	if err != nil {
		return nil, nil, err
	}

	// 推送到全部目标:单个目标失败不影响其他目标,最后汇总报错;
	// 站内信模式收集写入的产物 DSN,记入运行记录
	smtpPassword, _ := r.secrets.SMTPPassword(ctx) // 非 smtp 模式可为空
	var pushErrs []error
	var artifacts []string
	for _, ref := range sub.Targets {
		target := r.cfg.TargetByName(ref)
		if target == nil {
			pushErrs = append(pushErrs, fmt.Errorf("推送目标 %q 不存在", ref))
			continue
		}
		p, err := pusher.New(target, &r.cfg.Mailer, smtpPassword, r.spaces, sub.Space)
		if err != nil {
			pushErrs = append(pushErrs, err)
			continue
		}
		res, err := p.Push(ctx, &pusher.Message{
			Subscription: sub.Name,
			Subject:      digest.Subject,
			Body:         digest.Body,
		})
		if err != nil {
			pushErrs = append(pushErrs, fmt.Errorf("推送到 %q: %w", ref, err))
			continue
		}
		artifacts = append(artifacts, res.Artifacts...)
	}

	// 全部推送成功才持久化去重状态:失败重跑不会漏内容
	if len(pushErrs) > 0 {
		return digest, artifacts, fmt.Errorf("订阅 %q 推送未全部成功: %w", sub.Name, joinErrors(pushErrs))
	}
	for _, store := range stores {
		if err := store.Save(time.Now()); err != nil {
			r.log.Warnf("sub_state_save_failed err=%v", err)
		}
	}
	r.log.Infow(
		"sub_run_done",
		logger.KeyTopic, sub.Name,
		logger.KeyDuration, time.Since(start).Round(time.Millisecond).String(),
		logger.KeyTargets, len(sub.Targets),
		logger.KeyStatus, "success",
	)
	return digest, artifacts, nil
}

// groupStatePaths 返回组内全部主题的去重状态文件路径(主题级,跨订阅共享)。
func groupStatePaths(stateDir string, g agent.Group) []string {
	paths := make([]string, 0, len(g.Topics))
	for _, t := range g.Topics {
		paths = append(paths, filepath.Join(stateDir, t.Name+".json"))
	}
	return paths
}

// singleTopicGroups 回退分组:一主题一组(意图分析不可用时的逐主题采集)。
func singleTopicGroups(topics []repository.Topic) []agent.Group {
	groups := make([]agent.Group, 0, len(topics))
	for _, t := range topics {
		groups = append(groups, agent.Group{Name: t.Name, Topics: []repository.Topic{t}})
	}
	return groups
}

// topicNamesOf 返回组内主题名列表(日志与错误信息用)。
func topicNamesOf(g agent.Group) []string {
	names := make([]string, 0, len(g.Topics))
	for _, t := range g.Topics {
		names = append(names, t.Name)
	}
	return names
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
