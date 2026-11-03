// Package config 负责 infosentinel 的配置加载、默认值填充与校验。
//
// 配置分四个模块:
//   - sources:公用数据源池(所有主题共享,每类源由内置工具支撑)
//   - topics: 主题,每项是一段外部撰写的 prompt,agent 据此生成报告
//   - targets: 推送目标(通道),当前支持 email,可配置多个
//   - tasks:   任务 = {主题, 推送目标, 调度表达式}
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/robfig/cron/v3"

	"gopkg.in/yaml.v3"
)

// Config 是 infosentinel 的顶层配置,对应 config.yaml。
type Config struct {
	// Sources 是公用数据源池。
	Sources Sources `yaml:"sources"`

	// Topics 是主题列表,每项一段 prompt。
	Topics []Topic `yaml:"topics"`

	// Targets 是推送目标(通道)列表。
	Targets []Target `yaml:"targets"`

	// Tasks 是任务列表:{主题, 推送目标, 调度表达式}。
	Tasks []Task `yaml:"tasks"`

	// LLM 是模型服务配置(OpenAI 兼容协议),全局共用。
	LLM LLMConfig `yaml:"llm"`

	// Mailer 是发送器全局配置(dry-run 输出目录等)。
	Mailer MailerConfig `yaml:"mailer"`
}

// Sources 是公用数据源池:每类源对应一个内置工具,同类多实例共用该工具。
type Sources struct {
	// Collect 是采集通用参数。
	Collect CollectConfig `yaml:"collect"`

	// RSS 是 RSS/Atom 订阅源(fetch_rss 工具)。
	RSS RSSConfig `yaml:"rss"`

	// Search 是 DuckDuckGo 搜索(search_web 工具)。
	Search SearchConfig `yaml:"search"`

	// HackerNews 是 HN 热榜(fetch_hackernews 工具)。
	HackerNews HackerNewsConfig `yaml:"hackernews"`

	// GitHubTrending 是 GitHub Trending(fetch_github_trending 工具)。
	GitHubTrending GitHubTrendingConfig `yaml:"github_trending"`

	// Webpages 是固定网页列表(fetch_webpage 工具按 URL 抓取),非空即启用。
	Webpages []Webpage `yaml:"webpages"`
}

// CollectConfig 是采集通用参数。
type CollectConfig struct {
	// WindowHours 是内容时间窗:只保留最近 N 小时内发布/更新的条目(RSS/HN)。
	WindowHours int `yaml:"window_hours"`
	// MaxItemsPerSource 是单个工具单次返回的最大条目数,控制上下文长度。
	MaxItemsPerSource int `yaml:"max_items_per_source"`
	// HTTPTimeoutSec 是采集请求的超时秒数。
	HTTPTimeoutSec int `yaml:"http_timeout_sec"`
	// MaxBodyChars 是单条网页正文的截断字符数。
	MaxBodyChars int `yaml:"max_body_chars"`
	// DedupDays 是 URL 去重状态保留天数,超过则重新允许出现。
	DedupDays int `yaml:"dedup_days"`
	// StateDir 是去重状态目录,每个任务一个文件:<state_dir>/<task>.json。
	StateDir string `yaml:"state_dir"`
}

// RSSConfig 是 RSS 订阅源配置。
type RSSConfig struct {
	Enabled bool      `yaml:"enabled"`
	Feeds   []RSSFeed `yaml:"feeds"`
}

// RSSFeed 是单个 RSS/Atom 源。
type RSSFeed struct {
	// Name 是源的可读名称,展示给 LLM 与邮件读者。
	Name string `yaml:"name"`
	// URL 是 RSS/Atom 地址。
	URL string `yaml:"url"`
}

// SearchConfig 是搜索引擎(DuckDuckGo)配置。
type SearchConfig struct {
	Enabled bool   `yaml:"enabled"`
	Region  string `yaml:"region"` // ddgsearch region,如 cn-zh / us-en / wt-wt
	// MaxResults 是单次搜索返回的最大结果数。
	MaxResults int `yaml:"max_results"`
}

