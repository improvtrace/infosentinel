#!/usr/bin/env bash
# 生成 ent 版本化迁移文件:schema 变更后,追加 <UTC时间戳>_<NAME>.sql 到迁移目录
# 并更新 atlas.sum。实现位于 backend/cmd/migrate-diff(内存库重放,不触碰真实数据库)。
#
# 用法:hack/migrate-diff.sh add_tags   (经 make migrate-diff NAME=add_tags 调用)
set -euo pipefail
cd "$(dirname "$0")/.."

name="${1:-changes}"
echo "==> ent migrate diff (name=$name)"
go run ./backend/cmd/migrate-diff -name "$name"
