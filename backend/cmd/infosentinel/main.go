// infosentinel:基于 CloudWeGo Eino 的兴趣资讯收集 Agent。
//
// 配置:config.yaml 只含引导项(数据库/日志);业务配置存于 SQLite,
// 由 Web 管理界面维护。HTTP 传输层使用 kratos transport/http,
// 依赖注入使用 google/wire(见 wire.go / provider.go 与各包 ProviderSet)。
//
// 用法:
//
//	infosentinel run                 执行全部订阅
//	infosentinel run -topic name     只执行指定主题的订阅
//	infosentinel serve               常驻,按各订阅的 cron 定时执行(含 Web 管理)
//	infosentinel list                打印主题/目标/订阅一览
//	infosentinel migrate             初始化/升级数据库并写入内置默认配置后退出
//	infosentinel -c other.yaml run   指定引导配置文件
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	klog "github.com/go-kratos/kratos/v2/log"

	"github.com/tomasgdl/infosentinel/backend/internal/artifact"
	"github.com/tomasgdl/infosentinel/backend/internal/config"
	"github.com/tomasgdl/infosentinel/backend/internal/repository"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data"
	"github.com/tomasgdl/infosentinel/backend/internal/runner"
)

// version 由构建注入(make build 时 -ldflags "-X main.version=..."),默认 dev。
var version = "dev"

const usage = `infosentinel — 基于 Eino 的兴趣资讯收集 Agent

用法:
  infosentinel [flags] <run|serve|list|migrate>

子命令:
  run     执行订阅(默认全部;配合 -topic 只跑一个),采集 → 总结 → 推送
  serve   常驻运行,按各订阅 cron 定时执行;内置 Web 管理界面(kratos HTTP)
  list    打印配置中的主题 / 推送目标 / 订阅一览
  migrate 初始化或升级数据库表结构;空库写入内置默认配置(幂等)后退出

Flags:
  -c string     引导配置文件路径(默认 "config.yaml";仅含 database / log / storage)
  -topic string  run 子命令:只执行指定主题的订阅
  -version      打印版本信息
`

// cliApp 聚合 CLI 子命令(run/list/migrate)所需依赖,由 initCLI 注入。
type cliApp struct {
	Boot    *config.Bootstrap
	Data    *data.Data
	Log     *klog.Helper
	Secrets *config.Resolver
}

