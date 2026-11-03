# InfoSentinel 架构设计

基于 [CloudWeGo Eino](https://github.com/cloudwego/eino)(v0.9.21)的 ReAct Agent,按"主题"自动采集资讯、总结报告并推送到多个目标。

## 配置模型:四个模块

```
sources(公用数据源池)          topics(主题 = 一段 prompt)
   ├─ rss.feeds[]                   ├─ ai-agent
   ├─ search                        └─ go-ecosystem
   ├─ hackernews                         │
   ├─ github_trending                    │ 引用
   └─ webpages[]                         │
        │ 公用,所有主题共享               │
        │                          targets(推送目标 / 通道)
        │                             ├─ mailbox(email)
        │                             └─ work-mailbox(email)
        │                                  │
        ▼                                  ▼
┌───────────────────── 任务 = {主题, 推送目标, 调度表达式} ─────────────────────┐
│  ai-agent-daily: topic=ai-agent  targets=[mailbox]  schedules=["0 8 * * *"]  │
│  go-weekly:      topic=go-ecosystem targets=[mailbox] schedules=["0 9 * * 1"]│
└──────────────────────────────────────────────────────────────────────────────┘
```

设计原则:

- **数据源是公用的**:数据源定义在全局 `sources` 模块,任何主题/任务都可用;
  每类源由一个内置工具支撑,**同类多实例共用一个工具**(如所有 RSS feed 共用
  `fetch_rss`,工具参数里指定要拉哪个源)。
- **主题是外部 prompt**:主题就是一段用户撰写的指令,定义"关注什么、如何取舍、
  报告怎么写"。同一主题可被多个任务以不同调度/目标复用。
- **任务是组合单元**:`{topic, targets[], schedules[]}`,schedules 可写多个
  cron 表达式,每个都会独立触发;targets 可写多个,报告会推送全部。

## 总体架构

```
┌──────────────────────────── cmd/infosentinel ─────────────────────────────────┐
│  run(单次,可 -task 指定) / serve(常驻,任务×表达式注册到 cron) / list        │
│  expect(LLM 交互式编辑配置:指令 → YAML 草案 → 本地校验 → diff 确认 → save)  │
└───────┬───────────────────────────────────┬───────────────────────────────────┘
        │ run                                │ serve
        ▼                                   ▼
┌── internal/runner ─────────── 每任务编排 ──────────────────────────────────────┐
│ 1. 加载任务级去重状态  out/state/<task>.json                                   │
│ 2. agent.BuildTools(sources) → 工具集 + 渠道说明(公用源,按任务注入状态)      │
│ 3. agent.New(topic) → Eino ReAct Agent                                        │
│ 4. Run(ctx) → Digest{标题, Markdown 正文}                                     │
│ 5. 逐个推送到 task.targets(pusher.Push),全部成功才保存去重状态               │
└───────┬──────────────────────────────────────────────────────────────────────┘
        │
        ▼
┌── internal/agent ───────────── Eino ReAct Agent ──────────────────────────────┐
│   system prompt = 角色 + 主题指令(topic.prompt) + 渠道说明 + 输出格式         │
│          │                                                                     │
│          ▼                                                                     │
│   ┌─────────────┐   tool calls    ┌───────── 内置工具(数据源) ────────────┐   │
│   │ ChatModel   │ ─────────────► │ fetch_rss            RSS/Atom 池        │   │
│   │ (DeepSeek,  │                │ search_web           DuckDuckGo         │   │
│   │  OpenAI 兼容)│               │ fetch_hackernews     HN Firebase API    │   │
│   └─────────────┘                │ fetch_github_trending  GitHub Trending  │   │
│          │                       │ fetch_webpage        eino-ext url loader│   │
│          ▼                       └──────────────────────────────────────────┘   │
│   Markdown 报告(首行=标题)                                                    │
└────────────────────────────────────────────────────────────────────────────────┘
        │
        ▼
┌── internal/pusher ──────────── 推送通道抽象 ───────────────────────────────────┐
│  Pusher 接口 ← EmailPusher(当前 dry-run 落盘;SMTP 预留)                      │
│  新通道(webhook / IM)= 新增 Pusher 实现 + target.type 注册,任务零改动       │
└────────────────────────────────────────────────────────────────────────────────┘
```

## 一轮任务的执行流程

1. **解析配置** `internal/config`:四模块加载、默认值、交叉引用校验
   (任务引用的主题/目标必须存在、cron 表达式必须合法、名称唯一)。
2. **任务状态**:`tools.SeenStore` 从 `out/state/<task>.json` 加载——
   **每个任务独立去重**,主题 A 报道过的内容不会影响主题 B 的视角。
3. **构建工具** `agent.BuildTools`:按 `sources` 开关实例化内置工具,
   产出 `[]tool.BaseTool` 与 `[]Channel`(渠道说明,注入系统提示词)。
4. **构建 Agent** `agent.New`:`openai.NewChatModel`(OpenAI 兼容,默认 DeepSeek)
   + `react.NewAgent`(ReAct 循环,`MaxStep` 防失控)。
5. **Agent 自主执行**:模型按主题指令决定调用哪些工具、参数、是否 `fetch_webpage`
   深读,直到产出符合格式约定的报告(首行标题 + Markdown 正文)。
6. **推送** `internal/pusher`:遍历 `task.targets`,逐个通道推送;
   单个通道失败不影响其他通道,最后汇总报错。
7. **状态持久化**:全部推送成功才把本轮 URL 写入状态文件,失败重跑不漏内容。

## Eino 概念映射

| 本项目 | Eino 概念 | 包 |
|---|---|---|
| `agent.New` 组装 | ReAct Agent(`react.NewAgent`) | `github.com/cloudwego/eino/flow/agent/react` |
| DeepSeek 接入 | ChatModel 组件(`openai.ChatModelConfig{BaseURL, Model, APIKey}`) | `github.com/cloudwego/eino-ext/components/model/openai` |
| 五个采集工具 | Invokable Tool(`utils.InferTool` 从结构体标签推导 JSON Schema) | `github.com/cloudwego/eino/components/tool/utils` |
| 网页转文本 | Document Loader(`url.NewLoader` + `Load`) | `github.com/cloudwego/eino-ext/components/document/loader/url` |

## 关键设计决策

- **工具错误不中断循环**:所有采集工具把错误序列化为 `{"error": ...}` 返回给 LLM,
  由模型换渠道或降级,而不是让整轮任务失败。
- **任务间状态隔离、公用工具**:工具实例按任务构建(注入各自的 SeenStore),
  数据源配置本身公用;两个任务互不"抢"内容。
- **采集与深读分离**:列表类工具只取标题+链接+摘要(控制 token),深读走
  `fetch_webpage`(`max_body_chars` 截断)。
- **单源失败不影响整体**:RSS 某个 feed 5xx 时以 `[feed error]` 条目回传;
  推送阶段单目标失败不影响其他目标。
- **推送通道接口化**:`Pusher` 接口 + 按 `target.type` 注册,多目标即多实例。

## 目录结构

```
cmd/infosentinel/     入口:run / serve / list / expect
internal/config/      四模块配置(sources/topics/targets/tasks)加载与校验
internal/runner/      任务编排:状态 → 工具 → agent → 推送
internal/agent/       Eino ReAct Agent(主题 prompt + 公用工具)
internal/expect/      LLM 交互式配置编辑(REPL + 草案校验 + diff)
internal/tools/       内置采集工具集 + SeenStore 去重状态
internal/pusher/      推送通道抽象(email 实现,SMTP 预留)
internal/scheduler/   cron 调度(任务 × 表达式,robfig/cron/v3)
bin/                  编译产物(make build)
out/                  运行产物:邮件文件、out/state/<task>.json 去重状态
.docs/                项目文档(本目录)
```
