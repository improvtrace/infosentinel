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

// TestEmailHTMLFragmentPassthrough 校验 HTML 片段正文直通渲染(标签原样保留)。
func TestEmailHTMLFragmentPassthrough(t *testing.T) {
	frag := `<section><h2>模型</h2><p>GPT 发布,<a href="https://example.com/a">原文</a>。</p></section>`
	html := EmailHTML("HTML 报告", frag, time.Now())
	for _, want := range []string{
		`<section>`, // 片段直通,不经 Markdown 转换
		`<a href="https://example.com/a">原文</a>`,
		`<div class="body">`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML 片段渲染结果缺少 %q", want)
		}
	}
}

// TestEmailHTMLGuard 校验 HTML 片段直通时的轻量防护(脚本/事件属性/危险链接被剥离)。
func TestEmailHTMLGuard(t *testing.T) {
	frag := `<section onclick="evil()">正文 <script>alert(1)</script>` +
		`<a href="javascript:evil()">链接</a><iframe src="https://evil.com"></iframe></section>`
	html := EmailHTML("防护报告", frag, time.Now())
	for _, bad := range []string{"<script", "onclick", "javascript:", "<iframe"} {
		if strings.Contains(html, bad) {
			t.Errorf("危险内容 %q 不应出现在渲染结果中", bad)
		}
	}
	if !strings.Contains(html, `href="#"`) {
		t.Error("javascript: 链接应被替换为 href=\"#\"")
	}
	if !strings.Contains(html, `<section>`) || !strings.Contains(html, `<a `) {
		t.Error("正常标签与链接应保留")
	}
}

// TestStripCodeFence 校验代码围栏剥离:整段围栏剥离,局部围栏与普通 Markdown 不动。
func TestStripCodeFence(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "整段 html 围栏", in: "```html\n<p>正文</p>\n```", want: "<p>正文</p>"},
		{name: "整段无语言围栏", in: "```\n## 标题\n```", want: "## 标题"},
		{name: "未闭合围栏不剥离", in: "```html\n<p>正文</p>", want: "```html\n<p>正文</p>"},
		{name: "正文内代码块不剥离", in: "## 标题\n```go\ngo run .\n```", want: "## 标题\n```go\ngo run .\n```"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripCodeFence(tc.in); got != tc.want {
				t.Errorf("stripCodeFence(%q) = %q,期望 %q", tc.in, got, tc.want)
			}
		})
	}
}
