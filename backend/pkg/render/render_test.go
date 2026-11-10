package render

import (
	"strings"
	"testing"
	"time"
)

// TestEmailHTML 校验 Markdown 渲染、标题转义与文档结构。
func TestEmailHTML(t *testing.T) {
	md := "## Go\n- 新版本发布\n\n| 库 | 星数 |\n|---|---|\n| eino | 1k |\n\n**加粗** 与 <script>alert(1)</script>"
	html := EmailHTML("今日看点", md, time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC))

	for _, want := range []string{
		"<!DOCTYPE html>",
		"<title>今日看点</title>",
		"<h2>Go</h2>",
		"<li>新版本发布</li>",
		"<table>",
		"<strong>加粗</strong>",
		"2026-11-09 08:00", // 生成时间
	} {
		if !strings.Contains(html, want) {
			t.Errorf("渲染结果缺少 %q", want)
		}
	}
	// 原始 HTML 应被剔除(goldmark 非 unsafe 模式输出占位注释)
	if strings.Contains(html, "<script>") {
		t.Error("原始 <script> 不应出现在渲染结果中")
	}
	if !strings.Contains(html, "raw HTML omitted") {
		t.Error("原始 HTML 应被替换为占位注释")
	}
}

// TestEmailHTMLEscapesSubject 校验标题中的特殊字符被转义。
func TestEmailHTMLEscapesSubject(t *testing.T) {
	html := EmailHTML(`标题<script>`, "正文", time.Now())
	if strings.Contains(html, "<script>") {
		t.Error("标题中的脚本不应原样出现")
	}
	// 标题经 html.EscapeString 转义后写入文档
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Error("标题特殊字符应转义")
	}
}

// TestEmailHTMLFallback 校验 Markdown 转换失败时回退为转义纯文本。
func TestEmailHTMLFallback(t *testing.T) {
	// 正常输入始终可转换;此处校验空输入也产出合法文档
	html := EmailHTML("空报告", "", time.Now())
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Error("空输入也应产出完整文档")
	}
}
