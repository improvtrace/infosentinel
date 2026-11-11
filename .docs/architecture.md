# InfoSentinel 架构设计

基于 CloudWeGo **Eino**(Agent)、**kratos**(HTTP / 应用生命周期 / 日志)、**google/wire**(依赖注入)、**ent + SQLite**(ORM / 迁移)的兴趣资讯收集服务。

## 分层与依赖方向

```
frontend/                  React SPA(Vite 构建,产物 go:embed 进二进制)
   │  fetch /api/*
   ▼
backend/internal/server    kratos transport/http:路由 DSL + 访问日志 Filter + SPA 托管
   │  依赖接口,不依赖实现
   ▼
backend/internal/repository   业务配置模型(model.go)+ 每表操作接口(repository.go 聚合)
   ▲
   │  ent ORM 实现(每表一文件)
backend/internal/repository/data      ent client / 聚合读取(ent 生成代码位于 data/ent)
backend/internal/repository/schema    ent schema:表结构唯一事实源(手写)
backend/internal/repository/migrate   版本化迁移执行器 + migrations 目录(内嵌,init.sql 含内置默认配置)
   ▼
SQLite(.tests/data/infosentinel.db)

业务编排:cmd → runner → agent(Eino)/ tools / pusher / scheduler / artifact(产物空间)
横切能力:crypto(加密) · logger(kratos log) · storage(产物存储驱动) · render(HTML 渲染)
```

规则:handler 只做参数校验与编解码;业务规则在 repository(Config.Finalize / 校验);
SQL 只存在于 `backend/internal/repository/data`(ent query)与
`backend/internal/repository/migrate/migrations`(ent 生成的迁移 SQL)。
业务配置的类型定义与校验集中在 `repository/model.go`;`config` 包只有引导配置
(Bootstrap:数据库/日志路径),不含任何业务配置语义。

## 依赖注入:google/wire(kratos 风格)

静态依赖图(进程级单例)由 wire 编译期生成构造代码:

```
config.yaml ──► *config.Bootstrap ──┬─► logger(*klog.Helper, cleanup)
                                    └─► *data.Data(迁移, cleanup)
                                            │ wire.Bind
                                            ├─► repository.Repository / SecretRepository / ConfigRepository
                                            ├─► *secret.Resolver
                                            └─► newConfig → *repository.Config(初始快照)
                                                    ▼
                                     cmd/infosentinel.newApp ──► *kratos.App(serve)
```

- **ProviderSet 惯例**:各包 `provider.go`(`config` / `logger` / `repository/data` / `secret`),
  cmd 层 `provider.go` 组装应用(`newConfig` / `newApp` / `newCLIApp`)
- **注入器**:`cmd/infosentinel/wire.go`(`//go:build wireinject`)声明两个注入器,
  `make wire` 生成 `wire_gen.go`:
  - `initApp(cfgPath)` → serve 常驻应用(kratos app)
  - `initCLI(cfgPath)` → run / list / migrate 所需依赖(cliApp:Boot/Data/Log/Secrets)
- **动态依赖不进静态图**:runner / scheduler / pusher / agent 随数据库配置重建
  (Web 界面保存 → `Apply()` 热生效),由 `newApp` 内的闭包按需构建
- **wire CLI 兼容**:wire v0.6.0 锁定的 x/tools 与 Go 1.26 不兼容,
  经 `hack/tools/tools.go` 以模块依赖引入并升级 x/tools(`go run` 从当前模块解析)

## 配置存储:SQLite 唯一事实源

```
config.yaml(引导)                      Web 界面(frontend)
  database.path ──► SQLite 文件            │ REST 写操作
  log.path/level                           ▼
        │                        server → repository(校验 → 落库)
        │ 首次初始化(migrate 或首次启动)  │
        └──► 内置默认配置(init 迁移 SQL)  └──► Apply():重载配置并重建调度(热生效)
```

- **业务配置表**:`topics` / `targets` / `subscriptions` / `settings`(llm/mailer/server/storage 四段 JSON);
  订阅以名称为唯一标识,与主题多对多、与产物空间一对一:`{name, topics[], synthesis, cron, targets[], space}`;
  产物空间(storage 段)可同时配置多个 `spaces[]`(local/sftp/ftp),订阅按名引用其一(空 = 不写产物空间)
