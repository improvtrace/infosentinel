// Package agent 组装 Eino ReAct Agent:接入 LLM(OpenAI 兼容协议)与内置采集工具,
// 按主题组(Group,一个或多个主题)完成一轮"采集 → 筛选 → 总结"并产出报告内容。
// 采集信息源没有全局配置:五个工具常驻,RSS 地址等由主题提示词描述、LLM 传参;
// 订阅执行前先做意图分组(intent.go),每组作为一个子任务独立采集。
package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
	"github.com/tomasgdl/infosentinel/backend/pkg/tools"
)

// Agent 包装 Eino react.Agent,执行一轮面向特定主题组的情报收集。
type Agent struct {
	ra    *react.Agent
	group *Group
}

// NewChatModel 按全局 LLM 配置与已解析的 API Key 创建 OpenAI 兼容 ChatModel。
// API Key 由调用方通过 config.Resolver 解析(页面配置,加密存储)。
func NewChatModel(ctx context.Context, cfg *repository.Config, apiKey string) (*openai.ChatModel, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("模型 API Key 未配置:请在 Web 界面“配置 → 全局设置”中填写并加密保存")
	}
	cm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		BaseURL:     cfg.LLM.BaseURL,
		Model:       cfg.LLM.Model,
		APIKey:      apiKey,
		Temperature: &cfg.LLM.Temperature,
		Timeout:     time.Duration(cfg.LLM.TimeoutSec) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 ChatModel: %w", err)
	}
	return cm, nil
}

// New 按配置组装模型与工具集,创建面向主题组 g 的 ReAct Agent。
// toolList 由 BuildTools 构建,调用方(每任务)传入各自的去重状态。
func New(ctx context.Context, cfg *repository.Config, g *Group, apiKey string, toolList []tool.BaseTool) (*Agent, error) {
	if g == nil || len(g.Topics) == 0 {
		return nil, fmt.Errorf("主题组不能为空")
	}
	cm, err := NewChatModel(ctx, cfg, apiKey)
	if err != nil {
		return nil, err
	}

	ra, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: cm,
		ToolsConfig:      compose.ToolsNodeConfig{Tools: toolList},
		MaxStep:          cfg.LLM.MaxStep,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 react agent: %w", err)
	}
	return &Agent{ra: ra, group: g}, nil
}

// BuildTools 构建内置采集工具集:五个工具全部常驻,信息源由主题提示词描述。
// store 由调用方按任务注入(任务间状态隔离)。
func BuildTools(store *tools.SeenStore) ([]tool.BaseTool, error) {
	rss, err := tools.NewRSSTool(store)
	if err != nil {
		return nil, fmt.Errorf("fetch_rss: %w", err)
	}
	search, err := tools.NewSearchTool(store)
	if err != nil {
		return nil, fmt.Errorf("search_web: %w", err)
	}
	hn, err := tools.NewHackerNewsTool(store)
	if err != nil {
		return nil, fmt.Errorf("fetch_hackernews: %w", err)
	}
	trending, err := tools.NewGitHubTrendingTool(store)
	if err != nil {
		return nil, fmt.Errorf("fetch_github_trending: %w", err)
	}
	webpage, err := tools.NewWebpageTool()
	if err != nil {
		return nil, fmt.Errorf("fetch_webpage: %w", err)
	}
	return []tool.BaseTool{rss, search, hn, trending, webpage}, nil
}

// channelGuide 是系统提示词中的采集渠道说明(静态):工具清单、参数与调用建议。
const channelGuide = `## 可用采集渠道(五个工具常驻,按需调用)
- fetch_rss:拉取指定 RSS/Atom 源的近期文章(url = 源地址;主题指令给出的订阅源地址用它拉取)
- search_web:按关键词搜索近期网页资讯(query = 关键词;time_range 可限定 d 天 / w 周 / m 月)
- fetch_hackernews:Hacker News 热榜(科技/开源风向,含分数与讨论数;min_points / top_n 可调筛选阈值)
- fetch_github_trending:GitHub Trending 仓库(language 可按语言过滤)
- fetch_webpage:按 URL 抓取网页正文,用于深读高价值条目(只深读要点,控制调用次数)

`

