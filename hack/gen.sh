#!/usr/bin/env bash
# 生成 ent ORM 代码:schema(backend/internal/repository/schema)→ backend/internal/repository/data/ent/
# 特性:sql/upsert(按唯一键 upsert)、sql/versioned-migration(在生成代码中提供 Diff 函数)
#
# 迁移文件不在本脚本生成:schema 变更后另执行 `make migrate-diff NAME=xxx`。
set -euo pipefail
cd "$(dirname "$0")/../backend/internal/repository/data"

echo "==> ent generate (schema → ent, features: sql/upsert + sql/versioned-migration)"
go run -mod=mod entgo.io/ent/cmd/ent generate \
    --feature sql/upsert,sql/versioned-migration \
    --target ./ent ../schema
echo "==> done: backend/internal/repository/data/ent"
