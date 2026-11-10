// provider.go 提供 wire 依赖注入的引导配置 Provider。
package config

import "github.com/google/wire"

// ProviderSet 是引导配置与敏感数据解析的 wire ProviderSet。
var ProviderSet = wire.NewSet(NewBootstrap, NewResolver)

// NewBootstrap 从引导配置文件路径加载 Bootstrap(供 wire 注入)。
func NewBootstrap(path string) (*Bootstrap, error) {
	return LoadBootstrap(path)
}
