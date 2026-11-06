//go:build tools

// tools.go 以依赖方式引入 wire 代码生成工具:
// 配合本模块 go.mod 中升级的 golang.org/x/tools,wire CLI 可在 Go 1.26 上运行
// (wire v0.6.0 自带的 x/tools v0.17.0 与 Go 1.26 不兼容)。
// 用法:make wire(内部 bash hack/wire.sh → go run github.com/google/wire/cmd/wire)
package tools

import (
	_ "github.com/google/wire/cmd/wire"
)
