// model.go 是数据库业务配置的模型层:类型定义、校验与查找辅助。
//
// 业务配置(sources/topics/targets/subscriptions/llm/mailer/server)全部存于 SQLite,
// 本文件提供运行时结构与校验;内置默认值由 repository/migrate 在迁移时初始化。
package repository

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/tomasgdl/infosentinel/backend/pkg/storage"
)

// Config 是 infosentinel 的运行时顶层业务配置(由数据库组装)。
type Config struct {
	// Sources 是公用数据源池。
	Sources Sources `json:"sources"`

	// Topics 是主题列表,每项一段 prompt。
	Topics []Topic `json:"topics"`

	// Targets 是推送目标(通道)列表。
	Targets []Target `json:"targets"`

	// Subscriptions 是订阅列表:{主题, cron, 推送目标};主题即订阅唯一标识。
	Subscriptions []Subscription `json:"subscriptions"`

	// Server 是 serve 模式的 Web 管理界面配置。
	Server ServerConfig `json:"server"`

	// LLM 是模型服务配置(OpenAI 兼容协议),全局共用。
	LLM LLMConfig `json:"llm"`

	// Mailer 是发送器全局配置(站内信/smtp)。
	Mailer MailerConfig `json:"mailer"`

	// Storage 是产物空间全局配置(本地磁盘/SFTP/FTP 驱动 + 分桶保留策略)。
	Storage StorageConfig `json:"storage"`
}

// Finalize 做完整校验(供从数据库组装的配置使用);先归一化历史遗留值。
func (c *Config) Finalize() error {
	c.Mailer.normalizeLegacy() // 旧库中的 dryrun 映射为站内信
	c.Storage.finalize()       // 缺省补 local 与默认分桶参数(兼容无 storage 段的旧库)
	return c.Validate()
}

// Sources 是公用数据源池:每类源对应一个内置工具,同类多实例共用该工具。
type Sources struct {
	// Collect 是采集通用参数。
	Collect CollectConfig `json:"collect"`

	// RSS 是 RSS/Atom 订阅源(fetch_rss 工具)。
	RSS RSSConfig `json:"rss"`

	// Search 是 DuckDuckGo 搜索(search_web 工具)。
	Search SearchConfig `json:"search"`

	// HackerNews 是 HN 热榜(fetch_hackernews 工具)。
	HackerNews HackerNewsConfig `json:"hackernews"`

	// GitHubTrending 是 GitHub Trending(fetch_github_trending 工具)。
	GitHubTrending GitHubTrendingConfig `json:"github_trending"`

	// Webpages 是固定网页列表(fetch_webpage 工具按 URL 抓取),非空即启用。
	Webpages []Webpage `json:"webpages"`
}

// CollectConfig 是采集通用参数。
type CollectConfig struct {
	// WindowHours 是内容时间窗:只保留最近 N 小时内发布/更新的条目(RSS/HN)。
	WindowHours int `json:"window_hours"`
	// MaxItemsPerSource 是单个工具单次返回的最大条目数,控制上下文长度。
	MaxItemsPerSource int `json:"max_items_per_source"`
	// HTTPTimeoutSec 是采集请求的超时秒数。
	HTTPTimeoutSec int `json:"http_timeout_sec"`
	// MaxBodyChars 是单条网页正文的截断字符数。
	MaxBodyChars int `json:"max_body_chars"`
	// DedupDays 是 URL 去重状态保留天数,超过则重新允许出现。
	DedupDays int `json:"dedup_days"`
}

// RSSConfig 是 RSS 订阅源配置。
type RSSConfig struct {
	Enabled bool      `json:"enabled"`
	Feeds   []RSSFeed `json:"feeds"`
}

// RSSFeed 是单个 RSS/Atom 源。
type RSSFeed struct {
	// Name 是源的可读名称,展示给 LLM 与邮件读者。
	Name string `json:"name"`
	// URL 是 RSS/Atom 地址。
	URL string `json:"url"`
}

// SearchConfig 是搜索引擎(DuckDuckGo)配置。
type SearchConfig struct {
	Enabled bool   `json:"enabled"`
	Region  string `json:"region"` // ddgsearch region,如 cn-zh / us-en / wt-wt
	// MaxResults 是单次搜索返回的最大结果数。
	MaxResults int `json:"max_results"`
}