// HackerNewsConfig 是 Hacker News 热榜配置。
type HackerNewsConfig struct {
	Enabled bool `yaml:"enabled"`
	// TopN 是从热榜取前 N 条。
	TopN int `yaml:"top_n"`
	// MinPoints 是最低分阈值,过滤低热度条目。
	MinPoints int `yaml:"min_points"`
}

// GitHubTrendingConfig 是 GitHub Trending 配置。
type GitHubTrendingConfig struct {
	Enabled bool `yaml:"enabled"`
	// Language 是过滤的编程语言,空表示全部。
	Language string `yaml:"language"`
	// TopN 是取前 N 个仓库。
	TopN int `yaml:"top_n"`
}

// Webpage 是固定抓取的网页。
type Webpage struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// Topic 是主题:一段外部撰写的 prompt,agent 据此采集与生成报告。
type Topic struct {
	// Name 是主题唯一标识,供任务引用。
	Name string `yaml:"name"`
	// Prompt 是主题指令:关注什么、如何取舍、报告怎么写。
	Prompt string `yaml:"prompt"`
}

// Target 是推送目标(通道)。当前支持 email;后续可扩展 webhook 等。
type Target struct {
	// Name 是目标唯一标识,供任务引用。
	Name string `yaml:"name"`
	// Type 是通道类型,当前仅支持 "email"。
	Type string `yaml:"type"`
	// To 是收件邮箱(email 通道必填)。
	To string `yaml:"to"`
	// SubjectPrefix 是邮件标题前缀。
	SubjectPrefix string `yaml:"subject_prefix"`
}

// Task 是任务:{主题, 推送目标, 调度表达式}。
type Task struct {
	// Name 是任务唯一标识,也用于隔离去重状态文件。
	Name string `yaml:"name"`
	// Topic 引用主题名。
	Topic string `yaml:"topic"`
	// Targets 引用推送目标名,可多个。
	Targets []string `yaml:"targets"`
	// Schedules 是 cron 表达式列表(五段),每个表达式都会触发该任务。
	Schedules []string `yaml:"schedules"`
}

// LLMConfig 是 OpenAI 兼容模型服务配置。
type LLMConfig struct {
	// BaseURL 是 API 根地址,例如 https://api.deepseek.com 或 https://open.bigmodel.cn/api/paas/v4。
	BaseURL string `yaml:"base_url"`
	// Model 是模型名,例如 deepseek-chat。
	Model string `yaml:"model"`
	// APIKeyEnv 是存放 API Key 的环境变量名。
	APIKeyEnv string `yaml:"api_key_env"`
	// Temperature 是采样温度。
	Temperature float32 `yaml:"temperature"`
	// TimeoutSec 是单次模型请求超时。
	TimeoutSec int `yaml:"timeout_sec"`
	// MaxStep 是 agent 循环的最大步数(防止失控)。
	MaxStep int `yaml:"max_step"`
}

// MailerConfig 是发送器全局配置。
type MailerConfig struct {
	// Type 是发送器类型:当前支持 dryrun;smtp 为预留。
	Type string `yaml:"type"`
	// OutputDir 是 dryrun 模式下邮件文件输出目录。
	OutputDir string `yaml:"output_dir"`
}

// Load 从 path 读取 YAML 配置,填充默认值并校验。
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件 %s: %w", path, err)
	}
	return Parse(raw)
}

// Parse 从 YAML 文本解析配置(不读文件),填充默认值并校验。
// 供 expect 子命令验证 LLM 生成的配置草案使用。
func Parse(data []byte) (*Config, error) {
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置 YAML: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("配置校验失败: %w", err)
	}
	return cfg, nil
}

