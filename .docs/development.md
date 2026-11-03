# InfoSentinel 开发指南

## 环境要求

- Go ≥ 1.24(开发环境为 Go 1.26)
- 网络可访问:所配的 LLM 服务、RSS 源、`hacker-news.firebaseio.com`、`github.com/trending`
  - 依赖拉取走 Go 模块代理,国内建议 `GOPROXY=https://goproxy.cn,direct`
  - DuckDuckGo 搜索在部分网络环境下不可达:工具会以 `{"error": ...}` 回传,
    Agent 自动换渠道,不影响整轮任务;确认不可用可在配置中 `search.enabled: false`

## 构建与运行

统一通过 Makefile(产物位置:二进制在 `bin/`,运行数据在 `out/`):

```bash
# 获取依赖 / 构建(产物:bin/infosentinel,自动注入 git 版本号)
make tidy
make build

# 准备配置与密钥
cp config.example.yaml config.yaml   # 编辑 topics prompt、targets 收件箱、tasks 调度
export DEEPSEEK_API_KEY=sk-...       # 或你在 llm.api_key_env 里指定的变量名

# 查看配置概览:主题 / 推送目标 / 任务
./bin/infosentinel list

# 执行任务;会自动先编译
make run                             # 全部任务
make run ARGS="-task ai-agent-daily" # 只跑指定任务

# 常驻运行,按各任务 schedules 定时执行
make serve

# 指定配置文件 / 覆盖版本号 / 查看全部命令
./bin/infosentinel -c /path/to/config.yaml run -task go-weekly
make build VERSION=v1.0.0
make help
```

dry-run 模式下,邮件输出为 `out/email-<task>-<时间戳>.md`,文件头部的 HTML 注释
包含 `To` / `Subject` / `Task`,可直接预览效果。多任务多目标时每个推送
各生成一个文件。

## 测试与静态检查

```bash
make check   # 提交前完整校验:vet + test + build
make test    # 等价 go test -race ./...
make vet     # 等价 go vet ./...
make cover   # 覆盖率报告写入 out/coverage.out
make fmt     # gofmt -s 格式化
make clean   # 清理 bin/ 与覆盖率(不影响 out/ 下的运行数据)
```

测试不依赖外网与真实 API Key:RSS / HN / GitHub Trending / 网页工具均用
`httptest` 起本地 mock 服务验证;配置校验用例覆盖交叉引用与 cron 合法性。

## 配置速记

- **新主题** = `topics` 加一段 prompt → `tasks` 里引用它。
- **推送到更多邮箱** = `targets` 加 email 目标 → 任务的 `targets` 列表里追加名字。
- **同主题多节奏** = 一个任务写多个 `schedules`,或多个任务复用同一 `topic`。
- **加数据源** = `sources` 里启用/增删实例(如 rss 加 feed),所有主题立即可用。
- 全字段说明见 [configuration.md](configuration.md)。
- 不想手改 YAML?用 `./bin/infosentinel expect` 用自然语言编辑(见下节)。

## expect:LLM 交互式配置编辑

`expect` 启动一个 REPL 会话,把自然语言指令转成配置修改:

```bash
./bin/infosentinel expect
# expect> 加一个 RSS 源:名字"阮一峰的网络日志" url https://www.ruanyifeng.com/blog/atom.xml
# expect> 把 ai-agent-daily 的调度改成每周一和周四的 9 点
# expect> save
```

工作方式(`internal/expect`):

