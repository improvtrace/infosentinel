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
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	klog "github.com/go-kratos/kratos/v2/log"
	khttp "github.com/go-kratos/kratos/v2/transport/http"

	"github.com/tomasgdl/infosentinel/backend/internal/history"
	"github.com/tomasgdl/infosentinel/backend/internal/logger"
	"github.com/tomasgdl/infosentinel/backend/internal/repository"
	"github.com/tomasgdl/infosentinel/backend/internal/scheduler"
	"github.com/tomasgdl/infosentinel/frontend"
)

// Deps 是界面后端依赖(全部以函数/接口注入,配置热重载后自动读取最新状态)。
type Deps struct {
	Version    string
	StartedAt  time.Time
	Config     func() *repository.Config  // 当前生效配置
	NextRuns   func() []scheduler.NextRun // 各 (任务, 表达式) 的下次触发
	History    *history.Store             // 运行历史
	IsRunning  func(name string) bool     // 任务是否在执行中
	Trigger    func(name string) error    // 手动触发任务(后台执行)
	TestTarget func(name string) error    // 向推送目标发送测试邮件
	Apply      func() error               // 从数据库重载配置并重建调度
	Repo       repository.Repository      // 配置存储
	EmailsDir  func() string              // dry-run 邮件产物目录
	Log        *klog.Helper               // 日志(访问埋点)
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

	api.GET("/tasks", s.listTasks)
	api.POST("/tasks", s.createTask)
	api.PUT("/tasks/{name}", s.updateTask)
	api.DELETE("/tasks/{name}", s.deleteTask)
	api.POST("/tasks/{name}/run", s.runTaskPost)

	api.GET("/sources", s.getSources)
	api.PUT("/sources", s.putSources)
	api.GET("/settings", s.getSettings)
	api.PUT("/settings/{section}", s.putSettings)
	api.GET("/secrets", s.listSecrets)
	api.PUT("/secrets/{name}", s.putSecret)
	api.DELETE("/secrets/{name}", s.deleteSecret)
	api.POST("/apply", s.applyPost)

	api.GET("/runs", s.listRuns)
	api.GET("/runs/{id}", s.runDetail)
	api.GET("/emails", s.listEmailsAPI)
	api.GET("/emails/{name}", s.emailContent)
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
	runs := s.d.History.Snapshot()

	running := 0
	for _, t := range cfg.Tasks {
		if s.d.IsRunning(t.Name) {
			running++
		}
	}
	ok, fail := 0, 0
	for _, rr := range runs {
		switch rr.Status {
		case history.Success:
			ok++
		case history.Failed:
			fail++
		}
	}
	recent := runs
	if len(recent) > 20 {
		recent = recent[:20]
	}

	return writeJSON(ctx, http.StatusOK, map[string]any{
		"version":     s.d.Version,
		"uptime_sec":  time.Since(s.d.StartedAt).Seconds(),
		"tasks":       len(cfg.Tasks),
		"running":     running,
		"ok":          ok,
		"fail":        fail,
		"nexts":       s.d.NextRuns(),
		"recent_runs": recent,
		"targets":     cfg.Targets,
		"mailer_type": cfg.Mailer.Type,
	})
}

func (s *Server) getConfig(ctx khttp.Context) error {
	return writeJSON(ctx, http.StatusOK, s.d.Config())
}

