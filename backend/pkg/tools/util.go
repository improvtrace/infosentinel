package tools

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"
)

// friendlyResult 把工具输出序列化为紧凑 JSON,供 LLM 阅读。
// 工具的返回值始终走此函数,保证错误也能以结构化形式回传而不是中断 agent 循环。
func friendlyResult(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		// 理论上不可达:输入都是简单结构
		return `{"error":"failed to encode tool result"}`
	}
	return string(raw)
}

var (
	// blockTagRe 匹配块级边界(闭合块级标签与 <br>),替换为空格以保留词间距
	blockTagRe = regexp.MustCompile(`(?i)</(?:p|div|li|h[1-6]|tr|table|blockquote|article|section)>|<br[^>]*/?>`)
	// htmlTagRe 匹配其余任意标签,直接删除(行内标签两侧不应引入空格)
	htmlTagRe    = regexp.MustCompile(`(?s)<[^>]*>`)
	whitespaceRe = regexp.MustCompile(`[ \t\r\n]+`)
)

// stripHTML 去除 HTML 标签并压缩空白,得到适合 LLM 阅读的纯文本。
// 块级标签转为空格,行内标签直接删除,避免中文之间出现多余空格。
func stripHTML(s string) string {
	s = blockTagRe.ReplaceAllString(s, " ")
	s = htmlTagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(whitespaceRe.ReplaceAllString(s, " "))
}

// truncate 按字符数截断(优先在空白处断开),超长时追加省略号。
func truncate(s string, max int) string {
	if max <= 0 || len([]rune(s)) <= max {
		return s
	}
	r := []rune(s)
	cut := string(r[:max])
	if i := strings.LastIndexAny(cut, " \t\n"); i > max/2 {
		cut = cut[:i]
	}
	return cut + "…"
}
