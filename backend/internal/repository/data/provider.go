// provider.go 提供 wire 依赖注入的数据层 Provider。
package data

import (
	"github.com/google/wire"

	"github.com/tomasgdl/infosentinel/backend/internal/config"
	"github.com/tomasgdl/infosentinel/backend/internal/repository"
)

// ProviderSet 是数据层的 wire ProviderSet:*Data 同时满足
// repository.Repository 及其全部子接口。
var ProviderSet = wire.NewSet(
	NewData,
	NewOptions,
	wire.Bind(new(repository.Repository), new(*Data)),
	wire.Bind(new(repository.SecretRepository), new(*Data)),
	wire.Bind(new(repository.ConfigRepository), new(*Data)),
)

// NewOptions 从引导配置构造数据层初始化参数。
func NewOptions(boot *config.Bootstrap) Options {
	return Options{DBPath: boot.Database.Path}
}

// NewData 打开数据库并返回 wire 形式的清理函数。
func NewData(opts Options) (*Data, func(), error) {
	d, err := New(opts)
	if err != nil {
		return nil, nil, err
	}
	return d, func() { _ = d.Close() }, nil
}
