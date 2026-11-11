# InfoSentinel 开发指南

## 环境要求

- Go ≥ 1.24(开发环境 1.26)
- Node.js ≥ 18 + npm(前端构建)
- 网络:LLM 服务、RSS 源、`hacker-news.firebaseio.com`、`github.com/trending`

## 构建与运行

```bash
make gen        # 生成 ent 代码(schema → ent;改动 schema 后必须执行)
make migrate-diff NAME=add_tags   # schema 变更后生成版本化迁移文件
make frontend   # 前端构建(npm ci + vite build → frontend/dist)
make wire       # 生成 wire 依赖注入代码(依赖图变更后执行)
make build      # 编译 artifacts/bin/infosentinel(前端内嵌,注入 git 版本)
make serve      # 启动(首次启动自动初始化数据库与内置默认配置)
make run ARGS='-topic "情报日报"'  # 单轮执行(按订阅名称,缺省执行全部)
make migrate    # 初始化/升级数据库;内置默认配置随迁移写入后退出
make package    # 一键打包 artifacts/dist/infosentinel-<ver>-<os>-<arch>.tar.gz
make check      # vet + test + build
make clean      # 清理 artifacts/(保留 .tests/ 运行数据)
```

`make help` 查看全部目标。脚本实现位于 `hack/`(gen/wire/migrate-diff/build/test/package)与
`hack/deploy/`(install/uninstall/systemd)。

## 目录与分层约定

| 目录 | 职责 | 约束 |
|---|---|---|
| `frontend/src` | SPA 页面组件 | 只通过 `src/api.ts` 调 `/api/*` |
| `backend/internal/server` | kratos HTTP handler | 不写 SQL、不做存储细节 |
| `backend/internal/repository` | 业务配置模型(model.go)+ 每表操作接口(一表一文件) | 上层只依赖接口与模型类型 |
| `backend/internal/repository/data` | ent 实现(每表一文件) | 唯一允许出现 ent/DB 代码的位置 |
| `backend/internal/repository/data/ent` | ent 生成代码 | 不手改;`make gen` 重新生成 |
| `backend/internal/repository/schema` | ent schema:表结构唯一事实源(手写) | 改完执行 `make gen` + `make migrate-diff` |
| `backend/internal/repository/migrate` | 版本化迁移执行器 + migrations 目录(init.sql 含内置默认配置) | 调整内置默认值直接改 init.sql 后重算 atlas.sum(`migrate-diff -hash`) |
| `backend/cmd/migrate-diff` | 迁移文件生成工具 | 经 `make migrate-diff NAME=xxx` 调用 |
| `backend/cmd/infosentinel/provider.go` | 应用组装 Provider(newApp / newCLIApp) | 动态依赖(runner/scheduler)不进静态 wire 图 |
| `backend/cmd/infosentinel/wire.go` | wire 注入器声明(initApp / initCLI) | 依赖图变更后 `make wire` 重新生成 |
| `backend/internal/{agent,tools,runner,...}` | 业务编排 | 通过构造参数注入依赖 |

新增表的标准流程:

1. `repository/schema/<table>.go` 定义字段与唯一键
2. `make gen` 生成 ent ORM 代码(含 Diff 函数)
3. `make migrate-diff NAME=add_tags` 生成迁移文件(`<时间戳>_add_tags.sql` + atlas.sum)
4. `repository/` 增加接口(或扩展已有接口文件)
5. `repository/data/<table>.go` 实现(参考 topic.go 的 upsert / 引用检查写法)
6. 需要初始数据则在迁移 SQL(init.sql)中补充内置数据,并重算 atlas.sum

## 数据库迁移

ent 标准版本化迁移(Atlas 引擎),`sql/versioned-migration` 特性已开启:

```bash
make gen                  # 1. 生成 ORM 代码(schema 变更后)
make migrate-diff NAME=add_tags   # 2. 生成迁移文件(内存库重放求差,不碰真实数据库)
make migrate              # 3. 验证:应用迁移(内置默认配置随迁移写入)后退出
```

- 迁移目录:`backend/internal/repository/migrate/migrations/`——
  `<UTC时间戳>_<描述>.sql` + `atlas.sum`(go:embed 进二进制)
- 执行:`backend/internal/repository/migrate`(包 `migrate`)用 atlas
  `migrate.Executor` 按版本序执行未应用迁移,版本记录在
  `atlas_schema_revisions`(Atlas CLI 兼容表);逐语句记录进度,中断可续跑;
  校验和不一致时报错
- 已发布的迁移文件与 atlas.sum 不可修改/删除;数据库版本高于迁移目录时拒绝启动
- 旧库兼容:早期自动迁移建的库无版本记录,首次迁移自动以最新版本为基线采纳
- 测试见 `backend/internal/repository/migrate/migrate_test.go`(新库、幂等、
  旧库采纳、未来版本拒绝、目录完整性、版本记录读写)

## 前端开发

```bash
cd frontend
npm run dev     # Vite 开发服务器,/api 代理到 127.0.0.1:8080
npm run build   # tsc --noEmit + vite build
```

- 技术栈:React 18 + TypeScript(strict)+ Tailwind 3 + react-router-dom
- 设计体系:`src/index.css` 中的组件类(`.card` `.btn` `.tbl` `.badge` `.sidebar` 等),
  深色模式为 `prefers-color-scheme` 自动;图标为 `src/components/icons.tsx` 内联 SVG
