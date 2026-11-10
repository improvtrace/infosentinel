// 领域错误构造(集中放置,便于统一文案)。
// 退出信号(SIGINT/SIGTERM)由 kratos app 统一等待与处理。
package main

import "fmt"

func errSubscriptionNotFound(topic string) error {
	return fmt.Errorf("订阅 %q 不存在", topic)
}

func errSubscriptionRunning(topic string) error {
	return fmt.Errorf("订阅 %q 正在运行中", topic)
}

func errTargetNotFound(name string) error {
	return fmt.Errorf("推送目标 %q 不存在", name)
}
