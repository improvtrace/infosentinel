#!/usr/bin/env bash
# 编译后端二进制(前端产物需先就绪:make frontend)
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
LDFLAGS="-s -w -X main.version=${VERSION}"

echo "==> go build (version=${VERSION})"
mkdir -p artifacts/bin
go build -ldflags "${LDFLAGS}" -o artifacts/bin/infosentinel ./backend/cmd/infosentinel
echo "==> done: artifacts/bin/infosentinel"
