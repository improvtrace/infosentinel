//go:build wireinject
// +build wireinject

// wire.go 声明 wire 依赖注入器;实际构造代码由 wire 工具生成到 wire_gen.go。
//
// 重新生成:make wire(内部调用 hack/wire.sh)
//
//	initApp  serve 常驻应用(kratos app)
//	initCLI  run / list / migrate 子命令所需依赖
package main

import (
	"github.com/go-kratos/kratos/v2"
	"github.com/google/wire"

	"github.com/tomasgdl/infosentinel/backend/internal/config"
	"github.com/tomasgdl/infosentinel/backend/internal/logger"
	"github.com/tomasgdl/infosentinel/backend/internal/repository/data"
	"github.com/tomasgdl/infosentinel/backend/internal/secret"
)

// initApp 构造 serve 常驻应用。
func initApp(cfgPath string) (*kratos.App, func(), error) {
	wire.Build(config.ProviderSet, logger.ProviderSet, data.ProviderSet, secret.ProviderSet, appProviderSet)
	return nil, nil, nil
}

// initCLI 构造 CLI 子命令依赖(引导配置 + 数据层 + 日志 + 敏感数据解析)。
func initCLI(cfgPath string) (*cliApp, func(), error) {
	wire.Build(config.ProviderSet, logger.ProviderSet, data.ProviderSet, secret.ProviderSet, cliProviderSet)
	return nil, nil, nil
}
