# InfoSentinel 配置参考

## 两种配置,各司其职

| | 位置 | 内容 | 修改方式 |
|---|---|---|---|
| 引导配置 | `config.yaml`(安装后为 `<PREFIX>/config/config.yaml`) | `database.path`、`log.path`、`log.level` | 编辑文件后重启 |
| 业务配置 | SQLite(`database.path`) | sources / topics / targets / tasks / llm / mailer / server | **Web 管理界面**,保存即生效 |

> 数据库为空时(`infosentinel migrate` 或首次启动)自动写入**内置默认配置**
> (1 个主题、1 个 email 目标、1 个每日任务、数据源开启搜索/RSS/HN/Trending)。
> 默认值集中在 `backend/internal/repository/migrate/builtin.go`,业务配置的类型定义
> 在 `backend/internal/repository/model.go` —— `config` 包只有引导配置。之后
> config.yaml 不再承载业务配置。

## 引导配置(config.yaml)

路径相对工作目录,默认统一收在 `.tests/` 下(运行时依赖与输出);部署环境按需修改:

```yaml
database:
  path: ".tests/data/infosentinel.db"   # SQLite 文件,业务配置唯一事实源
log:
  path: ".tests/logs/infosentinel.log"  # JSON 行日志
  level: "INFO"                         # DEBUG / INFO / WARN / ERROR
```

## 业务配置字段

### sources — 公用数据源池

| 段 | 对应工具 | 字段 | 默认值 |
|---|---|---|---|
| `collect` | — | `window_hours` / `max_items_per_source` / `http_timeout_sec` / `max_body_chars` / `dedup_days` / `state_dir` | 48 / 15 / 20 / 4000 / 7 / `.tests/data/state` |
| `rss` | `fetch_rss` | `enabled`、`feeds[]{name,url}` | — |
| `search` | `search_web` | `enabled`、`region`(cn-zh)、`max_results`(8) | — |
| `hackernews` | `fetch_hackernews` | `enabled`、`top_n`(15)、`min_points`(50) | — |
| `github_trending` | `fetch_github_trending` | `enabled`、`language`、`top_n`(10) | — |
| `webpages` | `fetch_webpage` | `[]{name,url}`(非空即启用) | 空 |

### topics — 主题

`name`(唯一标识)+ `prompt`(agent 指令:关注什么、如何取舍、报告怎么写)。

### targets — 推送目标

`name`、`type`(当前仅 `email`)、`to`(收件邮箱)、`subject_prefix`(默认 `[InfoSentinel]`)。

### tasks — 任务

`name`、`topic`(引用主题)、`targets[]`(引用目标,可多个)、`schedules[]`(cron 五段,可多条)。

### llm — 模型(OpenAI 兼容)

| 字段 | 默认值 | 说明 |
|---|---|---|
| `base_url` | `https://api.deepseek.com` | 智谱 `https://open.bigmodel.cn/api/paas/v4`、OpenAI `https://api.openai.com/v1`、方舟 `https://ark.cn-beijing.volces.com/api/v3` |
| `model` | `deepseek-chat` | 模型名 |
| `api_key_env` | `DEEPSEEK_API_KEY` | 环境变量名(优先级高于加密存储) |
| `temperature` / `timeout_sec` / `max_step` | 0.3 / 300 / 25 | |

### mailer — 发送器

| 字段 | 默认值 | 说明 |
|---|---|---|
| `type` | `dryrun` | `dryrun` 落盘预览 / `smtp` 真实发送 |
| `output_dir` | `.tests/output` | dry-run 产物目录 |
| `smtp.host` / `port` / `username` / `from` | — / 465 / — / — | 发件邮箱 |
| `smtp.password_env` | `SMTP_PASSWORD` | 授权码环境变量名 |
| `smtp.ssl` | 按端口 | 465 隐式 TLS;587 配 `false`(STARTTLS) |

### server — 管理界面

`enabled`(默认 true)、`addr`(默认 `127.0.0.1:8080`,修改需重启)。

## 敏感数据(加密存储)

两处密钥可在 Web 界面「配置 → 全局设置」填写,数据库中以 **AES-256-GCM 密文**保存:

| 名称 | 用途 | 对应环境变量 |
|---|---|---|
| `llm.api_key` | 模型 API Key | `llm.api_key_env`(默认 `DEEPSEEK_API_KEY`) |
| `smtp.password` | SMTP 授权码 | `mailer.smtp.password_env`(默认 `SMTP_PASSWORD`) |

- 解析顺序:**环境变量 → 数据库密文**;两者都缺失时报错并提示
- 启用加密存储需设置主密钥:`INFOSENTINEL_ENCRYPT_KEY=<任意强随机串>`
  (经 SHA-256 派生出 AES-256 密钥;**丢失主密钥将无法解密已存数据**)
- 界面接口只回传密钥名称,不回传明文

## 环境变量汇总

| 变量 | 必需性 | 说明 |
|---|---|---|
| `DEEPSEEK_API_KEY`(名可配) | 运行任务必需 | 模型 API Key(可改用界面加密存储) |
| `SMTP_PASSWORD`(名可配) | `mailer.type=smtp` 必需 | SMTP 授权码 |
| `INFOSENTINEL_ENCRYPT_KEY` | 使用加密存储时必需 | 加密主密钥 |

## 重置与迁移

- **重置配置**:停止服务 → 删除数据库文件 → `infosentinel migrate` 或重启(重新写入内置默认配置)
- **升级**:替换二进制即可;启动时自动执行版本化迁移(不删除已有数据)。
  也可先单独执行 `infosentinel migrate`(或 `make migrate`)再启动
- **回滚限制**:数据库被更高版本程序迁移后,旧程序拒绝启动;请恢复数据库备份
- **迁移部署**:复制整个安装目录(主要是 `.tests/` 与 `config/`)
