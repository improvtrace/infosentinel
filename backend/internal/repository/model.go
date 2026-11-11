// model.go 是数据库业务配置的模型层:类型定义、校验与查找辅助。
//
// 业务配置(topics/targets/subscriptions/llm/mailer/server/storage)全部存于 SQLite,
// 本文件提供运行时结构与校验;内置默认值由 repository/migrate 在迁移时初始化。
// 采集信息源没有全局配置:五个内置工具常驻,源地址等由主题提示词描述。
package repository

import (
	"fmt"

	"github.com/robfig/cron/v3"

	"github.com/tomasgdl/infosentinel/backend/pkg/storage"
)

// Config 是 infosentinel 的运行时顶层业务配置(由数据库组装)。
type Config struct {
	// Topics 是主题列表,每项一段 prompt。
	Topics []Topic `json:"topics"`

	// Targets 是推送目标(通道)列表。
	Targets []Target `json:"targets"`

	// Subscriptions 是订阅列表:{名称, 主题列表, cron, 推送目标, 产物空间}。
	Subscriptions []Subscription `json:"subscriptions"`

	// Server 是 serve 模式的 Web 管理界面配置。
	Server ServerConfig `json:"server"`

	// LLM 是模型服务配置(OpenAI 兼容协议),全局共用。
	LLM LLMConfig `json:"llm"`

	// Mailer 是发送器全局配置(站内信/smtp)。
	Mailer MailerConfig `json:"mailer"`

	// Storage 是产物空间全局配置:可同时配置多个空间(本地磁盘/SFTP/FTP),订阅按名引用。
	Storage StorageConfig `json:"storage"`
}