- 全局反馈:`useToast()` 提供 `ok()/err()`

## REST API(kratos 路由)

| 端点 | 说明 |
|---|---|
| `GET /api/overview` | 概览统计 / 下次触发 / 最近运行 / 推送目标 |
| `GET /api/config` | 当前生效配置 |
| `GET/POST /api/topics` · `PUT/DELETE /api/topics/{name}` | 主题 CRUD |
| `GET/POST /api/targets` · `PUT/DELETE /api/targets/{name}` | 推送目标 CRUD |
| `POST /api/targets/{name}/test` | 发送测试邮件 |
| `GET/POST /api/subscriptions` · `PUT/DELETE /api/subscriptions/{name}` | 订阅 CRUD(名称即订阅身份;`{name, topics[], synthesis, cron, targets[], space}`) |
| `POST /api/subscriptions/{name}/run` | 手动触发(运行中 409) |
| `GET /api/settings` · `PUT /api/settings/{llm\|mailer\|server\|storage}` | 全局设置(请求体可附带 `secret` 字段,随保存一并加密落库,对应分区密钥 `llm.api_key` / `smtp.password`;storage 段另附 `secrets` 表(`{空间名: 密码}`,加密为 `storage.<空间名>.password`)。旧库扁平 storage JSON 由 `Finalize()` 包装为 `spaces[]`) |
| `POST /api/settings/llm/test` · `POST /api/settings/mailer/test` | 测试模型 / 发送器配置(表单值,不落库) |
| `GET /api/secrets` · `DELETE /api/secrets/{name}` | 密钥名称 / 删除 |
| `POST /api/apply` | 手动重载配置并重建调度 |
| `GET /api/runs?subscription=&limit=` · `GET /api/runs/{id}` | 运行历史(subscription_runs 表持久化;记录含 `artifacts[]` 产物 DSN) |
| `GET /api/artifacts` · `GET /api/artifacts/{space}/{bucket}/{name}` | 产物空间(HTML 产物,支持 ?subscription= / ?space= 过滤;条目含 `space` 与 `dsn`) |
| `GET /api/healthz` | 健康状态 |

写操作语义:整份配置 `Finalize()` 校验 → 落库 → 自动 `Apply()` 热生效;
校验失败 400 且不落库;应用失败 200 + `{"ok":true,"applied":false,"warning":...}`。

## 日志与埋点

- 输出:JSON 行 → `log.path` + stdout(kratos log)
- 事件:`startup` `database_seeded` `scheduler_started` `scheduler_applied`
  `sub_run_done` `sub_run_failed` `api_request` `secret_saved` `shutdown`
- 字段约定见 `backend/pkg/logger` 的 `Key*` 常量;调用示例:
  `lg.Infow("sub_run_done", logger.KeyTopic, topic, logger.KeyStatus, "success")`

## 测试

```bash
make test    # go test -race ./...
```

- `data`:配置聚合读取(`LoadConfig` 回读)、引用保护、密钥加密落库与解密
- `repository`:配置模型校验表驱动(`model_test.go`);`migrate`:迁移执行与目录完整性(`migrate_test.go`)
- `crypto`:往返加解密、错误主密钥、缺少主密钥
- `logger`:JSON 行格式与级别
- `tools`:采集工具用 httptest mock;`runner`:并发守卫
- `agent`:系统/合成提示词组装锚点、标题正文拆分、单份报告短路;`render`:Markdown 转换、HTML 片段直通与防护
- 不依赖外网与真实密钥

## 部署与运维

```bash
make package
sudo tar -xzf artifacts/dist/infosentinel-*.tar.gz -C /tmp && cd /tmp/infosentinel-*/
sudo ./scripts/install.sh            # → /usr/local/infosentinel(创建 infosentinel 系统用户)
sudo systemctl start infosentinel
sudo ./scripts/uninstall.sh [--purge]
```

- 目录:`bin/ config/ .tests/`(运行时路径由 config.yaml 决定,默认 .tests/...;见 README)
- systemd 单元使用 `User=infosentinel`,环境变量文件 `/usr/local/infosentinel/config/env`
  (`INFOSENTINEL_ENCRYPT_KEY=...`;业务配置与密钥均在 Web 界面维护,不读环境变量)
- 升级:替换二进制后重启(自动迁移);可先执行 `infosentinel migrate` 预检/预迁移;
  回滚:保留旧二进制即可(若新版本已升级数据库,旧程序会拒绝启动以避免破坏数据)
- 日志:`journalctl -u infosentinel -f` 或 `.tests/logs/infosentinel.log`

## 常见问题

- **界面报“前端资产未构建”**:执行 `make frontend` 后重新 `make build`
- **加密保存失败**:未设置 `INFOSENTINEL_ENCRYPT_KEY`;设置后重启服务
- **解密失败(主密钥不匹配)**:加密主密钥被更换;恢复原主密钥或删除并重新填写密钥
- **改了 config.yaml 业务字段无效**:业务配置在数据库;请用界面修改或删库重启
- **订阅总失败**:`./artifacts/bin/infosentinel run -topic <name>` 单独排查;看结构化日志的 `sub_run_failed`
- **DuckDuckGo 不可达**:关闭 `search.enabled` 或为 `ddgsearch` 配置代理
- **serve 未按预期触发**:cron 为本地时区五段表达式;启动日志含 `schedule_registered`(next 时间)
