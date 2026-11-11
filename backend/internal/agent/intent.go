// intent.go 实现订阅级意图分组:一轮执行前对订阅关联的全部主题做一次意图分析
// (单次无工具的模型调用),按采集意图相近程度把主题划分为若干分组,每组作为
// 一个子任务独立采集。单主题订阅直接成组,不调用模型。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
)

// Group 是意图分组后的一个子任务:一组共同采集的主题。
// 单主题组即原逐主题采集(Goal 为空);多主题组共享一次采集与去重状态,
// 由 Goal 概括本组共同的采集意图。
type Group struct {
	Name   string             // 组名(意图概括,用于日志与错误信息)
	Goal   string             // 本组共同采集意图(单主题组为空)
	Topics []repository.Topic // 组内主题(至少一个,采集须同时覆盖)
}

// PlanGroups 对订阅关联的全部主题做意图分组。单主题直接成组;多主题时做一次
// 无工具的模型调用,输出按采集意图相近程度划分的分组。解析失败会重试一次,
// 仍失败返回错误,由调用方决定回退策略(一主题一组)。
func PlanGroups(ctx context.Context, cfg *repository.Config, apiKey string, topics []repository.Topic) ([]Group, error) {
	if len(topics) == 0 {
		return nil, fmt.Errorf("没有可分组的主题")
	}
	if len(topics) == 1 {
		return []Group{{Name: topics[0].Name, Topics: topics}}, nil
	}
	cm, err := NewChatModel(ctx, cfg, apiKey)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		out, err := cm.Generate(ctx, []*schema.Message{
			{Role: schema.System, Content: intentSystemPrompt(now)},
			{Role: schema.User, Content: renderIntentInput(topics)},
		})
		if err != nil {
			return nil, fmt.Errorf("意图分析调用失败: %w", err)
		}
		groups, err := parseIntents(out.Content, topics)
		if err == nil {
			return groups, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("意图分析结果不可用: %w", lastErr)
}

// intentSystemPrompt 构建意图分析的系统提示词:按采集意图相近程度为主题分组。
func intentSystemPrompt(now time.Time) string {
	return fmt.Sprintf(`你是一名情报采集编排专家。今天是 %s。
给定一次订阅执行所关联的全部主题(每个主题是一段采集指令),请分析各主题的采集意图,
按意图相近程度把它们划分为一到多个分组:
- 采集意图相近、信息来源重叠的主题合为一组,以便单次采集同时覆盖,避免重复劳动;
- 意图独立、互不相关的主题各自单独成组,不要为凑组而强行合并;
- 每个主题必须且只能出现在一个分组中,所有主题都要被分配。
严格只输出 JSON(不输出任何其他文字,不包代码围栏),格式如下,
topics 只能使用输入给出的主题名,不得改写:
{"groups":[{"name":"分组名(意图概括,10字以内)","goal":"本组共同的采集意图,一句话","topics":["主题名",...]}]}`,
		now.Format("2006-01-02"))
}

// renderIntentInput 把主题清单拼接为意图分析的输入材料。
func renderIntentInput(topics []repository.Topic) string {
	var b strings.Builder
	fmt.Fprintf(&b, "请对以下 %d 个主题做意图分组:\n\n", len(topics))
	for _, t := range topics {
		fmt.Fprintf(&b, "=== 主题:%s ===\n%s\n\n", t.Name, t.Prompt)
	}
	return b.String()
}

// intentPlan 是意图分析输出的 JSON 契约。
type intentPlan struct {
	Groups []struct {
		Name   string   `json:"name"`
		Goal   string   `json:"goal"`
		Topics []string `json:"topics"`
	} `json:"groups"`
}

// parseIntents 解析并校验意图分析输出:组名非空唯一、组非空、
// 每个主题恰好被分配一次(不得引用未知主题,也不得遗漏)。
func parseIntents(content string, topics []repository.Topic) ([]Group, error) {
	planJSON := stripJSONFence(content)
	var plan intentPlan
	if err := json.Unmarshal([]byte(planJSON), &plan); err != nil {
		return nil, fmt.Errorf("解析意图分析 JSON: %w", err)
	}
	if len(plan.Groups) == 0 {
		return nil, fmt.Errorf("意图分析未返回任何分组")
	}

	byName := make(map[string]repository.Topic, len(topics))
	for _, t := range topics {
		byName[t.Name] = t
	}
	assigned := make(map[string]string, len(topics)) // 主题名 -> 所属组名
	groups := make([]Group, 0, len(plan.Groups))
	seenGroup := make(map[string]struct{}, len(plan.Groups))
	for _, g := range plan.Groups {
		switch {
		case g.Name == "":
			return nil, fmt.Errorf("存在组名为空的分组")
		case len(g.Topics) == 0:
			return nil, fmt.Errorf("分组 %q 没有任何主题", g.Name)
		}
		if _, dup := seenGroup[g.Name]; dup {
			return nil, fmt.Errorf("组名 %q 重复", g.Name)
		}
		seenGroup[g.Name] = struct{}{}

		members := make([]repository.Topic, 0, len(g.Topics))
		for _, name := range g.Topics {
			t, ok := byName[name]
			if !ok {
				return nil, fmt.Errorf("分组 %q 引用了未知主题 %q", g.Name, name)
			}
			if owner, dup := assigned[name]; dup {
				return nil, fmt.Errorf("主题 %q 同时出现在分组 %q 与 %q", name, owner, g.Name)
			}
			assigned[name] = g.Name
			members = append(members, t)
		}
		groups = append(groups, Group{Name: g.Name, Goal: g.Goal, Topics: members})
	}
	if len(assigned) != len(topics) {
		missing := make([]string, 0, len(topics)-len(assigned))
		for _, t := range topics {
			if _, ok := assigned[t.Name]; !ok {
				missing = append(missing, t.Name)
			}
		}
		return nil, fmt.Errorf("主题未被分配分组: %s", strings.Join(missing, ", "))
	}
	return groups, nil
}

// stripJSONFence 去掉模型输出可能包裹的代码围栏(意图分析要求纯 JSON,
// 部分模型仍会带 ```json 围栏,这里兜底)。
func stripJSONFence(content string) string {
	s := strings.TrimSpace(content)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