1. 每条指令连同**当前配置全文**发给 LLM,要求其输出修改后的完整 YAML(单个 ```yaml 代码块);
2. 提取代码块后用 `config.Parse` 在本地做解析 + 默认值 + 交叉引用/cron 校验,
   不通过时把错误回传给模型自动重试(最多 3 次);
3. 通过后展示**行级 diff**(内置 LCS 实现,无外部依赖),确认 `y` 才应用到内存;
4. `save` 写回前自动备份原文件为 `<config>.bak`;`show`/`diff`/`reset`/`quit` 管理会话状态;
5. 会话保留最近 12 条对话消息,支持"再把时间改到 9 点"这类增量表述。

安全设计:任何修改都不会直接落盘——必须经过校验 + diff + 确认 + save 四道关;
LLM 无法触达 expect 会话之外的能力(没有工具、没有文件写权限)。

## 切换 LLM 服务

所有走 OpenAI 兼容协议的服务只需改三处配置(代码零改动):

```yaml
llm:
  base_url: "https://open.bigmodel.cn/api/paas/v4"   # 智谱 GLM
  model: "glm-4.6"
  api_key_env: "ZHIPU_API_KEY"
```

| 服务 | base_url | 模型示例 |
|---|---|---|
| DeepSeek(默认) | `https://api.deepseek.com` | `deepseek-chat` |
| 智谱 GLM | `https://open.bigmodel.cn/api/paas/v4` | `glm-4.6` |
| OpenAI | `https://api.openai.com/v1` | `gpt-4o-mini` |
| 火山方舟 | `https://ark.cn-beijing.volces.com/api/v3` | `doubao-...` |

## 接入真实邮件发送(SMTP)

当前 `mailer.type: dryrun` 只落盘。接 SMTP 的步骤:

1. `internal/pusher/pusher.go` 中 `EmailPusher.Push` 的 smtp 分支目前返回
   "尚未实现";在 `internal/pusher/` 新增 `smtp.go` 补上实现(host/port/
   username/password 授权码,密码走环境变量),可用 `gopkg.in/mail.v2`;
2. 配置 `mailer.type: smtp` 并补充 `mailer.smtp` 段;
3. (可选)正文 Markdown → HTML 转换,可用 `github.com/russross/blackfriday/v2`。

QQ 邮箱 / 163 / Gmail 均支持 SMTP 授权码方式,注意各家发信频率限制。

## 新增推送通道

1. 在 `internal/pusher/` 新增实现,实现 `Pusher` 接口:
   `Push(ctx, *Message) error`(`Message` 含 Task/Subject/Body);
2. 在 `pusher.New` 的 switch 中按新的 `target.type` 注册;
3. 在 `internal/config` 的 Target 校验白名单中加入该类型,并在 `targets` 中配置。

## 新增数据源(工具)

1. 在 `internal/tools/` 新建文件,参考 `rss.go` 的模式:

   ```go
   type myParams struct {
       Topic string `json:"topic" jsonschema_description:"..."`
   }
   func (t *myTool) invoke(ctx context.Context, p *myParams) (string, error) {
       // 采集 → 过滤(时间窗/去重:t.store.Filter/Mark)→ friendlyResult(...) 返回
   }
   return utils.InferTool("my_tool", "工具描述(给 LLM 看,写清楚何时该用)", t.invoke)
   ```

2. `internal/config` 的 `Sources` 增加对应配置段与开关 + 校验;
3. 在 `internal/agent/BuildTools` 中按开关注册并生成 `Channel` 说明(含实例清单),
   系统提示词会自动列出。

约定:工具永不返回 Go 层 error(网络失败等以 `{"error": ...}` JSON 返回),
避免打断 ReAct 循环。

## 常见问题

- **`环境变量 DEEPSEEK_API_KEY 未设置`**:启动前 export 对应变量;变量名可配。
- **某个任务总失败**:`serve` 下单任务失败不影响其他任务;先用
  `./bin/infosentinel run -task <name>` 单独排查。
- **搜索工具总是返回 error**:多为网络到 DuckDuckGo 不通,关掉 `search.enabled` 或配代理
  (`internal/tools/search.go` 中 `ddgsearch.Config{Proxy: ...}`)。
- **某任务内容重复 / 为空**:去重状态按任务隔离在 `out/state/<task>.json`;
  删除该文件可重置该任务的状态。反之内容总重复,确认 `dedup_days` 足够长。
- **serve 没有按预期触发**:`schedules` 为本地时区的五段 cron;用
  `crontab.guru` 类工具校验表达式;启动日志会打印每个 (任务, 表达式) 的下次触发时间。
