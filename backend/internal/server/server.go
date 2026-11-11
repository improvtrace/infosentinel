// Package server 是管理界面的 HTTP 传输层,基于 kratos transport/http:
//   - 路由使用 kratos Route DSL(分组 /api + 访问日志 Filter 埋点)
//   - 业务处理委托给 service 与 repository 接口,不含存储细节
//   - 静态资源(前端 SPA 构建产物)由 frontend.DistFS 托管,未命中路径回退 index.html
//
// REST 端点与语义见 .docs/development.md。
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	klog "github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"

	"github.com/tomasgdl/infosentinel/backend/internal/artifact"
	"github.com/tomasgdl/infosentinel/backend/internal/config"
	"github.com/tomasgdl/infosentinel/backend/internal/repository"
	"github.com/tomasgdl/infosentinel/backend/internal/scheduler"
	"github.com/tomasgdl/infosentinel/backend/pkg/logger"
	"github.com/tomasgdl/infosentinel/backend/pkg/storage"
	"github.com/tomasgdl/infosentinel/frontend"
)

// Deps 是界面后端依赖(全部以函数/接口注入,配置热重载后自动读取最新状态)。
type Deps struct {
	Version   string
	StartedAt time.Time
	Config    func() *repository.Config  // 当前生效配置
	NextRuns  func() []scheduler.NextRun // 各订阅的下次触发
	Artifacts func() *artifact.Spaces    // 产物空间集合(随配置热重载)
	IsRunning func(name string) bool     // 订阅是否在执行中
	Trigger   func(name string) error    // 手动触发订阅(后台执行)
	// TestTarget 向推送目标发送测试邮件。
	TestTarget func(name string) error
	// TestLLM 用表单值(不落库)测试模型配置:发一次最小对话请求。
	TestLLM func(ctx context.Context, cfg repository.LLMConfig, apiKey string) (string, error)
	// TestMailer 用表单值(不落库)测试发送器配置:发测试邮件/写测试产物。
	TestMailer func(ctx context.Context, cfg repository.MailerConfig, password, to string) (string, error)
	// Apply 从数据库重载配置并重建调度。
	Apply func() error
	// Repo 是配置存储(含运行记录)。
	Repo repository.Repository
	Log  *klog.Helper // 日志(访问埋点)
}

// Server 是管理界面后端。
type Server struct {
	d Deps
}

// New 创建 kratos HTTP Server 并注册全部路由与 SPA 托管。
func New(d Deps, addr string) *khttp.Server {
	if d.Log == nil {
		d.Log = logger.NewDiscard()
	}
	s := &Server{d: d}

	srv := khttp.NewServer(khttp.Address(addr), khttp.Filter(s.accessLog))
	api := srv.Route("/api")
	api.GET("/overview", s.overview)
	api.GET("/config", s.getConfig)

	api.GET("/topics", s.listTopics)
	api.POST("/topics", s.createTopic)
	api.PUT("/topics/{name}", s.updateTopic)
	api.DELETE("/topics/{name}", s.deleteTopic)

	api.GET("/targets", s.listTargets)
	api.POST("/targets", s.createTarget)
	api.PUT("/targets/{name}", s.updateTarget)
	api.DELETE("/targets/{name}", s.deleteTarget)
	api.POST("/targets/{name}/test", s.targetTestPost)

	api.GET("/subscriptions", s.listSubscriptions)
	api.POST("/subscriptions", s.createSubscription)
	api.PUT("/subscriptions/{name}", s.updateSubscription)
	api.DELETE("/subscriptions/{name}", s.deleteSubscription)
	api.POST("/subscriptions/{name}/run", s.runSubscriptionPost)

	api.GET("/settings", s.getSettings)
	api.PUT("/settings/{section}", s.putSettings)
	api.POST("/settings/llm/test", s.llmTestPost)
	api.POST("/settings/mailer/test", s.mailerTestPost)
	api.GET("/secrets", s.listSecrets)
	api.DELETE("/secrets/{name}", s.deleteSecret)
	api.POST("/apply", s.applyPost)

	api.GET("/runs", s.listRuns)
	api.GET("/runs/{id}", s.runDetail)
	api.GET("/artifacts", s.listArtifacts)
	api.GET("/artifacts/{space}/{bucket}/{name}", s.artifactContent)
	api.GET("/healthz", s.health)

	srv.HandlePrefix("/", s.spaHandler())
	return srv
}