// applyDefaults 为空字段填充默认值。
func (c *Config) applyDefaults() {
	cc := &c.Sources.Collect
	if cc.WindowHours <= 0 {
		cc.WindowHours = 48
	}
	if cc.MaxItemsPerSource <= 0 {
		cc.MaxItemsPerSource = 15
	}
	if cc.HTTPTimeoutSec <= 0 {
		cc.HTTPTimeoutSec = 20
	}
	if cc.MaxBodyChars <= 0 {
		cc.MaxBodyChars = 4000
	}
	if cc.DedupDays <= 0 {
		cc.DedupDays = 7
	}
	if cc.StateDir == "" {
		cc.StateDir = "out/state"
	}

	if c.Sources.Search.Region == "" {
		c.Sources.Search.Region = "cn-zh"
	}
	if c.Sources.Search.MaxResults <= 0 {
		c.Sources.Search.MaxResults = 8
	}
	if c.Sources.HackerNews.TopN <= 0 {
		c.Sources.HackerNews.TopN = 15
	}
	if c.Sources.HackerNews.MinPoints <= 0 {
		c.Sources.HackerNews.MinPoints = 50
	}
	if c.Sources.GitHubTrending.TopN <= 0 {
		c.Sources.GitHubTrending.TopN = 10
	}

	for i := range c.Targets {
		if c.Targets[i].SubjectPrefix == "" {
			c.Targets[i].SubjectPrefix = "[InfoSentinel]"
		}
	}

	if c.LLM.BaseURL == "" {
		c.LLM.BaseURL = "https://api.deepseek.com"
	}
	if c.LLM.Model == "" {
		c.LLM.Model = "deepseek-chat"
	}
	if c.LLM.APIKeyEnv == "" {
		c.LLM.APIKeyEnv = "DEEPSEEK_API_KEY"
	}
	if c.LLM.Temperature <= 0 {
		c.LLM.Temperature = 0.3
	}
	if c.LLM.TimeoutSec <= 0 {
		c.LLM.TimeoutSec = 300
	}
	if c.LLM.MaxStep <= 0 {
		c.LLM.MaxStep = 25
	}
	if c.Mailer.Type == "" {
		c.Mailer.Type = "dryrun"
	}
	if c.Mailer.OutputDir == "" {
		c.Mailer.OutputDir = "out"
	}
}

// Validate 检查配置合法性,返回发现的第一个问题。
func (c *Config) Validate() error {
	if len(c.Topics) == 0 {
		return fmt.Errorf("topics 不能为空:请至少定义一个主题(一段 prompt)")
	}
	if len(c.Targets) == 0 {
		return fmt.Errorf("targets 不能为空:请至少配置一个推送目标")
	}
	if len(c.Tasks) == 0 {
		return fmt.Errorf("tasks 不能为空:请至少定义一个任务")
	}

	// 数据源:至少启用一个渠道
	s := &c.Sources
	if !s.RSS.Enabled && !s.Search.Enabled && !s.HackerNews.Enabled &&
		!s.GitHubTrending.Enabled && len(s.Webpages) == 0 {
		return fmt.Errorf("sources 未启用任何数据源:请至少开启 rss/search/hackernews/github_trending 之一,或配置 webpages")
	}

	if err := validateUnique(topicNames(c.Topics), "topic"); err != nil {
		return err
	}
	for _, t := range c.Topics {
		if t.Name == "" {
			return fmt.Errorf("topic.name 不能为空")
		}
		if t.Prompt == "" {
			return fmt.Errorf("topic %q 的 prompt 不能为空:主题必须是一段 prompt", t.Name)
		}
	}

	if err := validateUnique(targetNames(c.Targets), "target"); err != nil {
		return err
	}
	for _, t := range c.Targets {
		if t.Name == "" {
			return fmt.Errorf("target.name 不能为空")
		}
		switch t.Type {
		case "email":
			if t.To == "" {
				return fmt.Errorf("target %q(type=email)缺少 to:收件邮箱", t.Name)
			}
		default:
			return fmt.Errorf("target %q 的 type 仅支持 email,当前为 %q", t.Name, t.Type)
		}
	}

	if err := validateUnique(taskNames(c.Tasks), "task"); err != nil {
		return err
	}
	topics := topicSet(c.Topics)
	targets := targetSet(c.Targets)
	for _, t := range c.Tasks {
		if t.Name == "" {
			return fmt.Errorf("task.name 不能为空")
		}
		if _, ok := topics[t.Topic]; !ok {
			return fmt.Errorf("task %q 引用了不存在的 topic %q", t.Name, t.Topic)
		}
		if len(t.Targets) == 0 {
			return fmt.Errorf("task %q 未配置任何 targets", t.Name)
		}
		seen := map[string]struct{}{}
		for _, ref := range t.Targets {
			if _, ok := targets[ref]; !ok {
				return fmt.Errorf("task %q 引用了不存在的 target %q", t.Name, ref)
			}
			if _, dup := seen[ref]; dup {
				return fmt.Errorf("task %q 的 targets 中 %q 重复", t.Name, ref)
			}
			seen[ref] = struct{}{}
		}
		if len(t.Schedules) == 0 {
			return fmt.Errorf("task %q 未配置任何 schedules", t.Name)
		}
		for _, sched := range t.Schedules {
			if _, err := cron.ParseStandard(sched); err != nil {
				return fmt.Errorf("task %q 的 cron 表达式 %q 不合法: %v", t.Name, sched, err)
			}
		}
	}

	if c.Sources.RSS.Enabled && len(c.Sources.RSS.Feeds) == 0 {
		return fmt.Errorf("sources.rss.enabled 为 true 时 feeds 不能为空")
	}
	for i, f := range c.Sources.RSS.Feeds {
		if f.URL == "" {
			return fmt.Errorf("sources.rss.feeds[%d].url 不能为空", i)
		}
	}
	for i, p := range c.Sources.Webpages {
		if p.URL == "" {
			return fmt.Errorf("sources.webpages[%d].url 不能为空", i)
		}
	}

	switch c.Mailer.Type {
	case "dryrun", "smtp":
	default:
		return fmt.Errorf("mailer.type 仅支持 dryrun / smtp,当前为 %q", c.Mailer.Type)
	}
	return nil
}

