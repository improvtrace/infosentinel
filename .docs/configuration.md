# InfoSentinel 配置参考

## 两种配置,各司其职

| | 位置 | 内容 | 修改方式 |
|---|---|---|---|
| 引导配置 | `config.yaml`(安装后为 `<PREFIX>/config/config.yaml`) | `database.path`、`log.path`、`log.level`、`storage.base_dir`、`state_dir` | 编辑文件后重启 |
| 业务配置 | SQLite(`database.path`) | topics / targets / subscriptions / llm / mailer / server / storage | **Web 管理界面**,保存即生效 |

分层原则:**引导配置只放部署路径,业务配置只放业务语义**。
业务配置不读环境变量,Web 界面是唯一修改入口;唯一的例外是加密主密钥
`INFOSENTINEL_ENCRYPT_KEY`(见下文敏感数据)。

> 数据库首次初始化(`infosentinel migrate` 或首次启动执行迁移)时写入**内置默认配置**
> (4 个内置主题、1 个 email 目标、1 条关联全部主题的每日订阅「情报日报」,投递类型为站内信)。
> 默认值集中在 init 迁移 SQL(`backend/internal/repository/migrate/migrations/`)中,
> 业务配置的类型定义在 `backend/internal/repository/model.go` —— `config` 包只有引导配置。

## 引导配置(config.yaml)

路径相对工作目录,默认统一收在 `.tests/` 下(运行时依赖与输出);部署环境按需修改:

```yaml
database:
  path: ".tests/data/infosentinel.db"   # SQLite 文件,业务配置唯一事实源
log:
  path: ".tests/logs/infosentinel.log"  # JSON 行日志
  level: "INFO"                         # DEBUG / INFO / WARN / ERROR
storage:
  base_dir: ".tests/artifacts"          # 产物空间本地磁盘根目录(推荐绝对路径)
state_dir: ".tests/data/state"          # 运行时状态(采集去重记录,<topic>.json)
```

## 业务配置字段(均在 Web 界面维护)

### 采集工具(无全局配置)

采集信息源没有全局配置:五个内置工具常驻,由模型按主题指令调用,
RSS 地址、筛选阈值等直接写在主题 prompt 里、经工具参数传入:

| 工具 | 参数 | 内置默认 |
|---|---|---|
| `fetch_rss` | `url`(RSS/Atom 源地址) | 时间窗 48h,单次 ≤15 条 |
| `fetch_webpage` | `url`(网页地址) | 正文截断 4000 字符 |
| `fetch_hackernews` | `min_points` / `top_n`(可选) | 50 分 / 15 条 |
| `fetch_github_trending` | `language`(可选) | 前 10 个仓库 |
| `search_web` | `query`、`time_range`(d/w/m) | 区域 cn-zh,≤8 条 |

采集通用参数(HTTP 超时 20s、URL 去重保留 7 天等)为
`backend/pkg/tools` 的内置常量,不提供配置。

> 历史版本的 `sources` 设置段(公用数据源池)已移除,旧库中残留的该设置行
> 启动时自动删除;去重状态目录 `state_dir` 此前也已移到引导配置 `config.yaml`。

### topics — 主题

`name`(唯一标识)+ `prompt`(agent 指令:关注什么、如何取舍、报告怎么写)。

### targets — 推送目标

`name`、`type`(当前仅 `email`)、`to`(收件邮箱)、`subject_prefix`(默认 `[InfoSentinel]`)。

### subscriptions — 订阅

`name`(订阅的唯一标识,产物文件与运行记录按它命名)、`topics[]`(引用主题,**可多个**,
订阅与主题多对多:一个主题可被多个订阅引用)、`synthesis`(合成提示词:多主题各自采集后
按它合并为一份报告,新建为空时回填内置默认值 —— 默认要求输出 HTML 正文片段;
正文为 Markdown 时推送前自动转换,HTML 片段则直通渲染)、`cron`(单个 cron 五段表达式)、
`targets[]`(引用目标,可多选)、`space`(引用产物空间,**单个**;空 = 不写产物空间)。
订阅 = 名称 + 多主题(合并为一份报告)+ cron + 推送目标 + 产物空间,在侧边栏「订阅管理」维护。

### llm — 模型(OpenAI 兼容)

| 字段 | 默认值 | 说明 |
|---|---|---|
| `base_url` | `https://api.deepseek.com` | 智谱 `https://open.bigmodel.cn/api/paas/v4`、OpenAI `https://api.openai.com/v1`、方舟 `https://ark.cn-beijing.volces.com/api/v3` |
| `model` | `deepseek-chat` | 模型名 |
| `temperature` / `timeout_sec` / `max_step` | 0.3 / 300 / 25 | 采样温度 / 单次请求超时(秒)/ agent 最大循环步数 |

