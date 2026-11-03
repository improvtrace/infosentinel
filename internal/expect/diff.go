package expect

import (
	"slices"
	"strings"
)

// Diff 返回 a→b 的逐行差异(基于最长公共子序列,无外部依赖)。
// 相同行前缀 "  ",删除行 "- ",新增行 "+ ";无差异返回空串。
// 配置文件体量小(百行级),O(n·m) 内存开销可忽略。
func Diff(a, b string) string {
	al := splitLines(a)
	bl := splitLines(b)
	if slices.Equal(al, bl) {
		return ""
	}

	// dp[i][j] = al[i:] 与 bl[j:] 的 LCS 长度
	n, m := len(al), len(bl)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if al[i] == bl[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("--- 当前配置\n+++ 修改后\n")
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case al[i] == bl[j]:
			sb.WriteString("  " + al[i] + "\n")
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			sb.WriteString("- " + al[i] + "\n")
			i++
		default:
			sb.WriteString("+ " + bl[j] + "\n")
			j++
		}
	}
	for ; i < n; i++ {
		sb.WriteString("- " + al[i] + "\n")
	}
	for ; j < m; j++ {
		sb.WriteString("+ " + bl[j] + "\n")
	}
	return sb.String()
}

// splitLines 按行拆分,忽略末尾换行造成的空行。
func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