- **内置默认配置**:由 init 迁移 SQL 写入(`migrations/20261111070119_init.sql`
  的种子数据段:内置主题/订阅、mailbox 推送目标、llm/mailer/server/storage 全局设置),
  是内置默认值的唯一来源;去重状态目录等部署路径由引导配置 `config.yaml` 提供,
  站内信报告写入产物空间(见 artifact);`migrate` 子命令同样生效
- **敏感数据表**:`secrets` —— value 为 AES-256-GCM 密文(`v1:` 前缀),明文不落库;
  名称约定 `llm.api_key` / `smtp.password` / `storage.<空间名>.password`(SFTP/FTP 逐空间密码,
  缺失时回退旧版全局 `storage.password`)。
  数据库密文是唯一来源(不读环境变量),主密钥为引导层环境变量 `INFOSENTINEL_ENCRYPT_KEY`;
  旧库中 `mailer.type=dryrun` 在 `Finalize()` 归一化为 `站内信`,旧库单空间扁平 storage JSON
  在 `Finalize()` 包装为 `spaces[]` 列表
- **校验**:每次写操作先在内存组装整份配置并 `Finalize()`(引用存在 / cron 合法 /
  名称唯一 / LLM 必填字段),通过才落库;模型层不填充默认值
- **引用保护**:删除已被订阅引用的主题/目标返回 409 并列出引用它的订阅

## 数据库迁移

ent 标准版本化迁移(Atlas 引擎),schema 与迁移同源:

- **迁移目录**:`backend/internal/repository/migrate/migrations/`——每个版本一个
  `<UTC时间戳>_<描述>.sql` + `atlas.sum` 校验和(go:embed 进二进制)
- **生成**:改 `repository/schema` → `make gen`(ORM 代码 + Diff 函数)→
  `make migrate-diff NAME=xxx`(内存库重放现有目录后与 schema 求差,追加迁移文件)
- **执行**(atlas `migrate.Executor`,实现在 `backend/internal/repository/migrate`):
  服务启动或 `infosentinel migrate` 时,按版本序执行未应用的迁移,版本记录写入
  `atlas_schema_revisions`(Atlas CLI 兼容表);逐语句记录进度,中断可续跑;
  `atlas.sum` 校验和不一致时报错,防止迁移文件被篡改
- **旧库兼容**:早期版本用 ent 自动迁移建表、无版本记录,首次迁移自动以最新版本
  为基线(baseline)采纳,不重复建表、数据保留
- **版本高于目录**:数据库被更高版本程序迁移过后,旧程序拒绝启动

操作入口:

- 服务启动:`data.New()` → 版本化迁移(内置默认配置随 init 迁移写入)
- 独立执行:`infosentinel migrate` / `make migrate`,初始化/升级数据库,
  内置默认配置随迁移写入,可在部署前预执行

新增表/字段:改 ent schema → `make gen`(ORM 代码)→
`make migrate-diff NAME=xxx`(迁移文件)→ 启动时自动执行。
注意:ent 的 diff 只生成建表/改表类变更(`AddTable`/`ModifyTable`),删除实体时需在
迁移文件中手动补 `DROP TABLE`(参考 `20261110064516_refactor_subscriptions.sql`)。
已发布的迁移文件与 `atlas.sum` 不可修改或删除(手工改动后需重建 atlas.sum)。

## kratos 集成点

| 能力 | 用法 |
|---|---|
| HTTP 传输 | `khttp.NewServer(khttp.Address(addr), khttp.Filter(accessLog))`;`srv.Route("/api")` + `GET/POST/PUT/DELETE("/path/{name}", handler)` |
| 静态资源 | `srv.HandlePrefix("/", spaHandler)`(未命中回退 index.html) |
| 日志 | `klog.NewHelper(klog.NewFilter(jsonLogger, klog.FilterLevel(...)))`,JSON 行落文件 + stdout |
| 生命周期 | `kratos.New(kratos.Server(httpSrv), kratos.AfterStop(...))`;`app.Run()` 内置信号处理与优雅退出(wire 注入的 `newApp` 组装) |

