// synthesize.go 实现多主题报告合成:把一轮订阅执行中各主题分别产出的报告,
// 按订阅的合成提示词(synthesis)合并为一份最终报告(单次无工具的模型调用)。
package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/tomasgdl/infosentinel/backend/internal/repository"
)

// Synthesize 按合成提示词把多份主题报告合并为一份最终报告。
// parts 不足两份时无需合并,直接返回首份(合成是为一对多主题设计的)。
func Synthesize(ctx context.Context, cfg *repository.Config, apiKey, prompt string, parts []*Digest) (*Digest, error) {
	if len(parts) == 0 {
		return nil, fmt.Errorf("没有可合成的主题报告")
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	cm, err := NewChatModel(ctx, cfg, apiKey)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out, err := cm.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: synthesisSystemPrompt(prompt, now)},
		{Role: schema.User, Content: renderParts(parts)},
	})
	if err != nil {
		return nil, fmt.Errorf("合成报告失败: %w", err)
	}
	subject, body := splitSubjectBody(out.Content)
	return &Digest{Subject: subject, Body: body}, nil
}

// synthesisSystemPrompt 构建合成的系统提示词:角色 + 订阅的合成指令 + 输出格式。
// 正文格式以合成指令为准(Markdown 或 HTML 片段);要求 HTML 时给出片段契约:
// 只用内容级标签,禁止文档级标签与 style/script(外层文档结构与样式由 render 包统一负责)。
func synthesisSystemPrompt(prompt string, now time.Time) string {
	return fmt.Sprintf(`你是一名资讯编辑。今天是 %s。

## 合成指令(由用户定义,严格遵守)
%s

## 输出格式(严格遵守)
第一行输出合并后报告的总标题:纯文本一行,不含"#"等标记或任何 HTML 标签。
从第二行起按合成指令规定的格式输出正文,按指令组织结构:
- 指令要求 Markdown 时:输出 Markdown 正文。
- 指令要求 HTML 时:输出 HTML 正文片段,只使用 <section> <div> <h2>-<h4> <p> <ul> <ol> <li>
  <a> <strong> <em> <blockquote> <table> 等内容级标签,链接用 <a> 原样保留;
  禁止 <!DOCTYPE> <html> <head> <body> 等文档级标签,禁止 <style> 与 <script>,
  页面样式由系统统一提供。
- 不要把整个正文包进代码围栏(反引号)。
没有合适内容时如实说明,不要硬凑。`, now.Format("2006-01-02 (MST)"), prompt)
}

// renderParts 把各主题报告拼接为合成的输入材料。
func renderParts(parts []*Digest) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("以下是 %d 份主题报告,请按合成指令合并为一份最终报告:\n\n", len(parts)))
	for i, p := range parts {
		b.WriteString(fmt.Sprintf("--- 第 %d 份报告 ---\n标题:%s\n\n%s\n\n", i+1, p.Subject, p.Body))
	}
	return b.String()
}