func (s *Server) health(ctx khttp.Context) error {
	cfg := s.d.Config()
	running := 0
	for _, t := range cfg.Tasks {
		if s.d.IsRunning(t.Name) {
			running++
		}
	}
	return writeJSON(ctx, http.StatusOK, map[string]any{
		"version": s.d.Version,
		"uptime":  time.Since(s.d.StartedAt).Round(time.Second).String(),
		"tasks":   len(cfg.Tasks),
		"running": running,
		"history": s.d.History.Len(),
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

// ---- 任务 ----

func (s *Server) listTasks(ctx khttp.Context) error {
	items, err := s.d.Repo.ListTasks(ctx)
	if err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	return writeJSON(ctx, http.StatusOK, map[string]any{"items": items})
}

// saveTask 是创建/更新任务的公共路径:整份配置校验(Finalize)→ 落库 → 应用。
func (s *Server) saveTask(ctx khttp.Context, t repository.Task, isNew bool) error {
	if isNew {
		if err := validName(t.Name); err != nil {
			return apiError(ctx, http.StatusBadRequest, err)
		}
	}
	cur := *s.d.Config()
	replaced := false
	tasks := make([]repository.Task, 0, len(cur.Tasks)+1)
	for _, e := range cur.Tasks {
		if e.Name == t.Name {
			tasks = append(tasks, t)
			replaced = true
		} else {
			tasks = append(tasks, e)
		}
	}
	if !replaced {
		tasks = append(tasks, t)
	}
	cur.Tasks = tasks
	if err := cur.Finalize(); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	if err := s.d.Repo.UpsertTask(ctx, t); err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	return s.respondOK(ctx)
}

func (s *Server) createTask(ctx khttp.Context) error {
	var t repository.Task
	if err := decode(ctx.Request(), &t); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	return s.saveTask(ctx, t, true)
}

func (s *Server) updateTask(ctx khttp.Context) error {
	name := ctxParam(ctx, "name")
	if _, err := s.d.Repo.GetTask(ctx, name); err != nil {
		return apiError(ctx, http.StatusNotFound, fmt.Errorf("任务 %q 不存在", name))
	}
	var in repository.Task
	if err := decode(ctx.Request(), &in); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	in.Name = name
	return s.saveTask(ctx, in, false)
}

func (s *Server) deleteTask(ctx khttp.Context) error {
	if err := s.d.Repo.DeleteTask(ctx, ctxParam(ctx, "name")); err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	return s.respondOK(ctx)
}

func (s *Server) runTaskPost(ctx khttp.Context) error {
	if err := s.d.Trigger(ctxParam(ctx, "name")); err != nil {
		return apiError(ctx, http.StatusConflict, err)
	}
	return writeJSON(ctx, http.StatusOK, map[string]any{"ok": true, "started": true})
}

// ---- 数据源 / 设置 / 敏感数据 ----

func (s *Server) getSources(ctx khttp.Context) error {
	raw, err := yamlMarshal(s.d.Config().Sources)
	if err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	return writeJSON(ctx, http.StatusOK, map[string]any{"yaml": string(raw)})
}

func (s *Server) putSources(ctx khttp.Context) error {
	var in struct {
		YAML string `json:"yaml"`
	}
	if err := decode(ctx.Request(), &in); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	var src repository.Sources
	if err := yamlUnmarshal([]byte(in.YAML), &src); err != nil {
		return apiError(ctx, http.StatusBadRequest, fmt.Errorf("YAML 解析失败:%v", err))
	}
	cur := *s.d.Config()
	cur.Sources = src
	if err := cur.Finalize(); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	if err := s.d.Repo.PutSetting(ctx, "sources", src); err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	return s.respondOK(ctx)
}

func (s *Server) getSettings(ctx khttp.Context) error {
	cfg := s.d.Config()
	secrets, _ := s.d.Repo.ListSecrets(ctx)
	return writeJSON(ctx, http.StatusOK, map[string]any{
		"llm": cfg.LLM, "mailer": cfg.Mailer, "server": cfg.Server,
		"secrets": secrets,
	})
}

func (s *Server) putSettings(ctx khttp.Context) error {
	section := ctxParam(ctx, "section")
	cur := *s.d.Config()

	var persist func() error
	switch section {
	case "llm":
		var in repository.LLMConfig
		if err := decode(ctx.Request(), &in); err != nil {
			return apiError(ctx, http.StatusBadRequest, err)
		}
		cur.LLM = in
		persist = func() error { return s.d.Repo.PutSetting(ctx, "llm", in) }
	case "mailer":
		var in repository.MailerConfig
		if err := decode(ctx.Request(), &in); err != nil {
			return apiError(ctx, http.StatusBadRequest, err)
		}
		cur.Mailer = in
		persist = func() error { return s.d.Repo.PutSetting(ctx, "mailer", in) }
	case "server":
		var in repository.ServerConfig
		if err := decode(ctx.Request(), &in); err != nil {
			return apiError(ctx, http.StatusBadRequest, err)
		}
		cur.Server = in
		persist = func() error { return s.d.Repo.PutSetting(ctx, "server", in) }
	default:
		return apiError(ctx, http.StatusNotFound, fmt.Errorf("未知设置分区 %q", section))
	}

	if err := cur.Finalize(); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	if err := persist(); err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	return s.respondOK(ctx)
}

// listSecrets 只返回已配置的敏感数据名称(不回传明文)。
func (s *Server) listSecrets(ctx khttp.Context) error {
	names, err := s.d.Repo.ListSecrets(ctx)
	if err != nil {
		return apiError(ctx, http.StatusInternalServerError, err)
	}
	return writeJSON(ctx, http.StatusOK, map[string]any{"items": names})
}

// putSecret 加密保存敏感数据(明文仅存在于请求体)。
func (s *Server) putSecret(ctx khttp.Context) error {
	name := ctxParam(ctx, "name")
	if name != "llm.api_key" && name != "smtp.password" {
		return apiError(ctx, http.StatusBadRequest, fmt.Errorf("不支持的敏感数据名称 %q", name))
	}
	var in struct {
		Value string `json:"value"`
	}
	if err := decode(ctx.Request(), &in); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	if strings.TrimSpace(in.Value) == "" {
		return apiError(ctx, http.StatusBadRequest, fmt.Errorf("值不能为空"))
	}
	if err := s.d.Repo.SetSecret(ctx, name, in.Value); err != nil {
		return apiError(ctx, http.StatusBadRequest, err)
	}
	s.d.Log.Infow("secret_saved", "name", name)
	return s.respondOK(ctx)
}

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

// ---- 运行 / 邮件 ----

func (s *Server) listRuns(ctx khttp.Context) error {
	runs := s.d.History.Snapshot()
	if tf := ctx.Query().Get("task"); tf != "" {
		filtered := make([]history.Record, 0, len(runs))
		for _, rr := range runs {
			if rr.Task == tf {
				filtered = append(filtered, rr)
			}
		}
		runs = filtered
	}
	if limit, err := strconv.Atoi(ctx.Query().Get("limit")); err == nil && limit > 0 && limit < len(runs) {
		runs = runs[:limit]
	}
	return writeJSON(ctx, http.StatusOK, map[string]any{"items": runs})
}

func (s *Server) runDetail(ctx khttp.Context) error {
	id, err := strconv.ParseInt(ctxParam(ctx, "id"), 10, 64)
	if err != nil {
		return apiError(ctx, http.StatusBadRequest, fmt.Errorf("非法运行 ID"))
	}
	rec, ok := s.d.History.Get(id)
	if !ok {
		return apiError(ctx, http.StatusNotFound, fmt.Errorf("运行记录不存在"))
	}
	return writeJSON(ctx, http.StatusOK, rec)
}

func (s *Server) listEmailsAPI(ctx khttp.Context) error {
	files := listEmailFiles(s.d.EmailsDir())
	if tf := ctx.Query().Get("task"); tf != "" {
		filtered := make([]emailFile, 0, len(files))
		for _, f := range files {
			if f.Task == tf {
				filtered = append(filtered, f)
			}
		}
		files = filtered
	}
	return writeJSON(ctx, http.StatusOK, map[string]any{"items": files})
}

func (s *Server) emailContent(ctx khttp.Context) error {
	name := ctxParam(ctx, "name")
	if !emailNameRe.MatchString(name) {
		return apiError(ctx, http.StatusBadRequest, fmt.Errorf("非法文件名"))
	}
	path := filepath.Join(s.d.EmailsDir(), name)
	raw, err := os.ReadFile(path)
	if err != nil {
		return apiError(ctx, http.StatusNotFound, fmt.Errorf("邮件文件不存在"))
	}
	info, _ := os.Stat(path)
	ef := emailFile{Name: name, ModTime: info.ModTime(), Size: info.Size()}
	if m := emailNameRe.FindStringSubmatch(name); m != nil {
		ef.Task = m[1]
	}
	return writeJSON(ctx, http.StatusOK, map[string]any{
		"name": ef.Name, "task": ef.Task, "size": ef.Size,
		"mod_time": ef.ModTime, "content": string(raw),
	})
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

// ---- 邮件产物辅助 ----

// emailFile 是一个 dry-run 邮件产物文件。
type emailFile struct {
	Name    string    `json:"name"`
	Task    string    `json:"task"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

// emailNameRe 匹配 dry-run 邮件文件名:email-<task>-<日期>-<时间>.md,
// 同时用于 URL 段校验(防路径穿越)。
var emailNameRe = regexp.MustCompile(`^email-(.+)-(\d{8}-\d{6})\.md$`)

// listEmailFiles 列出产物目录下的邮件文件,最新在前。
func listEmailFiles(dir string) []emailFile {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return []emailFile{}
	}
	out := []emailFile{}
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !emailNameRe.MatchString(name) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		ef := emailFile{Name: name, Size: info.Size(), ModTime: info.ModTime()}
		if m := emailNameRe.FindStringSubmatch(name); m != nil {
			ef.Task = m[1]
		}
		out = append(out, ef)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out
}

// ctxBackground 返回处理器的 context(供需要 context.Context 的调用)。
func ctxBackground(ctx khttp.Context) context.Context { return ctx }