## 一轮订阅的执行流程

1. **API/调度触发** → `runner.RecordedJob`(防并发守卫:订阅名 + 全部关联主题;一次执行一条运行记录)
2. **解析密钥** `config.Resolver`:DB 密文(LLM API Key / SMTP 授权码 / 逐空间登录密码)
3. **意图分组** `agent.PlanGroups`:对订阅关联的全部主题做一次无工具的模型调用,
   按采集意图相近程度划分分组(意图相近、来源重叠的主题合为一组;意图独立的各自成组);
   解析失败重试一次,仍失败回退为一主题一组(`runner.singleTopicGroups`),流程不中断;
   单主题订阅直接成组,不调用模型
4. **逐组采集(子任务)**:合并加载组内全部主题的去重状态
   (`.tests/data/state/<topic>.json`,多文件合并)→ `agent.BuildTools` 实例化五个
   常驻内置工具(RSS/搜索/HN/Trending/网页;无全局数据源配置,信息源由主题提示词描述)
   → 每组一个 Eino ReAct 产出一份组报告(首行标题);任一组失败则本轮失败(重跑不会漏内容)
5. **合成** `agent.Synthesize`:多组时按订阅的合成提示词(synthesis)单次模型调用
   把各组报告合并为一份最终报告(单组原样返回;默认合成提示词要求输出 HTML 正文片段)
6. **推送** `pusher`:遍历订阅目标,单目标失败不影响其他目标;站内信模式写入订阅
   关联的产物空间,产物路径记为 DSN(`类型://空间名/分桶/文件名`)
7. **状态**:全部推送成功才把合并后的去重状态写回组内全部主题的状态文件;
   运行记录收尾写入标题/正文/产物 DSN 列表
8. **埋点**:`sub_run_done(topic,duration,status,targets)`、`group_run_done(group,...)`、
   `api_request(method,path,code,duration)`

## Agent 内部执行流程(单个主题组的一轮采集)

每个分组独立一个 Agent 实例(Eino `react.Agent` + 五个内置工具),一轮采集分三步:

```
① 提示词组装(agent.systemPrompt,每轮按当前时间重建)
   角色与目标(个人信息情报员;采集 → 筛选 → 总结)
     → 组意图(多主题组:agent.Group.Goal 注入;单主题组无此段)
     → 主题指令(topic.prompt 原文注入,取舍的最高依据;多主题组逐主题分列,要求同时覆盖)
     → 采集渠道(五工具清单 + 参数说明 + 调用建议,agent.channelGuide)
     → 工作方法(先广度后深度 · 步数预算 · 交叉去重 · 筛选标准 · 空结果处理 · 链接真实性)
     → 输出格式(首行纯文本标题 + Markdown 正文,≤40 行)
② ReAct 循环(react.Agent.Generate,步数上限 llm.max_step,默认 25)
   模型决策 ──► 工具调用(fetch_rss / search_web / fetch_hackernews /
                fetch_github_trending / fetch_webpage)
     ▲                    │ 结果回填对话(SeenStore 已按 URL 过滤本轮与跨轮重复)
     └────────────────────┘  直至模型给出最终报告或达到 MaxStep
③ 产出解析(splitSubjectBody):首行 = 报告标题,其余 = 正文 → Digest{Subject, Body}
```

- **去重发生在工具层**:`pkg/tools.SeenStore` 按主题持久化已推送 URL
  (`.tests/data/state/<topic>.json`,保留 7 天),工具返回前过滤,模型无感知;
  组内多主题合并加载为一份状态(`SeenStoreMulti`),全部推送成功后写回组内全部
  主题文件;同一主题跨订阅共享状态,故跨订阅互斥执行
- **正文格式**:单组报告为 Markdown;多组合成报告由订阅的合成提示词决定格式,
  默认提示词要求输出 HTML 正文片段(只允许内容级标签,禁文档级标签与 style/script)
