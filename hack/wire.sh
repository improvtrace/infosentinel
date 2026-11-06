#!/usr/bin/env bash
# 生成 wire 依赖注入代码(cmd/infosentinel/wire.go → wire_gen.go)
#
# wire CLI 经 hack/tools/tools.go 以模块依赖引入(而非 @latest 直装):
# wire v0.6.0 锁定的 golang.org/x/tools v0.17.0 与 Go 1.26 不兼容,
# 本模块已将 x/tools 升级,go run 直接从当前模块解析即可。
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> wire gen"
go run github.com/google/wire/cmd/wire gen ./backend/cmd/infosentinel
echo "==> done: backend/cmd/infosentinel/wire_gen.go"