// accessLog 是访问日志过滤器:为每个请求记录 method/path/code/duration 埋点。
func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rw, req)
		s.d.Log.Infow(
			"api_request",
			logger.KeyMethod, req.Method,
			logger.KeyPath, req.URL.Path,
			logger.KeyCode, rw.code,
			logger.KeyDuration, time.Since(start).Round(time.Microsecond).String(),
		)
	})
}

// statusWriter 捕获响应状态码。
type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) { w.code = code; w.ResponseWriter.WriteHeader(code) }

// ---- 通用辅助 ----

func writeJSON(ctx khttp.Context, status int, v any) error {
	return ctx.JSON(status, v)
}

func apiError(ctx khttp.Context, status int, err error) error {
	return ctx.JSON(status, map[string]string{"error": err.Error()})
}

// respondOK 写入成功结果并自动应用配置;应用失败不回滚持久化,以 warning 提示。
func (s *Server) respondOK(ctx khttp.Context) error {
	resp := map[string]any{"ok": true}
	if err := s.d.Apply(); err != nil {
		resp["applied"] = false
		resp["warning"] = "已保存,但应用失败:" + err.Error()
	} else {
		resp["applied"] = true
	}
	return ctx.JSON(http.StatusOK, resp)
}

func decode(r *http.Request, dst any) error {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("读取请求体: %w", err)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return fmt.Errorf("请求体为空")
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("请求体不是合法 JSON:%v", err)
	}
	return nil
}

// validName 校验实体名称:非空、无保留字、仅中英文/数字/下划线/连字符。
var (
	nameRe   = regexp.MustCompile(`^[\p{Han}\p{L}\p{N}_-]{1,64}$`)
	reserved = map[string]bool{"new": true, "edit": true}
)

func validName(name string) error {
	if name == "" {
		return fmt.Errorf("名称不能为空")
	}
	if reserved[name] {
		return fmt.Errorf("名称 %q 为保留字,请换一个", name)
	}
	if !nameRe.MatchString(name) {
		return fmt.Errorf("名称只能包含中英文、数字、下划线和连字符,长度 1-64")
	}
	return nil
}

func ctxParam(ctx khttp.Context, key string) string {
	return ctx.Vars().Get(key)
}

// ---- 概览 / 配置 ----

func (s *Server) overview(ctx khttp.Context) error {
	cfg := s.d.Config()
	runs, err := s.d.Repo.ListRuns(ctx, "", 200)
	if err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}

	running := 0
	for _, sub := range cfg.Subscriptions {
		if s.d.IsRunning(sub.Name) {
			running++
		}
	}
	ok, fail := 0, 0
	for _, rr := range runs {
		switch rr.Status {
		case repository.RunStatusSuccess:
			ok++
		case repository.RunStatusFailed:
			fail++
		}
	}
	recent := runs
	if len(recent) > 20 {
		recent = recent[:20]
	}

	return writeJSON(ctx, http.StatusOK, map[string]any{
		"version":       s.d.Version,
		"uptime_sec":    time.Since(s.d.StartedAt).Seconds(),
		"subscriptions": len(cfg.Subscriptions),
		"running":       running,
		"ok":            ok,
		"fail":          fail,
		"nexts":         s.d.NextRuns(),
		"recent_runs":   recent,
		"targets":       cfg.Targets,
		"mailer_type":   cfg.Mailer.Type,
	})
}

func (s *Server) getConfig(ctx khttp.Context) error {
	return writeJSON(ctx, http.StatusOK, s.d.Config())
}

func (s *Server) health(ctx khttp.Context) error {
	cfg := s.d.Config()
	running := 0
	for _, sub := range cfg.Subscriptions {
		if s.d.IsRunning(sub.Name) {
			running++
		}
	}
	runs, _ := s.d.Repo.CountRuns(ctx)
	return writeJSON(ctx, http.StatusOK, map[string]any{
		"version":       s.d.Version,
		"uptime":        time.Since(s.d.StartedAt).Round(time.Second).String(),
		"subscriptions": len(cfg.Subscriptions),
		"running":       running,
		"history":       runs,
	})
}

// ---- 主题 ----

type topicOut struct {
	repository.Topic
	UsedBy int `json:"used_by"`
}

