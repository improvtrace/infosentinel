# InfoSentinel

基于 [CloudWeGo Eino](https://github.com/cloudwego/eino) 的兴趣资讯收集 Agent:配置公用数据源池,为主题(一段你撰写的 prompt)自动采集内容、总结成结构化报告,按 `{主题, 推送目标, 调度表达式}` 组成任务定时推送到多个邮箱(当前 dry-run,SMTP 已预留)。

## 特性

- **Eino ReAct Agent**:模型自主决策调用哪些采集工具、何时深读网页、如何取舍
- **expect 交互式配置编辑**:自然语言指挥 LLM 修改配置,草案校验 + diff 确认后才落盘
- **配置四模块**:`sources`(公用数据源)/ `topics`(主题 prompt)/ `targets`(推送目标)/ `tasks`(任务)
- **多数据源共用**:RSS、DuckDuckGo 搜索、HN 热榜、GitHub Trending、固定网页,同类源共用一个工具,所有主题共享
- **多推送目标**:任务可引用多个通道,当前支持 email(多邮箱),接口化可扩展
- **任务化调度**:每个任务挂多条 cron 表达式,独立触发、独立去重状态
- **模型可换**:OpenAI 兼容协议,DeepSeek / 智谱 GLM / OpenAI / 火山方舟一键切换

## 快速开始

```bash
make build                           # 产物 bin/infosentinel(版本号自动从 git 注入)

cp config.example.yaml config.yaml   # 编辑 topics prompt、targets 收件箱、tasks 调度
export DEEPSEEK_API_KEY=sk-...

./bin/infosentinel list              # 查看主题 / 目标 / 任务
./bin/infosentinel expect            # LLM 交互式编辑配置(自然语言 → diff 确认)
make run                             # 执行全部任务(可 ARGS="-task <name>")
make serve                           # 常驻,按各任务 schedules 定时
```

邮件生成在 `out/email-<task>-<时间戳>.md`。

## 文档

- [架构设计](.docs/architecture.md) — 四模块配置模型、组件与数据流、Eino 概念映射
- [配置参考](.docs/configuration.md) — `config.yaml` 全字段说明
- [开发指南](.docs/development.md) — 测试、切换 LLM、接 SMTP、加通道/数据源

## 项目结构

```
cmd/infosentinel/     入口(run / serve / list)
internal/config/      sources / topics / targets / tasks 四模块配置
internal/runner/      任务编排:去重状态 → 工具 → agent → 推送
internal/agent/       Eino ReAct Agent(主题 prompt + 公用工具)
internal/tools/       内置采集工具 + SeenStore 去重(任务级隔离)
internal/pusher/      推送通道抽象(email 实现,SMTP 预留)
internal/scheduler/   cron 调度(任务 × 表达式)
```
