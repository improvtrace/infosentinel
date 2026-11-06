// builtin.go 定义内置默认业务配置:空库在迁移时写入的种子数据。
//
// 默认值集中在本文件(不落在 config 模型层):首次 `migrate` 或首次启动时
// 由 repository/data 写入数据库,之后全部通过 Web 管理界面维护。
package migrate

import (
	"fmt"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
)

// Builtins 返回内置默认业务配置(已通过完整校验)。
func Builtins() (*repository.Config, error) {
	enabled := true
	cfg := &repository.Config{
		Sources: repository.Sources{
			Collect: repository.CollectConfig{
				WindowHours:       48,
				MaxItemsPerSource: 15,
				HTTPTimeoutSec:    20,
				MaxBodyChars:      4000,
				DedupDays:         7,
				StateDir:          ".tests/data/state",
			},
			RSS: repository.RSSConfig{
				Enabled: true,
				Feeds: []repository.RSSFeed{
					{Name: "CloudWeGo", URL: "https://www.cloudwego.io/zh/index.xml"},
					{Name: "Hacker News 精选", URL: "https://hnrss.org/frontpage?points=100"},
				},
			},
			Search: repository.SearchConfig{
				Enabled:    true,
				Region:     "cn-zh",
				MaxResults: 8,
			},
			HackerNews: repository.HackerNewsConfig{
				Enabled:   true,
				TopN:      15,
				MinPoints: 50,
			},
			GitHubTrending: repository.GitHubTrendingConfig{
				Enabled:  true,
				Language: "go",
				TopN:     10,
			},
		},
		Topics: []repository.Topic{
			{
				Name: "ai-agent",
				Prompt: `关注 LLM 与 AI Agent 应用开发生态,重点:
- 新框架、新模型的发布与重要版本更新
- 值得借鉴的工程实践、评测结论与落地案例
报告用中文撰写,按要点分组,每条附一句话摘要与原文链接。`,
			},
		},
		Targets: []repository.Target{
			{
				Name:          "mailbox",
				Type:          "email",
				To:            "me@example.com",
				SubjectPrefix: "[InfoSentinel]",
			},
		},
		Tasks: []repository.Task{
			{
				Name:      "ai-agent-daily",
				Topic:     "ai-agent",
				Targets:   []string{"mailbox"},
				Schedules: []string{"0 8 * * *"},
			},
		},
		Server: repository.ServerConfig{
			Enabled: &enabled,
			Addr:    "127.0.0.1:8080",
		},
		LLM: repository.LLMConfig{
			BaseURL:     "https://api.deepseek.com",
			Model:       "deepseek-chat",
			APIKeyEnv:   "DEEPSEEK_API_KEY",
			Temperature: 0.3,
			TimeoutSec:  300,
			MaxStep:     25,
		},
		Mailer: repository.MailerConfig{
			Type:      "dryrun",
			OutputDir: ".tests/output",
			Smtp: repository.SmtpConfig{
				Port:        465,
				PasswordEnv: "SMTP_PASSWORD",
			},
		},
	}
	if err := cfg.Finalize(); err != nil {
		return nil, fmt.Errorf("内置默认配置不合法: %w", err)
	}
	return cfg, nil
}