- **推送渲染**(`pkg/render.EmailHTML`)按正文内容自动适配:以 `<` 开头视为 HTML
  片段直通内嵌(剥离 script/iframe/事件属性等做轻量防护),否则按 Markdown 转换
  (goldmark 非 unsafe 模式,原始 HTML 被转义防注入);外层文档结构与内嵌样式统一提供

## 关键设计决策

- **配置与代码解耦**:业务配置的类型定义在 `repository/model.go`,内置默认值在
  init 迁移 SQL 的种子数据段,运行值在数据库;`config` 包只剩引导项
- **依赖注入编译期生成**:wire 保证构造顺序与清理函数配对,无运行时反射
- **敏感数据静态加密**:Web 写入口令即加密;数据库密文是唯一来源,便于页面化管理与迁移
- **repository 接口隔离**:server 依赖 `repository.Repository`,测试可注入假实现;
  数据层未来可换 PostgreSQL 而不影响上层(ent dialect 切换 + 迁移)
- **单二进制交付**:前端构建产物内嵌,安装包只有二进制 + 引导配置 + 脚本
- **订阅级隔离**:工具实例按分组构建(SeenStore 以主题命名、组内多文件合并加载,
  同一主题跨订阅共享去重状态);防并发守卫按「订阅名 + 关联主题」占用槽位,
  同主题跨订阅互斥;采集信息源没有全局配置,由主题提示词描述,采集通用参数为
  `pkg/tools` 内置常量
- **意图分组先于采集**:多主题订阅先整体做意图分析并分组,每组作为子任务采集,
  组报告再按订阅的合成提示词合并 —— 意图相近的主题一次采集覆盖,避免重复劳动;
  分组是优化而非依赖,分析失败自动回退逐主题采集

## 产物目录约定

- `artifacts/`:编译产物 —— `artifacts/bin/`(本地二进制)、`artifacts/dist/`(安装包)
- `.tests/`:运行时依赖与输出 —— 数据库、日志、去重状态、产物空间(artifacts)、覆盖率;
  路径相对工作目录,部署环境在 config.yaml 覆盖
- `frontend/dist/`:前端构建产物,`go:embed` 输入,随仓库提交

## 目录结构

```
frontend/                   Vite + React + TS + Tailwind(页面/组件/类型化 API 客户端)
backend/cmd/infosentinel/   入口:run / serve / list / migrate(kratos app)
  provider.go               应用组装 Provider(newApp / newConfig / newCLIApp)
  wire.go / wire_gen.go     wire 注入器声明(initApp / initCLI)与生成代码
backend/cmd/migrate-diff/   生成版本化迁移文件(内存库重放求差;-hash 重算 atlas.sum)
backend/internal/
  server/                   kratos HTTP:REST API + SPA + 访问日志
  repository/               业务配置模型(model.go)+ 每表操作接口(repository.go 聚合)
    data/                   ent 实现(每表一文件)+ 测试
      ent/                  ent 生成代码(client / migrate Diff 函数)
    schema/                 ent schema:表结构唯一事实源(手写)
    migrate/                版本化迁移执行器(atlas Executor)+ init.sql 内置默认配置
      migrations/           <时间戳>_<描述>.sql + atlas.sum(内嵌,不可手改)
  config/                   引导配置(Bootstrap:数据库/日志/产物根目录)+ 密钥解析
  agent/ runner/            采集编排(Eino)与订阅执行
  pusher/ scheduler/        推送与调度
  artifact/                 产物空间(分桶 + 保留策略,驱动为 pkg/storage)
backend/pkg/
  tools/ crypto/ logger/    通用能力(采集工具 / 加密 / 日志)
  storage/ render/          产物存储驱动(local/sftp/ftp)· 报告正文 → 独立 HTML 文档渲染
hack/                       gen / wire / migrate-diff / build / test / package / deploy
  tools/tools.go            wire CLI 以模块依赖引入(Go 1.26 兼容)
artifacts/                  编译产物(bin / dist),make clean 清理
.tests/                     运行时依赖与输出(data / logs / artifacts / out),make clean 保留
.docs/                      文档
```
