#!/usr/bin/env bash
# 运行全部 Go 测试(含竞态检测)
set -euo pipefail
cd "$(dirname "$0")/.."
go test -race ./...
