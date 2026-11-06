// 领域错误构造(集中放置,便于统一文案)。
// 退出信号(SIGINT/SIGTERM)由 kratos app 统一等待与处理。
package main

import "fmt"

func errTaskNotFound(name string) error {
	return fmt.Errorf("任务 %q 不存在", name)
}

func errTaskRunning(name string) error {
	return fmt.Errorf("任务 %q 正在运行中", name)
}

func errTargetNotFound(name string) error {
	return fmt.Errorf("推送目标 %q 不存在", name)
}
