# InfoSentinel 配置参考

配置文件默认为工作目录下的 `config.yaml`(可用 `-c` 指定),从 `config.example.yaml` 复制起步。
配置分四个模块:**sources(公用数据源)/ topics(主题)/ targets(推送目标)/ tasks(任务)**,
外加全局的 `llm` 与 `mailer`。

## sources — 公用数据源池

数据源是公用的:所有主题共享;每类源由一个内置工具支撑,同类多实例共用该工具。

| 段 | 对应工具 | 字段 | 默认值 | 说明 |
|---|---|---|---|---|
| `collect` | — | `window_hours` | `48` | 时间窗:RSS / HN 只取最近 N 小时内条目 |
| | | `max_items_per_source` | `15` | 单工具单次返回最大条目数 |
| | | `http_timeout_sec` | `20` | 采集 HTTP 超时 |
| | | `max_body_chars` | `4000` | `fetch_webpage` 正文截断字符数 |
| | | `dedup_days` | `7` | 已采集 URL 的去重保留天数 |
| | | `state_dir` | `"out/state"` | 去重状态目录,每任务一个文件 `<state_dir>/<task>.json` |
| `rss` | `fetch_rss` | `enabled` | — | 启用时 `feeds[]` 必填;每项 `name`(工具参数用)+ `url` |
| `search` | `search_web` | `enabled` | — | DuckDuckGo,无需 API Key |
| | | `region` / `max_results` | `cn-zh` / `8` | 搜索区域 / 结果数 |
| `hackernews` | `fetch_hackernews` | `enabled` | — | HN 热榜 |
| | | `top_n` / `min_points` | `15` / `50` | 取前 N 条 / 最低分过滤 |
| `github_trending` | `fetch_github_trending` | `enabled` | — | GitHub Trending |
| | | `language` / `top_n` | 空 / `10` | 语言过滤(空=全部)/ 取前 N |
| `webpages` | `fetch_webpage` | — | 空 | 固定网页列表(`name`+`url`),**非空即启用** |

> 至少启用一个数据源,否则启动报错。

## topics — 主题

每个主题就是一段你撰写的 prompt,agent 按它采集与撰写报告:

```yaml
topics:
  - name: "ai-agent"          # 唯一标识,任务用 topic 引用
    prompt: |
      关注 LLM 与 AI Agent 应用开发生态……   # 必填
```

校验:`name` 唯一且非空;`prompt` 必填。主题不含调度和目标——那是任务的事。

## targets — 推送目标(通道)

```yaml
targets:
  - name: "mailbox"
    type: "email"
    to: "me@example.com"
    subject_prefix: "[InfoSentinel]"     # 默认 [InfoSentinel]
```

- 当前支持 `type: email`;可配置任意多个目标,任务里引用即推送到全部。
- 新通道类型(webhook、IM 等)= 新增 `Pusher` 实现,配置面不变。
- `name` 唯一;email 通道 `to` 必填。

## tasks — 任务:{主题, 推送目标, 调度表达式}

```yaml
tasks:
  - name: "ai-agent-daily"
    topic: "ai-agent"
    targets: ["mailbox"]                # 可多个
    schedules: ["0 8 * * *"]            # 可多个,每个表达式独立触发
```

校验:`name` 唯一;`topic`/`targets` 引用必须存在;`targets` 无重复;
`schedules` 非空且每项为合法 cron 五段表达式(本地时区)。

**同一主题想要多时间推送**:一个任务写多个 schedules;想要不同目标各自节奏,
就建多个任务引用同一主题。

## llm — 模型服务(全局,OpenAI 兼容协议)

| 字段 | 默认值 | 说明 |
|---|---|---|
| `base_url` | `https://api.deepseek.com` | 切换服务商只改这里(智谱 `https://open.bigmodel.cn/api/paas/v4`、OpenAI `https://api.openai.com/v1`、方舟 `https://ark.cn-beijing.volces.com/api/v3`) |
| `model` | `"deepseek-chat"` | 模型名 |
| `api_key_env` | `"DEEPSEEK_API_KEY"` | 读取 API Key 的环境变量名,Key 不落配置文件 |
| `temperature` | `0.3` | 采样温度 |
| `timeout_sec` | `300` | 单次模型请求超时(也是 serve 每轮任务的超时上限) |
| `max_step` | `25` | ReAct 循环最大步数 |

## mailer — 发送器(全局)

| 字段 | 默认值 | 说明 |
|---|---|---|
| `mailer.type` | `"dryrun"` | `dryrun` 落盘不发送;`smtp` 预留 |
| `mailer.output_dir` | `"out"` | dry-run 邮件文件目录,命名 `email-<task>-<时间戳>.md` |

## 环境变量

| 变量 | 说明 |
|---|---|
| `DEEPSEEK_API_KEY`(名可由 `llm.api_key_env` 改) | 模型 API Key,启动前必须设置 |

## 最小可用配置

```yaml
sources:
  search: {enabled: true}
topics:
  - name: t
    prompt: "关注 AI Agent 领域动态,中文输出。"
targets:
  - {name: m, type: email, to: "me@example.com"}
tasks:
  - {name: daily, topic: t, targets: [m], schedules: ["0 8 * * *"]}
```

其余全部走默认值:dry-run 输出到 `out/`。