// HackerNewsConfig 是 Hacker News 热榜配置。
type HackerNewsConfig struct {
	Enabled bool `json:"enabled"`
	// TopN 是从热榜取前 N 条。
	TopN int `json:"top_n"`
	// MinPoints 是最低分阈值,过滤低热度条目。
	MinPoints int `json:"min_points"`
}

// GitHubTrendingConfig 是 GitHub Trending 配置。
type GitHubTrendingConfig struct {
	Enabled bool `json:"enabled"`
	// Language 是过滤的编程语言,空表示全部。
	Language string `json:"language"`
	// TopN 是取前 N 个仓库。
	TopN int `json:"top_n"`
}

// Webpage 是固定抓取的网页。
type Webpage struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Topic 是主题:一段外部撰写的 prompt,agent 据此采集与生成报告。
type Topic struct {
	// Name 是主题唯一标识,供订阅引用。
	Name string `json:"name"`
	// Prompt 是主题指令:关注什么、如何取舍、报告怎么写。
	Prompt string `json:"prompt"`
}

// Target 是推送目标(通道)。当前支持 email;后续可扩展 webhook 等。
type Target struct {
	// Name 是目标唯一标识,供订阅引用。
	Name string `json:"name"`
	// Type 是通道类型,当前仅支持 "email"。
	Type string `json:"type"`
	// To 是收件邮箱(email 通道必填)。
	To string `json:"to"`
	// SubjectPrefix 是邮件标题前缀。
	SubjectPrefix string `json:"subject_prefix"`
}

// Subscription 是订阅:{主题, cron, 推送目标}。
// 主题即订阅的唯一标识:一个主题最多一条订阅,也用于隔离去重状态与产物分桶。
type Subscription struct {
	// Topic 引用主题名(唯一)。
	Topic string `json:"topic"`
	// Cron 是单个 cron 表达式(五段),订阅的定时调度。
	Cron string `json:"cron"`
	// Targets 引用推送目标名,可多个。
	Targets []string `json:"targets"`
}

// ServerConfig 是 serve 模式的 Web 管理界面配置。
type ServerConfig struct {
	// Enabled 是否启用;未配置时默认开启(nil → true)。
	Enabled *bool `json:"enabled"`
	// Addr 是监听地址;默认仅本机 127.0.0.1:8080。
	Addr string `json:"addr"`
}

// IsEnabled 返回是否启用 Web 管理界面(未配置视为启用)。
func (s ServerConfig) IsEnabled() bool { return s.Enabled == nil || *s.Enabled }

// LLMConfig 是 OpenAI 兼容模型服务配置。
type LLMConfig struct {
	// BaseURL 是 API 根地址,例如 https://api.deepseek.com 或 https://open.bigmodel.cn/api/paas/v4。
	BaseURL string `json:"base_url"`
	// Model 是模型名,例如 deepseek-chat。
	Model string `json:"model"`
	// Temperature 是采样温度。
	Temperature float32 `json:"temperature"`
	// TimeoutSec 是单次模型请求超时。
	TimeoutSec int `json:"timeout_sec"`
	// MaxStep 是 agent 循环的最大步数(防止失控)。
	MaxStep int `json:"max_step"`
}

// 发送器投递类型:站内信(报告写入产物空间,界面在线查看)或 SMTP 真实发送邮件。
const (
	MailerTypeInbox = "站内信"
	MailerTypeSMTP  = "smtp"
)

// MailerConfig 是发送器全局配置。
type MailerConfig struct {
	// Type 是投递类型:站内信(报告写入产物空间,界面在线查看)或 smtp(内置发件邮箱真实发送)。
	Type string `json:"type"`
	// Smtp 是内置发件邮箱(SMTP 账号)配置,type=smtp 时必须配置 host 与 from。
	Smtp SmtpConfig `json:"smtp"`
}

// normalizeLegacy 归一化历史遗留的投递类型值(旧库中的 dryrun → 站内信)。
func (m *MailerConfig) normalizeLegacy() {
	if m.Type == "dryrun" {
		m.Type = MailerTypeInbox
	}
}