// systemPrompt 构建本轮运行的系统提示词:
// 角色与目标 → 组意图(多主题组) → 主题指令 → 采集渠道 → 工作方法(策略/筛选/真实性) → 输出格式。
func systemPrompt(g *Group, now time.Time) string {
	var b strings.Builder
	b.WriteString("你是一名严谨的个人信息情报员,擅长从公开信息渠道筛选高价值资讯。\n")
	b.WriteString("本轮任务:围绕给定主题完成一次「采集 → 筛选 → 总结」,产出一份可直接阅读的资讯报告。\n\n")

	// 多主题组:先给出本组共同意图,主题指令随后逐主题分列
	if len(g.Topics) > 1 {
		b.WriteString(fmt.Sprintf("## 本组共同意图(由 %d 个意图相近的主题合并采集)\n", len(g.Topics)))
		if g.Goal != "" {
			b.WriteString(g.Goal + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(topicDirective(g))

	b.WriteString(channelGuide)

	b.WriteString(fmt.Sprintf(`## 工作方法
1. 先广度后深度:先用多个渠道并行采集(主题指令给出的订阅源地址用 fetch_rss 拉取),
   再挑选最有价值的少数条目用 fetch_webpage 深读原文后总结。
2. 注意步数预算:工具调用要克制(广度采集 3-5 次、深读 1-3 次),避免无效重试,尽快产出报告。
3. 交叉去重:同一事件被多个渠道报道时合并为一条,注明信息最全的来源。
4. 筛选标准(按优先级):与主题相关 > 时间新近(今天是 %s,48 小时内优先)> 信息密度
   (有事实、数据、发布物,而非营销话术)> 来源可靠且多样(避免全部出自同一站点)。
5. 渠道返回为空或全部被去重过滤属正常现象(此前轮次可能已采集过),不要换词反复重试;
   及时转向其他渠道或直接进入总结。
6. 严格基于工具返回的真实内容撰写,禁止编造、臆测或脑补细节;
   报告中的链接必须原样取自工具返回结果,不得自行拼改 URL。

## 输出格式(严格遵守)
第一行输出报告标题:纯文本一行,概括本期最有价值的看点,不超过 40 字,不含"#"等标记。
从第二行起输出 Markdown 正文:
- 按主题指令要求的结构分节(## 二级标题);每条要点 = 一句话摘要 + 原文链接,
  重要条目可展开 2-3 句;结尾以"## 本期小结"收束(3 句以内)。
- 篇幅控制在 40 行以内,宁缺毋滥;确实没有合适内容时如实说明原因,不要硬凑。
`, now.Format("2006-01-02 (MST)")))
	return b.String()
}

// topicDirective 渲染主题指令段(取舍的最高依据):单主题组与历史格式一致;
// 多主题组逐主题分列,要求一次采集同时覆盖全部主题。
func topicDirective(g *Group) string {
	var b strings.Builder
	const basis = "以下是用户为本主题编写的指令,是本轮采集与取舍的最高依据;与主题无关的内容一律不收录。\n"
	if len(g.Topics) == 1 {
		fmt.Fprintf(&b, "## 主题指令:%s\n%s%s\n\n", g.Topics[0].Name, basis, g.Topics[0].Prompt)
		return b.String()
	}
	b.WriteString("## 主题指令(本组包含多个主题,采集与取舍须同时覆盖全部主题)\n")
	for _, t := range g.Topics {
		fmt.Fprintf(&b, "### 主题:%s\n%s%s\n\n", t.Name, basis, t.Prompt)
	}
	return b.String()
}

// Digest 是一轮运行的产出。
type Digest struct {
	Subject string // 报告标题(不含通道前缀)
	Body    string // Markdown 正文
}

// Run 执行一轮完整的采集与总结。
func (a *Agent) Run(ctx context.Context) (*Digest, error) {
	now := time.Now()
	out, err := a.ra.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: systemPrompt(a.group, now)},
		{Role: schema.User, Content: kickoffMessage(a.group, now)},
	})
	if err != nil {
		return nil, fmt.Errorf("agent 执行失败: %w", err)
	}

	subject, body := splitSubjectBody(out.Content)
	return &Digest{Subject: subject, Body: body}, nil
}

// kickoffMessage 构建启动本轮采集的用户消息:单主题组沿用主题口径;
// 多主题组列出组内全部主题。
func kickoffMessage(g *Group, now time.Time) string {
	if len(g.Topics) > 1 {
		names := make([]string, 0, len(g.Topics))
		for _, t := range g.Topics {
			names = append(names, t.Name)
		}
		return fmt.Sprintf("开始执行分组「%s」的本轮资讯收集(覆盖主题:%s)。当前时间:%s。请按工作方法采集与筛选,并严格按输出格式交付报告。",
			g.Name, strings.Join(names, "、"), now.Format("2006-01-02 15:04 MST"))
	}
	return fmt.Sprintf("开始执行主题「%s」的本轮资讯收集。当前时间:%s。请按工作方法采集与筛选,并严格按输出格式交付报告。",
		g.Name, now.Format("2006-01-02 15:04 MST"))
}

// splitSubjectBody 从 LLM 输出中拆出标题(首行)与正文。
func splitSubjectBody(content string) (subject, body string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "今日资讯", "本轮未产出内容。"
	}
	lines := strings.SplitN(content, "\n", 2)
	subject = strings.TrimSpace(lines[0])
	subject = strings.TrimPrefix(subject, "#")
	subject = strings.TrimSpace(subject)
	body = ""
	if len(lines) > 1 {
		body = strings.TrimSpace(lines[1])
	}
	if subject == "" {
		subject = "今日资讯"
	}
	return subject, body
}
