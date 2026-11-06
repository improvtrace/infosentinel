// provider.go 提供 wire 依赖注入的敏感数据解析 Provider。
package secret

import "github.com/google/wire"

// ProviderSet 是敏感数据解析器的 wire ProviderSet。
var ProviderSet = wire.NewSet(New)