// SmtpConfig 是内置 SMTP 发件邮箱配置(服务内置的发送通道)。
type SmtpConfig struct {
	// Host 是 SMTP 服务地址,如 smtp.qq.com / smtp.163.com / smtp.gmail.com。
	Host string `json:"host"`
	// Port 是 SMTP 端口;默认 465(隐式 TLS)。587 通常为 STARTTLS,配合 ssl: false。
	Port int `json:"port"`
	// Username 是 SMTP 登录用户名,通常是发件邮箱本身。
	Username string `json:"username"`
	// From 是发件人地址(展示在邮件 From 头)。
	From string `json:"from"`
	// SSL 是否使用隐式 TLS;nil 时按端口判断:465 → true,其余 → false(自动尝试 STARTTLS)。
	SSL *bool `json:"ssl"`
}

// StorageConfig 是产物空间全局配置。
type StorageConfig struct {
	// Type 是存储驱动类型:local(本地磁盘)/ sftp / ftp。
	Type string `json:"type"`
	// Path 是产物路径(页面配置,相对存储根目录;空为根目录)。
	// 本地磁盘实际路径 = 引导配置 storage.base_dir(绝对路径) + 本路径。
	Path string `json:"path"`
	// PeriodDays 是分桶周期:每 m 天的产物保存到同一个子目录(以周期起始日期命名)。
	PeriodDays int `json:"period_days"`
	// MaxDirs 是最多保留的分桶子目录数,超出自动删除最旧的。
	MaxDirs int `json:"max_dirs"`
	// SFTP 是 type=sftp 时的连接配置。
	SFTP StorageSFTP `json:"sftp"`
	// FTP 是 type=ftp 时的连接配置。
	FTP StorageFTP `json:"ftp"`
}

// StorageSFTP 是 SFTP 驱动配置。
type StorageSFTP struct {
	// Host 是 SFTP 服务地址。
	Host string `json:"host"`
	// Port 是端口;0 取默认 22。
	Port int `json:"port"`
	// Username 是登录用户名。
	Username string `json:"username"`
	// KeyPath 是私钥文件路径(可选,与密码二选一或并存)。
	KeyPath string `json:"key_path"`
	// HostKey 是期望的远端公钥(authorized_keys 单行格式;空则不校验指纹)。
	HostKey string `json:"host_key"`
	// BaseDir 是远端基础目录(空为登录目录)。
	BaseDir string `json:"base_dir"`
}

// StorageFTP 是 FTP 驱动配置。
type StorageFTP struct {
	// Host 是 FTP 服务地址。
	Host string `json:"host"`
	// Port 是端口;0 取默认 21。
	Port int `json:"port"`
	// Username 是登录用户名(空按匿名)。
	Username string `json:"username"`
	// BaseDir 是远端基础目录(空为登录目录)。
	BaseDir string `json:"base_dir"`
	// TLS 是否使用 FTPS(显式 TLS)。
	TLS bool `json:"tls"`
}

// 产物空间存储驱动类型。
const (
	StorageTypeLocal = "local"
	StorageTypeSFTP  = "sftp"
	StorageTypeFTP   = "ftp"
)

// finalize 补齐产物空间配置缺省值:类型缺省 local,分桶参数缺省 1 天 / 30 个。
func (s *StorageConfig) finalize() {
	if s.Type == "" {
		s.Type = StorageTypeLocal
	}
	if s.PeriodDays < 1 {
		s.PeriodDays = 1
	}
	if s.MaxDirs < 1 {
		s.MaxDirs = 30
	}
}

// Validate 校验产物空间配置。
func (s *StorageConfig) Validate() error {
	switch s.Type {
	case StorageTypeLocal, StorageTypeSFTP, StorageTypeFTP:
	default:
		return fmt.Errorf("storage.type 仅支持 local / sftp / ftp,当前为 %q", s.Type)
	}
	if _, err := storage.CleanRel(s.Path); err != nil {
		return fmt.Errorf("storage.path 不合法: %w", err)
	}
	if s.PeriodDays < 1 {
		return fmt.Errorf("storage.period_days 必须 >= 1,当前为 %d", s.PeriodDays)
	}
	if s.MaxDirs < 1 {
		return fmt.Errorf("storage.max_dirs 必须 >= 1,当前为 %d", s.MaxDirs)
	}
	if s.Type == StorageTypeSFTP && s.SFTP.Host == "" {
		return fmt.Errorf("storage.type=sftp 时必须配置 storage.sftp.host")
	}
	if s.Type == StorageTypeFTP && s.FTP.Host == "" {
		return fmt.Errorf("storage.type=ftp 时必须配置 storage.ftp.host")
	}
	return nil
}