func main() {
	var (
		cfgPath     string
		topicName   string
		showVersion bool
	)
	flag.StringVar(&cfgPath, "c", "config.yaml", "引导配置文件路径")
	flag.StringVar(&topicName, "topic", "", "run 子命令:只执行指定主题的订阅")
	flag.BoolVar(&showVersion, "version", false, "打印版本信息")
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	flag.Parse()

	if showVersion {
		fmt.Printf("infosentinel %s\n", version)
		return
	}

	cmd := flag.Arg(0)
	if cmd == "" {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	switch cmd {
	case "serve":
		serveMain(cfgPath)
	case "run", "list", "migrate":
		cliMain(cmd, cfgPath, topicName)
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
}

// serveMain 常驻模式:wire 构造 kratos app 并运行(信号处理由 kratos 负责)。
func serveMain(cfgPath string) {
	app, cleanup, err := initApp(cfgPath)
	if err != nil {
		log.Fatalf("初始化服务失败: %v", err)
	}
	defer cleanup()
	if err := app.Run(); err != nil {
		log.Fatalf("服务异常退出: %v", err)
	}
}

// cliMain CLI 子命令:wire 构造依赖后分发。
func cliMain(cmd, cfgPath, topicName string) {
	cli, cleanup, err := initCLI(cfgPath)
	if err != nil {
		log.Fatalf("初始化失败: %v", err)
	}
	defer cleanup()

	if cli.Data.Seeded() {
		cli.Log.Infow("database_seeded")
	}

	// migrate:只做数据库初始化/升级(空库写入内置默认配置)后退出
	if cmd == "migrate" {
		res := cli.Data.MigrationResult()
		printMigration(res, cli.Boot.Database.Path)
		cli.Log.Infow("database_migrated",
			"db", cli.Boot.Database.Path,
			"from", res.From,
			"to", res.To,
			"applied", len(res.Applied),
		)
		return
	}

	cfg := loadEffective(cli.Log, cli.Data)
	cli.Log.Infow("startup",
		"version", version,
		"db", cli.Boot.Database.Path,
		"log", cli.Boot.Log.Path,
		"subscriptions", len(cfg.Subscriptions),
		"schema_version", cli.Data.MigrationResult().To,
	)

	switch cmd {
	case "run":
		run(cfg, cli.Boot, cli.Secrets, cli.Log, topicName)
	case "list":
		list(cfg)
	}
}

// printMigration 打印 migrate 子命令结果。
func printMigration(res data.MigrationResult, dbPath string) {
	if len(res.Applied) == 0 {
		fmt.Printf("数据库已是最新版本 %s: %s\n", res.To, dbPath)
		return
	}
	if res.From == "" {
		fmt.Printf("数据库初始化完成 → %s: %s\n", res.To, dbPath)
	} else {
		fmt.Printf("数据库升级完成 %s → %s: %s\n", res.From, res.To, dbPath)
	}
	for _, version := range res.Applied {
		fmt.Printf("  applied %s\n", version)
	}
}

// loadEffective 从数据库读取生效配置。
func loadEffective(lg *klog.Helper, repo repository.Repository) *repository.Config {
	cfg, err := repo.LoadConfig(context.Background())
	if err != nil {
		lg.Fatalf("加载数据库配置失败: %v", err)
	}
	return cfg
}

// selectSubscriptions 按 -topic 参数筛选要执行的订阅;空 = 全部。
func selectSubscriptions(cfg *repository.Config, topic string) ([]repository.Subscription, error) {
	if topic == "" {
		return cfg.Subscriptions, nil
	}
	s := cfg.SubscriptionByTopic(topic)
	if s == nil {
		return nil, fmt.Errorf("订阅 %q 不存在(可用 list 子命令查看)", topic)
	}
	return []repository.Subscription{*s}, nil
}

// run 执行选中的订阅;单个订阅失败不影响其余订阅,最后汇总退出码。
func run(cfg *repository.Config, boot *config.Bootstrap, secrets *config.Resolver, lg *klog.Helper, topic string) {
	subs, err := selectSubscriptions(cfg, topic)
	if err != nil {
		lg.Fatalf("选择订阅失败: %v", err)
	}

	// 产物空间:站内信投递写入;密码缺失不致命(本地驱动/私钥登录不需要)
	ctx := context.Background()
	password, perr := secrets.StoragePassword(ctx, "")
	if perr != nil {
		lg.Warnf("storage_password_resolve_failed err=%v", perr)
	}
	arts, err := artifact.Build(&cfg.Storage, boot.Storage.BaseDir, password)
	if err != nil {
		lg.Fatalf("初始化产物空间失败: %v", err)
	}

	r := runner.New(cfg, secrets, arts, boot.StateDir, lg)
	var failed []string
	for _, s := range subs {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.LLM.TimeoutSec)*time.Second)
		_, err := r.RunTask(ctx, s)
		cancel()
		if err != nil {
			lg.Warnf("sub_run_failed topic=%s err=%v", s.Topic, err)
			failed = append(failed, s.Topic)
		}
	}
	if len(failed) > 0 {
		lg.Fatalf("执行结束,%d/%d 个订阅失败: %v", len(failed), len(subs), failed)
	}
	lg.Infow("run_finished", "subscriptions", len(subs))
}

// list 打印配置概览。
func list(cfg *repository.Config) {
	fmt.Println("主题:")
	for _, t := range cfg.Topics {
		fmt.Printf("  %-20s prompt %d 字\n", t.Name, len([]rune(t.Prompt)))
	}
	fmt.Println("推送目标:")
	for _, t := range cfg.Targets {
		fmt.Printf("  %-20s %-6s → %s\n", t.Name, t.Type, t.To)
	}
	fmt.Println("订阅:")
	for _, s := range cfg.Subscriptions {
		fmt.Printf("  topic=%-20s cron=%-14s targets=%v\n", s.Topic, s.Cron, s.Targets)
	}
}