API Key 不在模型配置里:在「配置 → 全局设置」中填写并加密保存(名称 `llm.api_key`)。

### mailer — 发送

| 字段 | 默认值 | 说明 |
|---|---|---|
| `type` | `站内信` | `站内信`:报告写入产物空间,界面在线查看;`smtp`:通过内置发件邮箱真实发送 HTML 邮件 |
| `smtp.host` / `port` / `username` / `from` | — / 465 / — / — | 发件邮箱(type=smtp 时 host 与 from 必填) |
| `smtp.ssl` | 按端口 | 465 隐式 TLS;587 配 `false`(STARTTLS) |

> 历史版本的投递类型值 `dryrun` 已更名为 `站内信`;升级后首次加载配置时自动归一化,无需手工处理。
> SMTP 授权码不在发送配置里:在「配置 → 全局设置」中填写并加密保存(名称 `smtp.password`)。

### storage — 产物空间(可配置多个)

`spaces[]` 为产物空间列表,可同时配置多个;订阅按 `name` 引用其中一个,站内信报告
写入订阅关联的那个空间。产物引用路径采用 DSN 格式:`类型://空间名/分桶/文件名`
(如 `local://默认空间/2026-11-11/world-situation-20261111-080000.html`),记入运行记录的
产物路径列。每个空间的字段:

| 字段 | 默认值 | 说明 |
|---|---|---|
| `name` | — | 空间唯一标识,订阅按名引用 |
| `type` | `local` | `local`(本地磁盘)/ `sftp` / `ftp`,驱动实现见 `backend/pkg/storage` |
| `path` | `""` | 产物路径(相对根目录;本地实际路径 = `storage.base_dir` + 本路径) |
| `period_days` / `max_dirs` | 1 / 30 | 每 m 天的产物存入同一分桶子目录(以起始日期命名),最多保留 n 个,超出删最旧 |
| `sftp.host/port/username` | — | SFTP 连接;`key_path` 私钥可选,`host_key` 指纹校验可选,`base_dir` 远端基础目录 |
| `ftp.host/port/username` | — | FTP 连接;`base_dir` 远端基础目录,`tls` 启用 FTPS(显式 TLS) |

SFTP/FTP 登录密码在「配置 → 全局设置」逐空间填写并加密保存(名称 `storage.<空间名>.password`;
本地驱动或私钥登录无需密码)。旧库只有全局 `storage.password` 时自动回退,升级无需重录。
旧版单空间扁平 storage 配置在升级后首次加载时自动包装为单空间列表,无需手工处理。

### server — 管理界面

`enabled`(默认 true)、`addr`(默认 `127.0.0.1:8080`,修改需重启)。

## 敏感数据(加密存储)

三处敏感数据在 Web 界面「配置 → 全局设置」填写,数据库中以 **AES-256-GCM 密文**保存,
**数据库密文是唯一来源**(不读环境变量):

| 名称 | 用途 | 何时必需 |
|---|---|---|
| `llm.api_key` | 模型 API Key | 运行订阅必需 |
| `smtp.password` | SMTP 密码/授权码 | `mailer.type=smtp` 必需 |
| `storage.<空间名>.password` | 逐空间 SFTP/FTP 登录密码 | 空间走 sftp/ftp 且用密码登录时(缺失回退旧版 `storage.password`) |

- 启用加密存储需设置主密钥:`INFOSENTINEL_ENCRYPT_KEY=<任意强随机串>`
  (经 SHA-256 派生出 AES-256 密钥;**丢失主密钥将无法解密已存数据**)
- 界面接口只回传密钥名称,不回传明文;「测试」端点可用表单值临时验证,不落库

## 环境变量汇总

业务配置不读环境变量。唯一的例外是加密基础设施的主密钥:

| 变量 | 必需性 | 说明 |
|---|---|---|
| `INFOSENTINEL_ENCRYPT_KEY` | 使用加密存储时必需 | 加密主密钥(引导层,不能存进它加密的数据库) |

## 重置与迁移

- **重置配置**:停止服务 → 删除数据库文件 → `infosentinel migrate` 或重启(重新写入内置默认配置)
- **升级**:替换二进制即可;启动时自动执行版本化迁移(不删除已有数据)。
  也可先单独执行 `infosentinel migrate`(或 `make migrate`)再启动。
  历史配置兼容:旧库中 `mailer.type=dryrun` 自动归一化为 `站内信`;
  旧库残留的 `sources` 设置段(公用数据源池)启动时自动删除;
  旧的环境变量密钥(`api_key_env` / `password_env` 字段)不再读取,
  请升级后在「全局设置」中重新填写并加密保存
- **回滚限制**:数据库被更高版本程序迁移后,旧程序拒绝启动;请恢复数据库备份
- **迁移部署**:复制整个安装目录(主要是 `.tests/` 与 `config/`)
