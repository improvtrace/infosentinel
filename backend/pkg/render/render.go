// Package render 将报告正文渲染为独立 HTML 文档,供 HTML 邮件正文与
// 产物空间落盘共用:Markdown 正文经 goldmark 转换(默认转义原始 HTML 防注入),
// HTML 片段正文(合成提示词默认输出)直通内嵌并做轻量防护。
package render

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// md 是 Markdown 转换器:GFM 扩展(表格/删除线/自动链接/任务列表)。
// 默认转义原始 HTML(不启用 WithUnsafe),报告内容不可注入脚本。
var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
)

// css 是内嵌样式:面向浏览器与主流邮件客户端的简洁排版。
const css = `
body{margin:0;padding:24px 12px;background:#f1f5f9;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Hiragino Sans GB","Microsoft YaHei",sans-serif;color:#1e293b;}
.page{max-width:720px;margin:0 auto;background:#ffffff;border-radius:12px;overflow:hidden;border:1px solid #e2e8f0;}
.head{padding:20px 28px;border-bottom:1px solid #e2e8f0;background:#f8fafc;}
.head .subject{font-size:18px;font-weight:600;line-height:1.4;}
.head .meta{margin-top:6px;font-size:12px;color:#64748b;}
.body{padding:8px 28px 28px;font-size:14px;line-height:1.75;}
.body h1{font-size:20px;margin:24px 0 12px;}
.body h2{font-size:17px;margin:26px 0 10px;padding-bottom:6px;border-bottom:1px solid #e2e8f0;}
.body h3{font-size:15px;margin:20px 0 8px;}
.body p{margin:10px 0;}
.body a{color:#2563eb;text-decoration:none;word-break:break-all;}
.body ul,.body ol{padding-left:22px;margin:10px 0;}
.body li{margin:4px 0;}
.body blockquote{margin:12px 0;padding:8px 14px;border-left:3px solid #cbd5e1;background:#f8fafc;color:#475569;}
.body code{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:13px;background:#f1f5f9;border-radius:4px;padding:1px 5px;}
.body pre{background:#0f172a;color:#e2e8f0;border-radius:8px;padding:14px;overflow:auto;}
.body pre code{background:none;color:inherit;padding:0;}
.body table{border-collapse:collapse;width:100%;margin:14px 0;font-size:13px;}
.body th,.body td{border:1px solid #e2e8f0;padding:6px 10px;text-align:left;}
.body th{background:#f8fafc;font-weight:600;}
.body hr{border:none;border-top:1px solid #e2e8f0;margin:20px 0;}
.foot{padding:14px 28px;border-top:1px solid #e2e8f0;font-size:11.5px;color:#94a3b8;background:#f8fafc;}
`

// EmailHTML 将报告正文渲染为独立 HTML 文档。
// subject 作为页面/邮件标题;正文按内容自动识别:HTML 片段直通(轻量防护),
// Markdown 走 goldmark 转换;转换失败时回退为纯文本段落,保证始终有可用产物。
func EmailHTML(subject, body string, generatedAt time.Time) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s</title>
<style>%s</style>
</head>
<body>
<div class="page">
  <div class="head">
    <div class="subject">%s</div>
    <div class="meta">由 InfoSentinel 生成 · %s</div>
  </div>
  <div class="body">
%s
  </div>
  <div class="foot">InfoSentinel · 兴趣资讯收集 Agent</div>
</div>
</body>
</html>
`, html.EscapeString(subject), css, html.EscapeString(subject),
		generatedAt.Format("2006-01-02 15:04"), convertBody(body))
}

// convertBody 把报告正文转换为 HTML 片段:剥掉可能包裹全文的代码围栏后,
// 以 "<" 开头视为 HTML 片段直通(轻量防护),其余按 Markdown 转换。
func convertBody(body string) string {
	body = stripCodeFence(body)
	if strings.HasPrefix(body, "<") {
		return guardHTML(body)
	}
	var buf bytes.Buffer
	if err := md.Convert([]byte(body), &buf); err != nil {
		buf.Reset()
		buf.WriteString("<p>")
		buf.WriteString(html.EscapeString(body))
		buf.WriteString("</p>")
	}
	return buf.String()
}

// stripCodeFence 剥离 LLM 可能把整个正文包进的 Markdown 代码围栏(```html ... ```)。
// 仅当首行以 ``` 开头且末行以 ``` 结尾(整段一个围栏)时剥离,避免误伤正文内的合法代码块。
func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	lines := strings.Split(s, "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "```") {
		return s
	}
	if !strings.HasSuffix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		return s
	}
	return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
}

// 防护规则(轻量兜底,最佳努力):界面用沙箱 iframe(禁脚本)渲染,但产物 HTML
// 文件可能被独立打开,故剥离脚本类标签、事件属性与 javascript: 链接。
// 完整约束由合成提示词给出;如需严格白名单净化应引入专用 sanitizer。
var (
	scriptBlockRe = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>|<script\b[^>]*/?>`)
	dangerTagRe   = regexp.MustCompile(`(?is)<\s*/?\s*(iframe|object|embed|style)\b[^>]*>`)
	eventAttrRe   = regexp.MustCompile(`(?i)\son[a-z]+\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]+)`)
	jsHrefRe      = regexp.MustCompile(`(?i)href\s*=\s*(?:"\s*javascript:[^"]*"|'\s*javascript:[^']*'|\s*javascript:[^\s>]+)`)
)

// guardHTML 对直通的 HTML 片段做轻量防护:剥离 script(含内容)/iframe/object/embed/style
// 标签、内联事件属性与 javascript: 链接,提示词约束之外多一道兜底。
func guardHTML(s string) string {
	s = scriptBlockRe.ReplaceAllString(s, "")
	s = dangerTagRe.ReplaceAllString(s, "")
	s = eventAttrRe.ReplaceAllString(s, "")
	s = jsHrefRe.ReplaceAllString(s, `href="#"`)
	return s
}