func (s *Server) listTopics(ctx khttp.Context) error {
	items, err := s.d.Repo.ListTopics(ctx)
	if err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	out := make([]topicOut, 0, len(items))
	for _, t := range items {
		used, _ := s.d.Repo.UsedByTopic(ctx, t.Name)
		out = append(out, topicOut{Topic: t, UsedBy: used})
	}
	return writeJSON(ctx, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) createTopic(ctx khttp.Context) error {
	var t repository.Topic
	if err := decode(ctx.Request(), &t); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	if err := validName(t.Name); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	if strings.TrimSpace(t.Prompt) == "" {
		return apiError(ctx, http.StatusBadRequest, fmt.Errorf("prompt 不能为空:主题必须是一段 prompt"))
	}
	if err := s.d.Repo.UpsertTopic(ctx, t); err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	return s.respondOK(ctx)
}

func (s *Server) updateTopic(ctx khttp.Context) error {
	name := ctxParam(ctx, "name")
	var in struct {
		Prompt string `json:"prompt"`
	}
	if err := decode(ctx.Request(), &in); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	if _, err := s.d.Repo.GetTopic(ctx, name); err != nil {
		return apiError(ctx, http.StatusNotFound, fmt.Errorf("主题 %q 不存在", name))
	}
	if strings.TrimSpace(in.Prompt) == "" {
		return apiError(ctx, http.StatusBadRequest, fmt.Errorf("prompt 不能为空:主题必须是一段 prompt"))
	}
	if err := s.d.Repo.UpsertTopic(ctx, repository.Topic{Name: name, Prompt: in.Prompt}); err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	return s.respondOK(ctx)
}

func (s *Server) deleteTopic(ctx khttp.Context) error {
	if err := s.d.Repo.DeleteTopic(ctx, ctxParam(ctx, "name")); err != nil {
		return apiError(ctx, http.StatusConflict, err)
	}
	return s.respondOK(ctx)
}

// ---- 推送目标 ----

type targetOut struct {
	repository.Target
	UsedBy int `json:"used_by"`
}

func (s *Server) listTargets(ctx khttp.Context) error {
	items, err := s.d.Repo.ListTargets(ctx)
	if err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	out := make([]targetOut, 0, len(items))
	for _, t := range items {
		used, _ := s.d.Repo.UsedByTarget(ctx, t.Name)
		out = append(out, targetOut{Target: t, UsedBy: used})
	}
	return writeJSON(ctx, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) createTarget(ctx khttp.Context) error {
	var t repository.Target
	if err := decode(ctx.Request(), &t); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	if err := validName(t.Name); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	if t.Type == "" {
		t.Type = "email"
	}
	if strings.TrimSpace(t.To) == "" {
		return apiError(ctx, http.StatusBadRequest, fmt.Errorf("收件邮箱(to)不能为空"))
	}
	if err := s.d.Repo.UpsertTarget(ctx, t); err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	return s.respondOK(ctx)
}

func (s *Server) updateTarget(ctx khttp.Context) error {
	name := ctxParam(ctx, "name")
	old, err := s.d.Repo.GetTarget(ctx, name)
	if err != nil {
		return apiError(ctx, http.StatusNotFound, fmt.Errorf("推送目标 %q 不存在", name))
	}
	var in repository.Target
	if err := decode(ctx.Request(), &in); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	in.Name = old.Name // 名称不可改
	if in.Type == "" {
		in.Type = old.Type
	}
	if strings.TrimSpace(in.To) == "" {
		return apiError(ctx, http.StatusBadRequest, fmt.Errorf("收件邮箱(to)不能为空"))
	}
	if err := s.d.Repo.UpsertTarget(ctx, in); err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	return s.respondOK(ctx)
}

func (s *Server) deleteTarget(ctx khttp.Context) error {
	if err := s.d.Repo.DeleteTarget(ctx, ctxParam(ctx, "name")); err != nil {
		return apiError(ctx, http.StatusConflict, err)
	}
	return s.respondOK(ctx)
}

func (s *Server) targetTestPost(ctx khttp.Context) error {
	if err := s.d.TestTarget(ctxParam(ctx, "name")); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	return writeJSON(ctx, http.StatusOK, map[string]any{"ok": true, "sent": true})
}

// ---- 订阅 ----

func (s *Server) listSubscriptions(ctx khttp.Context) error {
	items, err := s.d.Repo.ListSubscriptions(ctx)
	if err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	return writeJSON(ctx, http.StatusOK, map[string]any{"items": items})
}

// saveSubscription 是创建/更新订阅的公共路径:整份配置校验(Finalize)→ 落库 → 应用。
func (s *Server) saveSubscription(ctx khttp.Context, sub repository.Subscription, isNew bool) error {
	if isNew {
		if err := validName(sub.Name); err != nil {
			return apiError(ctx, http.StatusBadRequest, err)
		}
		if strings.TrimSpace(sub.Synthesis) == "" {
			sub.Synthesis = repository.DefaultSynthesis // 合成提示词缺省回填
		}
	}
	cur := *s.d.Config()
	replaced := false
	subs := make([]repository.Subscription, 0, len(cur.Subscriptions)+1)
	for _, e := range cur.Subscriptions {
		if e.Name == sub.Name {
			subs = append(subs, sub)
			replaced = true
		} else {
			subs = append(subs, e)
		}
	}
	if !replaced {
		subs = append(subs, sub)
	}
	cur.Subscriptions = subs
	if err := cur.Finalize(); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	if err := s.d.Repo.UpsertSubscription(ctx, sub); err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	return s.respondOK(ctx)
}

func (s *Server) createSubscription(ctx khttp.Context) error {
	var sub repository.Subscription
	if err := decode(ctx.Request(), &sub); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	return s.saveSubscription(ctx, sub, true)
}

func (s *Server) updateSubscription(ctx khttp.Context) error {
	name := ctxParam(ctx, "name")
	if _, err := s.d.Repo.GetSubscription(ctx, name); err != nil {
		return apiError(ctx, http.StatusNotFound, fmt.Errorf("订阅 %q 不存在", name))
	}
	var in repository.Subscription
	if err := decode(ctx.Request(), &in); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	in.Name = name // 名称即订阅身份,不可改
	return s.saveSubscription(ctx, in, false)
}

func (s *Server) deleteSubscription(ctx khttp.Context) error {
	if err := s.d.Repo.DeleteSubscription(ctx, ctxParam(ctx, "name")); err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	return s.respondOK(ctx)
}

func (s *Server) runSubscriptionPost(ctx khttp.Context) error {
	if err := s.d.Trigger(ctxParam(ctx, "name")); err != nil {
		return apiError(ctx, http.StatusConflict, err)
	}
	return writeJSON(ctx, http.StatusOK, map[string]any{"ok": true, "started": true})
}

// ---- 设置 / 敏感数据 ----

func (s *Server) getSettings(ctx khttp.Context) error {
	cfg := s.d.Config()
	secrets, _ := s.d.Repo.ListSecrets(ctx)
	return writeJSON(ctx, http.StatusOK, map[string]any{
		"llm": cfg.LLM, "mailer": cfg.Mailer, "server": cfg.Server, "storage": cfg.Storage,
		"secrets": secrets,
	})
}

// putSettings 保存设置分区。请求体为分区配置,附带可选 secret(该分区密钥的明文,
// 非空则随本次保存一并加密落库,留空不动已存密钥)。
func (s *Server) putSettings(ctx khttp.Context) error {
	section := ctxParam(ctx, "section")
	cur := *s.d.Config()

	var secretName, secret string
	var persist func() error
	var persistSecrets func() error // 多密钥分区(如 storage 的逐空间密码)
	switch section {
	case "llm":
		var in struct {
			repository.LLMConfig
			Secret string `json:"secret"`
		}
		if err := decode(ctx.Request(), &in); err != nil {
			return apiError(ctx, http.StatusBadRequest, err)
		}
		cur.LLM = in.LLMConfig
		secretName, secret = "llm.api_key", in.Secret
		persist = func() error { return s.d.Repo.PutSetting(ctx, "llm", in.LLMConfig) }
	case "mailer":
		var in struct {
			repository.MailerConfig
			Secret string `json:"secret"`
		}
		if err := decode(ctx.Request(), &in); err != nil {
			return apiError(ctx, http.StatusBadRequest, err)
		}
		cur.Mailer = in.MailerConfig
		secretName, secret = "smtp.password", in.Secret
		persist = func() error { return s.d.Repo.PutSetting(ctx, "mailer", in.MailerConfig) }
	case "server":
		var in repository.ServerConfig
		if err := decode(ctx.Request(), &in); err != nil {
			return apiError(ctx, http.StatusBadRequest, err)
		}
		cur.Server = in
		persist = func() error { return s.d.Repo.PutSetting(ctx, "server", in) }
	case "storage":
		var in struct {
			repository.StorageConfig
			// Secrets 是逐空间密码表:键为空间名,值为明文密码(非空则加密落库为
			// storage.<空间名>.password);留空的键不动已存密钥。
			Secrets map[string]string `json:"secrets"`
		}
		if err := decode(ctx.Request(), &in); err != nil {
			return apiError(ctx, http.StatusBadRequest, err)
		}
		cur.Storage = in.StorageConfig
		persist = func() error { return s.d.Repo.PutSetting(ctx, "storage", in.StorageConfig) }
		persistSecrets = func() error {
			for name, pass := range in.Secrets {
				if strings.TrimSpace(pass) == "" {
					continue
				}
				if err := s.d.Repo.SetSecret(ctx, config.SpacePasswordName(name), pass); err != nil {
					return err
				}
				s.d.Log.Infow("secret_saved", "name", config.SpacePasswordName(name))
			}
			return nil
		}
	default:
		return apiError(ctx, http.StatusNotFound, fmt.Errorf("未知设置分区 %q", section))
	}

	if err := cur.Finalize(); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	// 密钥先于配置落库:加密失败(如未配置主密钥)时本次保存整体不生效
	if secretName != "" && strings.TrimSpace(secret) != "" {
		if err := s.d.Repo.SetSecret(ctx, secretName, secret); err != nil {
			return apiError(ctx, http.StatusBadRequest, err)
		}
		s.d.Log.Infow("secret_saved", "name", secretName)
	}
	if persistSecrets != nil {
		if err := persistSecrets(); err != nil {
			return apiError(ctx, http.StatusBadRequest, err)
		}
	}
	if err := persist(); err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	return s.respondOK(ctx)
}

// ---- 设置连通性测试(表单值,不落库) ----

// llmTestPost 用表单中的模型配置发起一次最小对话请求,验证配置与密钥可用。
// secret 为表单中输入的 API Key(空则回退环境变量/已保存密文)。
func (s *Server) llmTestPost(ctx khttp.Context) error {
	var in struct {
		Config repository.LLMConfig `json:"config"`
		Secret string               `json:"secret"`
	}
	if err := decode(ctx.Request(), &in); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	if s.d.TestLLM == nil {
		return apiError(ctx, http.StatusServiceUnavailable, fmt.Errorf("模型测试不可用"))
	}
	msg, err := s.d.TestLLM(ctx, in.Config, in.Secret)
	if err != nil {
		return writeJSON(ctx, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
	}
	return writeJSON(ctx, http.StatusOK, map[string]any{"ok": true, "message": msg})
}

// mailerTestPost 用表单中的发送器配置发送测试邮件(站内信模式写入测试产物)。
// secret 为表单中输入的授权码(空则使用已保存密文);to 为可选测试收件人,
// 缺省取第一个 email 推送目标的收件地址。
func (s *Server) mailerTestPost(ctx khttp.Context) error {
	var in struct {
		Config repository.MailerConfig `json:"config"`
		Secret string                  `json:"secret"`
		To     string                  `json:"to"`
	}
	if err := decode(ctx.Request(), &in); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	if s.d.TestMailer == nil {
		return apiError(ctx, http.StatusServiceUnavailable, fmt.Errorf("发送器测试不可用"))
	}
	msg, err := s.d.TestMailer(ctx, in.Config, in.Secret, in.To)
	if err != nil {
		return writeJSON(ctx, http.StatusOK, map[string]any{"ok": false, "message": err.Error()})
	}
	return writeJSON(ctx, http.StatusOK, map[string]any{"ok": true, "message": msg})
}

// listSecrets 只返回已配置的敏感数据名称(不回传明文)。
func (s *Server) listSecrets(ctx khttp.Context) error {
	names, err := s.d.Repo.ListSecrets(ctx)
	if err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	return writeJSON(ctx, http.StatusOK, map[string]any{"items": names})
}

// deleteSecret 删除已保存的敏感数据。
func (s *Server) deleteSecret(ctx khttp.Context) error {
	name := ctxParam(ctx, "name")
	if err := s.d.Repo.DeleteSecret(ctx, name); err != nil {
		return apiError(ctx, http.StatusNotFound, err)
	}
	return s.respondOK(ctx)
}

func (s *Server) applyPost(ctx khttp.Context) error {
	if err := s.d.Apply(); err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	return writeJSON(ctx, http.StatusOK, map[string]any{"ok": true, "applied": true})
}

// ---- 运行 / 产物 ----

func (s *Server) listRuns(ctx khttp.Context) error {
	subscription := ctx.Query().Get("subscription")
	limit := 200 // 缺省上限(与前端列表一致)
	if v, err := strconv.Atoi(ctx.Query().Get("limit")); err == nil && v > 0 {
		limit = v
	}
	runs, err := s.d.Repo.ListRuns(ctx, subscription, limit)
	if err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	return writeJSON(ctx, http.StatusOK, map[string]any{"items": runs})
}

func (s *Server) runDetail(ctx khttp.Context) error {
	id, err := strconv.ParseInt(ctxParam(ctx, "id"), 10, 64)
	if err != nil {
		return apiError(ctx, http.StatusBadRequest, fmt.Errorf("非法运行 ID"))
	}
	rec, err := s.d.Repo.GetRun(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return apiError(ctx, http.StatusNotFound, fmt.Errorf("运行记录不存在"))
	}
	if err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	return writeJSON(ctx, http.StatusOK, rec)
}

// artifactNameRe 匹配产物文件名 <订阅名>-<日期>-<时间>.html,同时用于提取订阅名。
var artifactNameRe = regexp.MustCompile(`^(.+)-(\d{8}-\d{6})\.html$`)

// artifactOut 是列表中的产物条目(附带所属空间、解析出的订阅名与产物 DSN)。
type artifactOut struct {
	artifact.File
	Space        string `json:"space"`        // 所属产物空间名
	Subscription string `json:"subscription"` // 从文件名解析的订阅名(解析不出为空)
	DSN          string `json:"dsn"`          // 产物路径:类型://空间名/分桶/文件名
}

// listArtifacts 列出全部产物空间的文件(跨分桶,最新在前);?subscription= 按订阅
// (文件名前缀)过滤,?space= 按空间过滤。
func (s *Server) listArtifacts(ctx khttp.Context) error {
	spaces := s.d.Artifacts()
	if spaces == nil {
		return apiError(ctx, http.StatusServiceUnavailable, fmt.Errorf("产物空间未就绪"))
	}
	files, err := spaces.List(ctx)
	if err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	subFilter := ctx.Query().Get("subscription")
	spaceFilter := ctx.Query().Get("space")
	items := make([]artifactOut, 0, len(files))
	for _, f := range files {
		if spaceFilter != "" && f.Space != spaceFilter {
			continue
		}
		item := artifactOut{File: f.File, Space: f.Space, DSN: artifact.DSN(spaces.Type(f.Space), f.Space, f.Bucket, f.Name)}
		if match := artifactNameRe.FindStringSubmatch(f.Name); match != nil {
			item.Subscription = match[1]
		}
		if subFilter != "" && item.Subscription != subFilter {
			continue
		}
		items = append(items, item)
	}
	return writeJSON(ctx, http.StatusOK, map[string]any{"items": items})
}

// artifactContent 读取一个产物文件(HTML 文档)。
func (s *Server) artifactContent(ctx khttp.Context) error {
	spaces := s.d.Artifacts()
	if spaces == nil {
		return apiError(ctx, http.StatusServiceUnavailable, fmt.Errorf("产物空间未就绪"))
	}
	space, bucket, name := ctxParam(ctx, "space"), ctxParam(ctx, "bucket"), ctxParam(ctx, "name")
	raw, err := spaces.Read(ctx, space, bucket, name)
	if errors.Is(err, storage.ErrNotFound) {
		return apiError(ctx, http.StatusNotFound, fmt.Errorf("产物不存在"))
	}
	if err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	out := map[string]any{
		"space":   space,
		"bucket":  bucket,
		"name":    name,
		"dsn":     artifact.DSN(spaces.Type(space), space, bucket, name),
		"content": string(raw),
		"size":    len(raw),
	}
	if files, err := spaces.List(ctx); err == nil {
		for _, f := range files {
			if f.Space == space && f.Bucket == bucket && f.Name == name {
				out["size"] = f.Size
				out["mod_time"] = f.ModTime
				if match := artifactNameRe.FindStringSubmatch(name); match != nil {
					out["subscription"] = match[1]
				}
				break
			}
		}
	}
	return writeJSON(ctx, http.StatusOK, out)
}

// ---- SPA 静态资源 ----

// spaHandler 托管前端构建产物:命中文件直接服务,未命中回退 index.html。
func (s *Server) spaHandler() http.Handler {
	dist, err := fs.Sub(frontend.DistFS, "dist")
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "前端资产不可用", http.StatusServiceUnavailable)
		})
	}
	fileServer := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "不支持的请求方法", http.StatusMethodNotAllowed)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if f, err := dist.Open(path); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		index, err := dist.Open("index.html")
		if err != nil {
			http.Error(w, "前端资产未构建:请运行 make frontend", http.StatusServiceUnavailable)
			return
		}
		defer index.Close()
		data, err := io.ReadAll(index)
		if err != nil {
			http.Error(w, "读取前端资产失败", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
}
