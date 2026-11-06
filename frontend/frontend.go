// Package frontend 内嵌 Web 管理界面的构建产物(Vite + React + TS 构建的 SPA)。
//
// go:embed 不能引用包目录之外的文件,因此由 frontend 目录自身提供嵌入出口,
// backend 的界面层(server)只读引用本包导出的 DistFS。
// dist/ 由 `make frontend`(vite build)生成;仓库中保留一份构建产物,
// 保证无 Node 环境时 go build 依然可用。
package frontend

import "embed"

// DistFS 是界面构建产物(frontend/dist)。
//
//go:embed all:dist
var DistFS embed.FS
