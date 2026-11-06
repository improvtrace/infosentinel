// provider.go 提供 wire 依赖注入的日志 Provider。
package logger

import (
	klog "github.com/go-kratos/kratos/v2/log"
	"github.com/google/wire"

	"github.com/tomasgdl/infosentinel/backend/internal/config"
)

// ProviderSet 是日志组件的 wire ProviderSet。
var ProviderSet = wire.NewSet(NewLogger, NewOptions)

// NewOptions 从引导配置构造日志初始化参数。
func NewOptions(boot *config.Bootstrap) Options {
	return Options{Path: boot.Log.Path, Level: boot.Log.Level}
}

// NewLogger 创建日志组件并返回 wire 形式的清理函数。
func NewLogger(opts Options) (*klog.Helper, func(), error) {
	lg, closer, err := New(opts)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {}
	if closer != nil {
		cleanup = func() { _ = closer.Close() }
	}
	return lg, cleanup, nil
}
