#!/usr/bin/env bash
# 一键打包:构建前端 + 后端 → 组装安装包(artifacts/dist/infosentinel-<version>-<os>-<arch>.tar.gz)
#
# 安装包内容:
#   bin/infosentinel            二进制
#   config/config.example.yaml  配置模板(引导段;运行时路径由 config.yaml 决定)
#   scripts/install.sh          一键安装(默认 /usr/local/infosentinel,可用 PREFIX 覆盖)
#   scripts/uninstall.sh        一键卸载
#   scripts/infosentinel.service systemd 单元模板
#   README.md
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> 前端构建"
( cd frontend && npm ci --no-audit --no-fund && npm run build )

echo "==> 后端构建"
VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/aarch64/arm64/; s/x86_64/amd64/')
NAME="infosentinel-${VERSION}-${OS}-${ARCH}"
STAGE="artifacts/dist/${NAME}"

rm -rf "$STAGE" "${STAGE}.tar.gz"
mkdir -p "$STAGE/bin" "$STAGE/config" "$STAGE/scripts"

echo "==> 编译(版本=${VERSION})"
LDFLAGS="-s -w -X main.version=${VERSION}"
GOOS=${GOOS:-$OS} GOARCH=${GOARCH:-$ARCH} go build -ldflags "${LDFLAGS}" \
  -o "$STAGE/bin/infosentinel" ./backend/cmd/infosentinel

cp config.example.yaml "$STAGE/config/config.example.yaml"
cp hack/deploy/install.sh "$STAGE/scripts/install.sh"
cp hack/deploy/uninstall.sh "$STAGE/scripts/uninstall.sh"
cp hack/deploy/infosentinel.service "$STAGE/scripts/infosentinel.service"
cp README.md "$STAGE/"

chmod +x "$STAGE/scripts/"*.sh "$STAGE/bin/infosentinel"

echo "==> 归档"
mkdir -p artifacts/dist
tar -czf "artifacts/dist/${NAME}.tar.gz" -C artifacts/dist "${NAME}"
rm -rf "$STAGE"

echo "==> 打包完成: artifacts/dist/${NAME}.tar.gz"
