// Package render 将 Markdown 报告渲染为独立 HTML 文档,供 HTML 邮件正文与
// 产物空间落盘共用:LLM 输出的 Markdown 转换为带内嵌样式的完整 HTML,
// 不再以 Markdown 纯文本形式发送/保存。
package render

import (
	"bytes"
	"fmt"
	"html"
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

// EmailHTML 将 Markdown 报告渲染为独立 HTML 文档。
// subject 作为页面/邮件标题;转换失败时回退为纯文本段落,保证始终有可用产物。
func EmailHTML(subject, markdown string, generatedAt time.Time) string {
	var buf bytes.Buffer
	if err := md.Convert([]byte(markdown), &buf); err != nil {
		buf.Reset()
		buf.WriteString("<p>")
		buf.WriteString(html.EscapeString(markdown))
		buf.WriteString("</p>")
	}
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
		generatedAt.Format("2006-01-02 15:04"), buf.String())
}
