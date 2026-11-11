// 领域错误构造(集中放置,便于统一文案)。
// 退出信号(SIGINT/SIGTERM)由 kratos app 统一等待与处理。
package main

import (
	"fmt"

	"github.com/tomasgdl/infosentinel/backend/internal/artifact"
)

func errSubscriptionNotFound(name string) error {
	return fmt.Errorf("订阅 %q 不存在", name)
}

func errSubscriptionRunning(name string) error {
	return fmt.Errorf("订阅 %q 正在运行中", name)
}

func errTargetNotFound(name string) error {
	return fmt.Errorf("推送目标 %q 不存在", name)
}

// testSpaceName 返回测试投递使用的产物空间名(站内信模式):
// 取第一个已配置空间;未配置任何空间时返回空串(由 pusher 报出明确错误)。
func testSpaceName(spaces *artifact.Spaces) string {
	names := spaces.Names()
	if len(names) == 0 {
		return ""
	}
	return names[0]
}