// TopicByName 按名称查找主题,找不到返回 nil。
func (c *Config) TopicByName(name string) *Topic {
	for i := range c.Topics {
		if c.Topics[i].Name == name {
			return &c.Topics[i]
		}
	}
	return nil
}

// TargetByName 按名称查找推送目标,找不到返回 nil。
func (c *Config) TargetByName(name string) *Target {
	for i := range c.Targets {
		if c.Targets[i].Name == name {
			return &c.Targets[i]
		}
	}
	return nil
}

// TaskByName 按名称查找任务,找不到返回 nil。
func (c *Config) TaskByName(name string) *Task {
	for i := range c.Tasks {
		if c.Tasks[i].Name == name {
			return &c.Tasks[i]
		}
	}
	return nil
}

// APIKey 从环境变量读取模型 API Key。
func (c *Config) APIKey() string {
	return os.Getenv(c.LLM.APIKeyEnv)
}

// HTTPTimeout 返回采集请求超时。
func (c *Config) HTTPTimeout() time.Duration {
	return time.Duration(c.Sources.Collect.HTTPTimeoutSec) * time.Second
}

// ---- 名称辅助 ----

func topicNames(ts []Topic) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Name
	}
	return out
}

func targetNames(ts []Target) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Name
	}
	return out
}

func taskNames(ts []Task) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.Name
	}
	return out
}

func topicSet(ts []Topic) map[string]struct{} {
	m := make(map[string]struct{}, len(ts))
	for _, t := range ts {
		m[t.Name] = struct{}{}
	}
	return m
}

func targetSet(ts []Target) map[string]struct{} {
	m := make(map[string]struct{}, len(ts))
	for _, t := range ts {
		m[t.Name] = struct{}{}
	}
	return m
}

// validateUnique 检查名称列表无重复、无空名。
func validateUnique(names []string, kind string) error {
	seen := map[string]struct{}{}
	for _, n := range names {
		if n == "" {
			return fmt.Errorf("%s.name 不能为空", kind)
		}
		if _, ok := seen[n]; ok {
			return fmt.Errorf("%s 名字重复: %q", kind, n)
		}
		seen[n] = struct{}{}
	}
	return nil
}