// Validate 检查配置合法性,返回发现的第一个问题。
// 数据库内容由 migrate 内置种子与 Web 界面维护,这里只校验不填充默认值。
func (c *Config) Validate() error {
	if len(c.Topics) == 0 {
		return fmt.Errorf("topics 不能为空:请至少定义一个主题(一段 prompt)")
	}
	if len(c.Targets) == 0 {
		return fmt.Errorf("targets 不能为空:请至少配置一个推送目标")
	}

	// 数据源
	if err := c.Sources.Validate(); err != nil {
		return err
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

	// 订阅:允许为空(全部退订是合法状态),但引用必须完整
	if err := validateUnique(subscriptionTopics(c.Subscriptions), "subscription"); err != nil {
		return err
	}
	topics := topicSet(c.Topics)
	targets := targetSet(c.Targets)
	for _, s := range c.Subscriptions {
		if s.Topic == "" {
			return fmt.Errorf("subscription.topic 不能为空")
		}
		if _, ok := topics[s.Topic]; !ok {
			return fmt.Errorf("subscription 引用了不存在的 topic %q", s.Topic)
		}
		if len(s.Targets) == 0 {
			return fmt.Errorf("订阅 %q 未配置任何 targets", s.Topic)
		}
		seen := map[string]struct{}{}
		for _, ref := range s.Targets {
			if _, ok := targets[ref]; !ok {
				return fmt.Errorf("订阅 %q 引用了不存在的 target %q", s.Topic, ref)
			}
			if _, dup := seen[ref]; dup {
				return fmt.Errorf("订阅 %q 的 targets 中 %q 重复", s.Topic, ref)
			}
			seen[ref] = struct{}{}
		}
		if _, err := cron.ParseStandard(s.Cron); err != nil {
			return fmt.Errorf("订阅 %q 的 cron 表达式 %q 不合法: %v", s.Topic, s.Cron, err)
		}
	}

	// 模型服务:零值会让 agent 在任务执行时才失败,这里显式拦截
	if c.LLM.BaseURL == "" {
		return fmt.Errorf("llm.base_url 不能为空:OpenAI 兼容 API 根地址")
	}
	if c.LLM.Model == "" {
		return fmt.Errorf("llm.model 不能为空:模型名")
	}
	if c.LLM.TimeoutSec <= 0 {
		return fmt.Errorf("llm.timeout_sec 必须大于 0")
	}
	if c.LLM.MaxStep <= 0 {
		return fmt.Errorf("llm.max_step 必须大于 0")
	}

	if err := c.Mailer.Validate(); err != nil {
		return err
	}
	if err := c.Storage.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate 校验数据源配置。
func (s *Sources) Validate() error {
	if !s.RSS.Enabled && !s.Search.Enabled && !s.HackerNews.Enabled &&
		!s.GitHubTrending.Enabled && len(s.Webpages) == 0 {
		return fmt.Errorf("sources 未启用任何数据源:请至少开启 rss/search/hackernews/github_trending 之一,或配置 webpages")
	}
	if s.RSS.Enabled && len(s.RSS.Feeds) == 0 {
		return fmt.Errorf("sources.rss.enabled 为 true 时 feeds 不能为空")
	}
	for i, f := range s.RSS.Feeds {
		if f.URL == "" {
			return fmt.Errorf("sources.rss.feeds[%d].url 不能为空", i)
		}
	}
	for i, p := range s.Webpages {
		if p.URL == "" {
			return fmt.Errorf("sources.webpages[%d].url 不能为空", i)
		}
	}
	return nil
}

// Validate 校验发送器配置。
func (m *MailerConfig) Validate() error {
	switch m.Type {
	case MailerTypeInbox, MailerTypeSMTP:
	default:
		return fmt.Errorf("mailer.type 仅支持 站内信 / smtp,当前为 %q", m.Type)
	}
	if m.Type == MailerTypeSMTP && (m.Smtp.Host == "" || m.Smtp.From == "") {
		return fmt.Errorf("mailer.type=smtp 时必须配置 mailer.smtp.host 与 mailer.smtp.from")
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

// SubscriptionByTopic 按主题查找订阅,找不到返回 nil。
func (c *Config) SubscriptionByTopic(topic string) *Subscription {
	for i := range c.Subscriptions {
		if c.Subscriptions[i].Topic == topic {
			return &c.Subscriptions[i]
		}
	}
	return nil
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

func subscriptionTopics(ss []Subscription) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.Topic
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
