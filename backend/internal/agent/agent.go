// Package agent 组装 Eino ReAct Agent:接入 LLM(OpenAI 兼容协议)与公用采集工具,
// 按主题(topic,一段外部 prompt)完成一轮"采集 → 筛选 → 总结"并产出报告内容。
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
	"github.com/tomasgdl/infosentinel/backend/internal/tools"
)

// Channel 描述一个已启用的采集渠道,用于在系统提示词中向 LLM 说明可用工具。
type Channel struct {
	Name string // 工具名,如 fetch_rss
	Hint string // 用途与参数提示(含已配置的实例清单)
}

// Agent 包装 Eino react.Agent,执行一轮面向特定主题的情报收集。
type Agent struct {
	ra       *react.Agent
	topic    *repository.Topic
	channels []Channel
}

// NewChatModel 按全局 LLM 配置与已解析的 API Key 创建 OpenAI 兼容 ChatModel。
// API Key 由调用方通过 secret.Resolver 解析(环境变量优先,其次数据库密文)。
func NewChatModel(ctx context.Context, cfg *repository.Config, apiKey string) (*openai.ChatModel, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("模型 API Key 为空:请设置环境变量 %s 或在 Web 界面填写", cfg.LLM.APIKeyEnv)
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

// New 按配置组装模型并创建面向 topic 的 ReAct Agent。
// toolList/channels 由 BuildTools 从公用数据源构建,调用方(每任务)传入各自的去重状态。
func New(ctx context.Context, cfg *repository.Config, topic *repository.Topic, apiKey string, toolList []tool.BaseTool, channels []Channel) (*Agent, error) {
	if topic == nil {
		return nil, fmt.Errorf("topic 不能为空")
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
	return &Agent{ra: ra, topic: topic, channels: channels}, nil
}

// BuildTools 从公用数据源配置构建工具列表与渠道说明。
// 同类数据源共用一个工具;store 由调用方按任务注入(任务间状态隔离)。
func BuildTools(src *repository.Sources, store *tools.SeenStore) ([]tool.BaseTool, []Channel, error) {
	var (
		list     []tool.BaseTool
		channels []Channel
	)

	if src.RSS.Enabled {
		t, err := tools.NewRSSTool(&src.RSS, &src.Collect, store)
		if err != nil {
			return nil, nil, fmt.Errorf("fetch_rss: %w", err)
		}
		names := make([]string, 0, len(src.RSS.Feeds))
		for _, f := range src.RSS.Feeds {
			names = append(names, f.Name)
		}
		list = append(list, t)
		channels = append(channels, Channel{
			Name: "fetch_rss",
			Hint: fmt.Sprintf("拉取订阅源近期新文章;可用源:%s(可用 feed 参数指定,留空拉取全部)", strings.Join(names, ", ")),
		})
	}
	if src.Search.Enabled {
		t, err := tools.NewSearchTool(&src.Search, store)
		if err != nil {
			return nil, nil, fmt.Errorf("search_web: %w", err)
		}
		list = append(list, t)
		channels = append(channels, Channel{
			Name: "search_web",
			Hint: fmt.Sprintf("按关键词搜索近期网页资讯(区域 %s)", src.Search.Region),
		})
	}
	if src.HackerNews.Enabled {
		t, err := tools.NewHackerNewsTool(&src.HackerNews, &src.Collect, store)
		if err != nil {
			return nil, nil, fmt.Errorf("fetch_hackernews: %w", err)
		}
		list = append(list, t)
		channels = append(channels, Channel{
			Name: "fetch_hackernews",
			Hint: "Hacker News 热榜(科技/开源风向,含分数与讨论数)",
		})
	}
	if src.GitHubTrending.Enabled {
		t, err := tools.NewGitHubTrendingTool(&src.GitHubTrending, &src.Collect, store)
		if err != nil {
			return nil, nil, fmt.Errorf("fetch_github_trending: %w", err)
		}
		list = append(list, t)
		channels = append(channels, Channel{
			Name: "fetch_github_trending",
			Hint: fmt.Sprintf("GitHub Trending 仓库(默认语言:%s)", orDefault(src.GitHubTrending.Language, "不过滤")),
		})
	}
	if len(src.Webpages) > 0 {
		t, err := tools.NewWebpageTool(src.Webpages, &src.Collect)
		if err != nil {
			return nil, nil, fmt.Errorf("fetch_webpage: %w", err)
		}
		list = append(list, t)
		channels = append(channels, Channel{
			Name: "fetch_webpage",
			Hint: "按 URL 抓取网页正文,用于深读重点条目;常读页面清单:\n" + tools.DefaultWebpages(src.Webpages),
		})
	}

	if len(list) == 0 {
		return nil, nil, fmt.Errorf("未启用任何数据源:请检查 sources 配置")
	}
	return list, channels, nil
}

// systemPrompt 构建本轮运行的系统提示词:角色 + 主题指令 + 渠道说明 + 输出格式。
func systemPrompt(topic *repository.Topic, channels []Channel, now time.Time) string {
	var b strings.Builder
	b.WriteString("你是一名个人信息情报员,任务是为用户整理一份资讯报告。\n\n")

	b.WriteString(fmt.Sprintf("## 主题:%s(以下指令由用户定义,严格遵守其中的取舍标准;与主题无关的内容不要收录)\n", topic.Name))
	b.WriteString(topic.Prompt)
	b.WriteString("\n\n")

	b.WriteString("## 可用采集渠道\n")
	for _, c := range channels {
		b.WriteString(fmt.Sprintf("- %s:%s\n", c.Name, c.Hint))
	}

	b.WriteString(fmt.Sprintf(`
## 工作方式
1. 综合使用多个渠道收集素材;对高价值条目可用 fetch_webpage 深入阅读后再总结。
2. 去重合并同一事件;渠道返回空属正常(时间窗/去重过滤),不要反复重试。
3. 今天是 %s。

## 输出格式(严格遵守)
第一行输出报告标题(不含"#"等标记),从第二行起输出 Markdown 正文:
- 标题:概括本期最有价值的看点,不超过 40 字。
- 正文:按主题指令要求的结构分节(## 二级标题),每条要点附一句话摘要与原文链接;
  重要条目可展开 2-3 句;结尾给"本期小结"一节(3 句以内)。
- 篇幅控制在 40 行以内,宁缺毋滥;没有合适内容时如实说明,不要硬凑。
`, now.Format("2006-01-02 (MST)")))
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
		{Role: schema.System, Content: systemPrompt(a.topic, a.channels, now)},
		{Role: schema.User, Content: fmt.Sprintf("请执行本次资讯收集任务,主题:%s。现在是 %s。收集、筛选并总结后,按约定格式输出报告。", a.topic.Name, now.Format("2006-01-02 15:04 MST"))},
	})
	if err != nil {
		return nil, fmt.Errorf("agent 执行失败: %w", err)
	}

	subject, body := splitSubjectBody(out.Content)
	return &Digest{Subject: subject, Body: body}, nil
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

// orDefault 返回 s 或默认值。
func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