// Finalize 做完整校验(供从数据库组装的配置使用);先归一化历史遗留值。
func (c *Config) Finalize() error {
	c.Mailer.normalizeLegacy() // 旧库中的 dryrun 映射为站内信
	c.Storage.finalize()       // 缺省补 local 与默认分桶参数(兼容无 storage 段的旧库)
	return c.Validate()
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

// Subscription 是订阅:{名称, 主题列表, 合成提示词, cron, 推送目标, 产物空间}。
// 名称即订阅的唯一标识;订阅与主题多对多(一个订阅可关联多个主题,一个主题
// 可被多个订阅引用),与产物空间一对一(不选则站内信无投递目标)。
type Subscription struct {
	// Name 是订阅唯一标识。
	Name string `json:"name"`
	// Topics 引用主题名列表(每个主题独立采集与去重,报告最终合并为一份)。
	Topics []string `json:"topics"`
	// Synthesis 是合成提示词:多主题报告合并为一份最终报告的指令。
	Synthesis string `json:"synthesis"`
	// Cron 是单个 cron 表达式(五段),订阅的定时调度。
	Cron string `json:"cron"`
	// Targets 引用推送目标名,可多个。
	Targets []string `json:"targets"`
	// Space 引用产物空间名(单个);空表示不写产物空间(纯 SMTP 推送)。
	Space string `json:"space"`
}

// DefaultSynthesis 是合成提示词的默认值(新建订阅为空时回填)。
// 默认让合成报告直接输出 HTML 正文片段:推送与产物最终都是 HTML,
// 由模型直接产出结构化片段可获得比 Markdown 转换更丰富的排版。
const DefaultSynthesis = "你是资讯编辑。请把多份主题报告合并为一份最终报告:去除重复与冗余,按主题分节或按重要性排序,保留每条的原文链接与一句话摘要,开头用一段导语概括本期整体看点。第一行只写总标题纯文本;从第二行起用中文 HTML 片段撰写正文(<section>/<h2>/<p>/<ul>/<table> 等内容级标签,不含 <html>/<body>/<style>/<script>),链接用 <a> 标签原样保留。"

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

// DefaultSpaceName 是产物空间的缺省空间名(旧库包装与空配置回填时使用)。
const DefaultSpaceName = "默认空间"

// StorageConfig 是产物空间全局配置:可同时配置多个空间,订阅按名引用其中之一。
type StorageConfig struct {
	// Spaces 是产物空间列表。
	Spaces []StorageSpace `json:"spaces"`

	// 以下扁平字段为旧库单空间 JSON 的兼容读取(omitempty:序列化时不输出;
	// 加载后由 finalize 包装进 Spaces 并清空),新配置不应再写入这些字段。
	Type       string      `json:"type,omitempty"`
	Path       string      `json:"path,omitempty"`
	PeriodDays int         `json:"period_days,omitempty"`
	MaxDirs    int         `json:"max_dirs,omitempty"`
	SFTP       StorageSFTP `json:"sftp,omitempty"`
	FTP        StorageFTP  `json:"ftp,omitempty"`
}

// StorageSpace 是一个产物空间配置(存储驱动 + 分桶保留策略)。
type StorageSpace struct {
	// Name 是空间唯一标识,供订阅引用。
	Name string `json:"name"`
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

// finalize 补齐产物空间配置:旧库单空间扁平 JSON 包装为一个空间;无任何空间时
// 回填一个本地默认空间(与旧版"缺省补 local"行为一致);逐空间补缺省值。
// 兼容扁平字段在包装后清空,持久化时只保留 Spaces 列表。
func (s *StorageConfig) finalize() {
	if len(s.Spaces) == 0 && s.hasLegacy() {
		s.Spaces = []StorageSpace{{
			Name:       DefaultSpaceName,
			Type:       s.Type,
			Path:       s.Path,
			PeriodDays: s.PeriodDays,
			MaxDirs:    s.MaxDirs,
			SFTP:       s.SFTP,
			FTP:        s.FTP,
		}}
	}
	s.Type, s.Path, s.PeriodDays, s.MaxDirs = "", "", 0, 0
	s.SFTP, s.FTP = StorageSFTP{}, StorageFTP{}
	if len(s.Spaces) == 0 {
		s.Spaces = []StorageSpace{{Name: DefaultSpaceName, Type: StorageTypeLocal}}
	}
	for i := range s.Spaces {
		s.Spaces[i].finalize()
	}
}

// hasLegacy 判断是否存在旧库单空间扁平配置(非零即视为有)。
func (s *StorageConfig) hasLegacy() bool {
	return s.Type != "" || s.Path != "" || s.PeriodDays > 0 || s.MaxDirs > 0
}

// finalize 补齐单个产物空间的缺省值:类型缺省 local,分桶参数缺省 1 天 / 30 个。
func (s *StorageSpace) finalize() {
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

// Validate 校验产物空间全局配置。
func (s *StorageConfig) Validate() error {
	if len(s.Spaces) == 0 {
		return fmt.Errorf("storage.spaces 不能为空:请至少配置一个产物空间")
	}
	if err := validateUnique(spaceNames(s.Spaces), "storage.space"); err != nil {
		return err
	}
	for i := range s.Spaces {
		if err := s.Spaces[i].Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Validate 校验单个产物空间配置。
func (s *StorageSpace) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("storage.space.name 不能为空")
	}
	switch s.Type {
	case StorageTypeLocal, StorageTypeSFTP, StorageTypeFTP:
	default:
		return fmt.Errorf("产物空间 %q 的 type 仅支持 local / sftp / ftp,当前为 %q", s.Name, s.Type)
	}
	if _, err := storage.CleanRel(s.Path); err != nil {
		return fmt.Errorf("产物空间 %q 的 path 不合法: %w", s.Name, err)
	}
	if s.PeriodDays < 1 {
		return fmt.Errorf("产物空间 %q 的 period_days 必须 >= 1,当前为 %d", s.Name, s.PeriodDays)
	}
	if s.MaxDirs < 1 {
		return fmt.Errorf("产物空间 %q 的 max_dirs 必须 >= 1,当前为 %d", s.Name, s.MaxDirs)
	}
	if s.Type == StorageTypeSFTP && s.SFTP.Host == "" {
		return fmt.Errorf("产物空间 %q(type=sftp)必须配置 sftp.host", s.Name)
	}
	if s.Type == StorageTypeFTP && s.FTP.Host == "" {
		return fmt.Errorf("产物空间 %q(type=ftp)必须配置 ftp.host", s.Name)
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

	// 订阅:允许为空(全部退订是合法状态);名称唯一,主题/推送目标/产物空间引用必须完整。
	// 订阅与主题多对多:同一主题可被多个订阅引用。
	if err := validateUnique(subscriptionNames(c.Subscriptions), "subscription"); err != nil {
		return err
	}
	spaces := spaceSet(c.Storage.Spaces)
	topics := topicSet(c.Topics)
	targets := targetSet(c.Targets)
	for _, s := range c.Subscriptions {
		if len(s.Topics) == 0 {
			return fmt.Errorf("订阅 %q 未关联任何主题", s.Name)
		}
		seenTopic := map[string]struct{}{}
		for _, ref := range s.Topics {
			if _, ok := topics[ref]; !ok {
				return fmt.Errorf("订阅 %q 引用了不存在的 topic %q", s.Name, ref)
			}
			if _, dup := seenTopic[ref]; dup {
				return fmt.Errorf("订阅 %q 的 topics 中 %q 重复", s.Name, ref)
			}
			seenTopic[ref] = struct{}{}
		}
		if s.Synthesis == "" {
			return fmt.Errorf("订阅 %q 的 synthesis 不能为空:多主题报告需合成提示词合并", s.Name)
		}
		if len(s.Targets) == 0 {
			return fmt.Errorf("订阅 %q 未配置任何 targets", s.Name)
		}
		seen := map[string]struct{}{}
		for _, ref := range s.Targets {
			if _, ok := targets[ref]; !ok {
				return fmt.Errorf("订阅 %q 引用了不存在的 target %q", s.Name, ref)
			}
			if _, dup := seen[ref]; dup {
				return fmt.Errorf("订阅 %q 的 targets 中 %q 重复", s.Name, ref)
			}
			seen[ref] = struct{}{}
		}
		if s.Space != "" {
			if _, ok := spaces[s.Space]; !ok {
				return fmt.Errorf("订阅 %q 引用了不存在的产物空间 %q", s.Name, s.Space)
			}
		}
		if _, err := cron.ParseStandard(s.Cron); err != nil {
			return fmt.Errorf("订阅 %q 的 cron 表达式 %q 不合法: %v", s.Name, s.Cron, err)
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

// SubscriptionByName 按名称查找订阅,找不到返回 nil。
func (c *Config) SubscriptionByName(name string) *Subscription {
	for i := range c.Subscriptions {
		if c.Subscriptions[i].Name == name {
			return &c.Subscriptions[i]
		}
	}
	return nil
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

func subscriptionNames(ss []Subscription) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.Name
	}
	return out
}

func spaceNames(ss []StorageSpace) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.Name
	}
	return out
}

func spaceSet(ss []StorageSpace) map[string]struct{} {
	m := make(map[string]struct{}, len(ss))
	for _, s := range ss {
		m[s.Name] = struct{}{}
	}
	return m
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
