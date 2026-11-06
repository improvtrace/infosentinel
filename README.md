# InfoSentinel

基于 [CloudWeGo Eino](https://github.com/cloudwego/eino) 的兴趣资讯收集 Agent:配置公用数据源,为主题(一段 prompt)自动采集内容、总结成结构化报告,按 `{主题, 推送目标, 调度表达式}` 组成任务定时推送到多个邮箱(内置 SMTP,dry-run 可预览)。

**技术栈**:Eino(Agent) · kratos(HTTP/应用生命周期/日志) · google/wire(依赖注入) · ent + SQLite(ORM 与迁移) · React 18 + TypeScript + Tailwind(Vite 构建,内嵌进二进制)。

## 特性

- **Eino ReAct Agent**:模型自主决策调用哪些采集工具、何时深读网页、如何取舍
- **配置以 SQLite 为唯一事实源**:config.yaml 仅保留数据库与日志路径;业务配置全部在 Web 界面维护,保存即校验并热生效;内置默认配置由 `repository/migrate` 在迁移时写入空库
- **wire 依赖注入**:kratos 风格 ProviderSet(各包 `provider.go`),`make wire` 生成注入代码;静态图 = 引导配置 → 日志/数据层 → 仓库接口 → 敏感数据解析 → kratos app
- **ent ORM + 版本化迁移(Atlas)**:schema 即代码,`make migrate-diff` 生成迁移文件(atlas.sum 完整性校验),启动时经 atlas Executor 执行并记录 `atlas_schema_revisions`,幂等、断点续跑;也可用 `migrate` 子命令单独执行(空库同时写入内置默认配置)
- **repository 分层**:`repository/` 每表一个接口文件 + 业务配置模型(`model.go`),`repository/data`(ent)每表一个实现文件;`schema/` 与 `migrate/` 独立于 ent 生成代码
- **敏感数据加密**:模型 API Key / SMTP 授权码 AES-256-GCM 加密入库(环境变量优先)
- **结构化日志**:kratos log 输出 JSON 到文件与 stdout,含 API 访问、任务运行、调度、密钥操作埋点
- **现代管理界面**:React + TypeScript + Tailwind(参考 DeepSeek Harness 的侧边栏与深色开发者审美),含 Toast 反馈、暗色自适应
- **一键打包与安装**:`make package` 产出 tar.gz(含 install/uninstall/systemd),统一安装到 `/usr/local/infosentinel/`

## 快速开始(开发)

```bash
make gen          # 生成 ent 代码(schema → ent,含版本化迁移支持)
make frontend     # 构建前端(需要 Node.js)
make wire         # 生成 wire 依赖注入代码(依赖图变更后执行)
make build        # 编译 artifacts/bin/infosentinel(前端内嵌)
make serve        # 启动服务(空库自动写入内置默认配置)

export DEEPSEEK_API_KEY=sk-...        # 或在界面“全局设置”中加密填写
export INFOSENTINEL_ENCRYPT_KEY=...   # 使用界面加密存储密钥时必需
# → http://127.0.0.1:8080
```

## 产物目录约定

- `artifacts/`:全部编译产物 —— 本地二进制 `artifacts/bin/`、安装包 `artifacts/dist/`(`make clean` 清理)
- `.tests/`:全部运行时依赖与输出 —— 数据库 `.tests/data/`、日志 `.tests/logs/`、dry-run 邮件 `.tests/output/`、去重状态 `.tests/data/state/`、覆盖率 `.tests/out/`(`make clean` 保留)
- `frontend/dist/`:前端构建产物,是 `go:embed` 输入、随仓库提交,不属以上两类

## 生产安装

```bash
make package
# 产物:artifacts/dist/infosentinel-<version>-<os>-<arch>.tar.gz

sudo tar -xzf artifacts/dist/infosentinel-*.tar.gz -C /tmp
cd /tmp/infosentinel-*/
sudo ./scripts/install.sh          # 默认安装到 /usr/local/infosentinel
sudo systemctl start infosentinel
sudo ./scripts/uninstall.sh        # 卸载(保留数据);--purge 连数据一起删
```

安装目录布局(运行时路径由 config.yaml 决定,默认相对工作目录):

```
/usr/local/infosentinel/
├── bin/infosentinel        二进制
├── config/config.yaml      引导配置(数据库路径 / 日志;按需修改)
├── config/env              systemd 环境变量文件(可选:API Key 等)
└── .tests/                 运行时依赖与输出(config.yaml 默认指向)
    ├── data/infosentinel.db  SQLite 配置数据库
    ├── data/state/           任务去重状态
    ├── logs/infosentinel.log JSON 结构化日志
    └── output/               dry-run 邮件产物
```

## 文档

- [架构设计](.docs/architecture.md) — 分层、配置存储、wire/kratos/ent/Eino 集成
- [配置参考](.docs/configuration.md) — 引导配置与 DB 配置字段、密钥管理
- [开发指南](.docs/development.md) — 构建/测试/API 一览/部署与运维

## 项目结构

```
frontend/                 Vite + React + TypeScript + Tailwind 界面工程(dist 内嵌)
backend/
  cmd/infosentinel/       入口(run / serve / list / migrate;kratos app 生命周期)
    provider.go           应用组装 Provider(newApp:newConfig → kratos app)
    wire.go / wire_gen.go wire 注入器声明与生成代码(initApp / initCLI)
  cmd/migrate-diff/       生成版本化迁移文件(内存库重放求差)
  internal/
    server/               kratos HTTP:REST API(/api/*)+ SPA 托管 + 访问日志
    repository/           业务配置模型(model.go)+ 每表操作接口(repository.go 聚合)
      data/               ent 实现:每表一文件 + 空库内置种子(ent 生成代码位于 data/ent)
      schema/             ent schema:表结构唯一事实源(手写)
      migrate/            版本化迁移执行器(atlas Executor)+ 内置默认配置(builtin.go)
        migrations/       迁移目录(<时间戳>_<描述>.sql + atlas.sum,内嵌进二进制)
    config/               引导配置(Bootstrap:数据库/日志路径)与 wire Provider
    agent/                Eino ReAct Agent(主题 prompt + 公用工具)
    tools/                内置采集工具 + 任务级去重状态
    runner/               任务编排(防并发守卫 + 结构化埋点)
    pusher/               推送通道(email:dry-run + 内置 SMTP)
    scheduler/            cron 调度(任务 × 表达式)
    history/              运行历史(内存环形)
    crypto/               AES-256-GCM 敏感数据加密
    logger/               kratos log JSON 文件日志
    secret/               敏感数据解析(环境变量优先)
hack/                     构建与部署脚本(gen/wire/build/test/package/deploy)
  tools/tools.go          wire CLI 以模块依赖引入(Go 1.26 兼容)
```
