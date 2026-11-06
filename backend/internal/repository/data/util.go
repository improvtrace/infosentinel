package data

import "strings"

// joinNames 以逗号连接名称列表(用于引用冲突的错误提示)。
func joinNames(names []string) string { return strings.Join(names, ", ") }
