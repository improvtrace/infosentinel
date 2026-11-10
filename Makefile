# InfoSentinel Makefile — 构建入口(实际逻辑在 hack/ 脚本)
#
# 产物位置约定:
#   artifacts/           编译产物(artifacts/bin/infosentinel、artifacts/dist/*.tar.gz)
#   frontend/dist/       前端构建产物(go:embed 输入,随仓库提交)
#   .tests/              运行时依赖与输出(数据库、日志、去重状态、产物空间、覆盖率);make clean 不清理它
#
# 部署:路径由 config.yaml 决定(见 hack/deploy/install.sh);
# systemd 部署默认位于 $PREFIX/{bin,config,.tests},详见 hack/deploy/

BINARY   := infosentinel
BIN_DIR  := artifacts/bin
PKG      := ./backend/cmd/infosentinel
COVERAGE := .tests/out/coverage.out

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
export VERSION

.DEFAULT_GOAL := help
.PHONY: frontend wire build run serve migrate migrate-diff test cover check vet fmt tidy gen package clean help

frontend: ## 构建前端 SPA(npm ci + vite build → frontend/dist)
	@cd frontend && npm ci --no-audit --no-fund && npm run build

wire: ## 生成 wire 依赖注入代码(wire.go → wire_gen.go;依赖变更后执行)
	@bash hack/wire.sh

gen: ## 生成 ent ORM 代码(hack/gen.sh;含 sql/versioned-migration 特性)
	@bash hack/gen.sh

migrate-diff: ## 生成版本化迁移文件:schema 变更后执行,NAME=迁移描述(默认 changes)
	@bash hack/migrate-diff.sh "$(NAME)"

build: ## 编译到 artifacts/bin/infosentinel(需 frontend/dist 已存在;版本号注入)
	@if [ ! -f frontend/dist/index.html ]; then \
		echo "frontend/dist 缺失:先执行 make frontend(需要 Node.js)"; exit 1; \
	fi
	@bash hack/build.sh

run: build ## 立即执行一轮(自动先编译;传参见 ARGS='-c other.yaml')
	$(BIN_DIR)/$(BINARY) $(ARGS) run

serve: build ## 常驻调度运行(自动先编译;传参见 ARGS='-c other.yaml')
	$(BIN_DIR)/$(BINARY) $(ARGS) serve

migrate: build ## 初始化/升级数据库;空库写入内置默认配置后退出
	$(BIN_DIR)/$(BINARY) $(ARGS) migrate

package: ## 一键打包:前端+后端 → artifacts/dist/infosentinel-<ver>-<os>-<arch>.tar.gz(含安装/卸载脚本)
	@bash hack/package.sh

test: ## 运行全部单元测试(含竞态检测)
	@bash hack/test.sh

cover: ## 单元测试并统计覆盖率(报告写入 .tests/out/coverage.out)
	@mkdir -p $(dir $(COVERAGE))
	go test -coverprofile=$(COVERAGE) ./...
	go tool cover -func=$(COVERAGE) | tail -n 1

check: vet test build ## 提交前完整校验(vet + test + build)

vet: ## go vet 静态检查
	go vet ./...

fmt: ## gofmt 格式化并简化
	gofmt -s -w .

tidy: ## 整理 go.mod / go.sum 依赖
	go mod tidy

clean: ## 清理编译产物与安装包 artifacts/(保留 .tests/ 运行数据)
	rm -rf artifacts

help: ## 显示本帮助
	@echo "InfoSentinel 构建命令(版本:$(VERSION))"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-9s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
