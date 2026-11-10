# InfoSentinel 配置参考

## 两种配置,各司其职

| | 位置 | 内容 | 修改方式 |
|---|---|---|---|
| 引导配置 | `config.yaml`(安装后为 `<PREFIX>/config/config.yaml`) | `database.path`、`log.path`、`log.level`、`storage.base_dir`、`state_dir` | 编辑文件后重启 |
| 业务配置 | SQLite(`database.path`) | sources / topics / targets / subscriptions / llm / mailer / server / storage | **Web 管理界面**,保存即生效 |

分层原则:**引导配置只放部署路径,业务配置只放业务语义**。
业务配置不读环境变量,Web 界面是唯一修改入口;唯一的例外是加密主密钥
`INFOSENTINEL_ENCRYPT_KEY`(见下文敏感数据)。

> 数据库为空时(`infosentinel migrate` 或首次启动)自动写入**内置默认配置**
> (1 个主题、1 个 email 目标、1 条每日订阅、数据源开启搜索/RSS/HN/Trending,
> 投递类型为站内信)。默认值集中在 `backend/internal/repository/migrate/builtin.go`,
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

### sources — 公用数据源池

| 段 | 对应工具 | 字段 | 默认值 |
|---|---|---|---|
| `collect` | — | `window_hours` / `max_items_per_source` / `http_timeout_sec` / `max_body_chars` / `dedup_days` | 48 / 15 / 20 / 4000 / 7 |
| `rss` | `fetch_rss` | `enabled`、`feeds[]{name,url}` | — |
| `search` | `search_web` | `enabled`、`region`(cn-zh)、`max_results`(8) | — |
| `hackernews` | `fetch_hackernews` | `enabled`、`top_n`(15)、`min_points`(50) | — |
| `github_trending` | `fetch_github_trending` | `enabled`、`language`、`top_n`(10) | — |
| `webpages` | `fetch_webpage` | `[]{name,url}`(非空即启用) | 空 |

> 历史版本曾把去重状态目录 `state_dir` 放在这里;它属于部署路径,
> 现已移到引导配置 `config.yaml` 的 `state_dir`。旧库中残留的该字段会被忽略。

### topics — 主题

`name`(唯一标识)+ `prompt`(agent 指令:关注什么、如何取舍、报告怎么写)。

### targets — 推送目标

`name`、`type`(当前仅 `email`)、`to`(收件邮箱)、`subject_prefix`(默认 `[InfoSentinel]`)。

### subscriptions — 订阅

`topic`(引用主题,订阅的唯一标识:一个主题最多一条订阅)、`cron`(单个 cron 五段表达式)、
`targets[]`(引用目标,可多个)。订阅 = 主题 + cron + 推送目标,在侧边栏「订阅管理」维护。

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

### storage — 产物空间

| 字段 | 默认值 | 说明 |
|---|---|---|
| `type` | `local` | `local`(本地磁盘)/ `sftp` / `ftp`,驱动实现见 `backend/pkg/storage` |
| `path` | `""` | 产物路径(相对根目录;本地实际路径 = `storage.base_dir` + 本路径) |
| `period_days` / `max_dirs` | 1 / 30 | 每 m 天的产物存入同一分桶子目录(以起始日期命名),最多保留 n 个,超出删最旧 |
| `sftp.host/port/username` | — | SFTP 连接;`key_path` 私钥可选,`host_key` 指纹校验可选,`base_dir` 远端基础目录 |
| `ftp.host/port/username` | — | FTP 连接;`base_dir` 远端基础目录,`tls` 启用 FTPS(显式 TLS) |

SFTP/FTP 登录密码在「配置 → 全局设置」中填写并加密保存(名称 `storage.password`;
本地驱动或私钥登录无需密码)。

### server — 管理界面

`enabled`(默认 true)、`addr`(默认 `127.0.0.1:8080`,修改需重启)。

## 敏感数据(加密存储)

三处敏感数据在 Web 界面「配置 → 全局设置」填写,数据库中以 **AES-256-GCM 密文**保存,
**数据库密文是唯一来源**(不读环境变量):

| 名称 | 用途 | 何时必需 |
|---|---|---|
| `llm.api_key` | 模型 API Key | 运行订阅必需 |
| `smtp.password` | SMTP 密码/授权码 | `mailer.type=smtp` 必需 |
| `storage.password` | SFTP/FTP 登录密码 | 产物空间走 sftp/ftp 且用密码登录时 |

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
  旧的环境变量密钥(`api_key_env` / `password_env` 字段)不再读取,
  请升级后在「全局设置」中重新填写并加密保存
- **回滚限制**:数据库被更高版本程序迁移后,旧程序拒绝启动;请恢复数据库备份
- **迁移部署**:复制整个安装目录(主要是 `.tests/` 与 `config/`)
