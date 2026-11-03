# InfoSentinel Makefile — 标准化构建流程与产物位置
#
# 产物位置约定:
#   bin/  编译产物(bin/infosentinel)
#   out/  运行产物(邮件、去重状态)与覆盖率报告;make clean 不清理它

BINARY   := infosentinel
BIN_DIR  := bin
PKG      := ./cmd/infosentinel
COVERAGE := out/coverage.out

# 版本号:默认取 git 描述(无 tag 时为短哈希),可外部覆盖:make build VERSION=v1.0.0
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)

.DEFAULT_GOAL := help
.PHONY: build run serve test cover check vet fmt tidy clean help

build: ## 编译到 bin/infosentinel(注入版本号并 strip 调试信息)
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(PKG)

run: build ## 立即执行一轮(自动先编译;传参见 ARGS='-c other.yaml')
	$(BIN_DIR)/$(BINARY) $(ARGS) run

serve: build ## 常驻调度运行(自动先编译;传参见 ARGS='-c other.yaml')
	$(BIN_DIR)/$(BINARY) $(ARGS) serve

test: ## 运行全部单元测试(含竞态检测)
	go test -race ./...

cover: ## 单元测试并统计覆盖率(报告写入 out/coverage.out)
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

clean: ## 清理编译产物与覆盖率(保留 out/ 下的运行数据)
	rm -rf $(BIN_DIR)
	rm -f $(COVERAGE)

help: ## 显示本帮助
	@echo "InfoSentinel 构建命令(产物:$(BIN_DIR)/$(BINARY),版本:$(VERSION))"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-8s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
