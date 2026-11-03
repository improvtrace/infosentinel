// Package expect 提供交互式配置编辑会话:用自然语言指挥 LLM 修改 infosentinel 配置。
//
// 流程:用户输入修改指令 → LLM 基于当前配置生成完整 YAML 草案 →
// 本地解析与校验(不合法自动带错误重试)→ 展示行级 diff → 确认后应用到内存 →
// save 命令写回配置文件(自动备份)。会话保留最近若干轮对话,支持
// "再把调度改成每周一"这类增量表述。
package expect

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/schema"

	"github.com/tomasgdl/infosentinel/internal/agent"
	"github.com/tomasgdl/infosentinel/internal/config"
)

// maxHistoryMessages 是滚动对话历史上限(条),防止上下文无限增长。
const maxHistoryMessages = 12

const systemPrompt = `你是 InfoSentinel 的配置编辑助手。InfoSentinel 的配置分四个模块:
- sources:公用数据源(rss / search / hackernews / github_trending / webpages,以及通用 collect 参数)
- topics:主题,每项为一段 prompt(name + prompt)
- targets:推送目标,当前 type 仅支持 email(name/type/to/subject_prefix)
- tasks:任务 = {name, topic, targets[], schedules[]},schedules 为 cron 五段表达式
另有全局 llm(OpenAI 兼容:base_url/model/api_key_env/temperature/timeout_sec/max_step)与 mailer(type: dryrun|smtp, output_dir)。

规则:
1. 收到指令后,输出修改后的【完整 YAML 配置】,只放一个 ` + "```yaml" + ` 代码块,不要输出任何解释文字。
2. 不要发明不存在的字段;保持既有结构与命名风格;保留原有中文注释,并在修改处同步更新注释。
3. tasks.targets 必须引用已存在的 target 名;tasks.topic 必须引用已存在的主题名;schedules 必须是合法 cron(分 时 日 月 周)。
4. 指令含糊或与配置无关时,原样返回当前 YAML(即不修改)。`

// Session 是一次 expect 交互会话。
type Session struct {
	cm      *openai.ChatModel
	path    string
	origRaw string // 最近一次保存(或会话开始)时的文件内容,diff/save 的基准
	workRaw string // 当前工作区内容(已应用但未必保存)
	history []*schema.Message

	in  *bufio.Reader
	out io.Writer
}

// New 创建会话:校验 API Key、读取配置文件作为初始工作区。
func New(ctx context.Context, cfg *config.Config, path string) (*Session, error) {
	cm, err := agent.NewChatModel(ctx, cfg)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件 %s: %w", path, err)
	}
	return &Session{
		cm:      cm,
		path:    path,
		origRaw: string(raw),
		workRaw: string(raw),
		in:      bufio.NewReader(os.Stdin),
		out:     os.Stdout,
	}, nil
}

// Run 运行 REPL 循环,直到 quit/exit 或输入流结束。
func (s *Session) Run(ctx context.Context) error {
	s.printf("InfoSentinel 配置助手(expect)— %s\n", s.path)
	s.printf("输入自然语言修改指令;内置命令:show / diff / save / reset / help / quit\n")
	s.printf("提示:修改仅在确认后应用到内存,save 才写回文件(自动备份 .bak)\n\n")

	for {
		s.printf("expect> ")
		line, err := s.in.ReadString('\n')
		cmd := strings.TrimSpace(line)
		if err != nil && cmd == "" {
			s.printf("\n")
			return s.quit()
		}
		if cmd == "" {
			continue
		}
		switch strings.ToLower(strings.Fields(cmd)[0]) {
		case "quit", "exit", "q":
			return s.quit()
		case "show":
			s.printf("%s\n", s.workRaw)
		case "diff":
			s.showDiff()
		case "save":
			s.save()
		case "reset":
			s.workRaw = s.origRaw
			s.printf("[expect] 工作区已重置到最近一次保存的版本\n")
		case "help", "?":
			s.printHelp()
		default:
			if err := s.edit(ctx, cmd); err != nil {
				s.printf("[expect] 出错:%v\n", err)
			}
		}
	}
}

// edit 处理一次自然语言修改指令:生成草案 → 校验 → diff → 确认应用。
func (s *Session) edit(ctx context.Context, instruction string) error {
	s.printf("[expect] 正在生成修改草案…\n")
	newRaw, err := s.ask(ctx, instruction)
	if err != nil {
		return err
	}

	d := Diff(s.workRaw, newRaw)
	if d == "" {
		s.printf("[expect] 该指令不会引起配置变化\n")
		return nil
	}
	s.printf("%s", d)

	if !s.confirm("应用以上修改? [y/N]") {
		s.printf("[expect] 已丢弃草案\n")
		return nil
	}
	s.workRaw = newRaw
	s.printf("[expect] 已应用(未保存)。输入 save 写回 %s\n", s.path)
	return nil
}

// ask 调用 LLM 生成新的完整配置 YAML:校验不通过时把错误回传给模型重试(最多 3 次)。
func (s *Session) ask(ctx context.Context, instruction string) (string, error) {
	msgs := s.buildMessages(instruction)

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		resp, err := s.cm.Generate(ctx, msgs)
		if err != nil {
			return "", fmt.Errorf("调用模型失败: %w", err)
		}
		raw := extractYAML(resp.Content)
		if _, err := config.Parse([]byte(raw)); err == nil {
			s.remember(instruction, raw)
			return raw, nil
		}
		lastErr = err
		s.printf("[expect] 第 %d 次草案未通过校验:%v,要求模型修正…\n", attempt+1, err)
		msgs = append(msgs,
			&schema.Message{Role: schema.Assistant, Content: resp.Content},
			&schema.Message{Role: schema.User, Content: fmt.Sprintf("以上 YAML 不合法:%v。请修正后重新输出完整配置,仍然只用一个 ```yaml 代码块。", err)},
		)
	}
	return "", fmt.Errorf("模型多次输出均未通过校验,最后一次错误:%w", lastErr)
}

// buildMessages 组装本轮请求:系统提示 + 滚动历史 + 当前配置与指令。
func (s *Session) buildMessages(instruction string) []*schema.Message {
	msgs := []*schema.Message{{Role: schema.System, Content: systemPrompt}}
	msgs = append(msgs, s.history...)
	msgs = append(msgs, &schema.Message{
		Role: schema.User,
		Content: "当前配置:\n```yaml\n" + s.workRaw +
			"\n```\n\n修改指令:" + instruction,
	})
	return msgs
}

// remember 把成功的 (指令, 草案) 对写入滚动历史。
func (s *Session) remember(instruction, raw string) {
	s.history = append(s.history,
		&schema.Message{Role: schema.User, Content: "修改指令:" + instruction},
		&schema.Message{Role: schema.Assistant, Content: "```yaml\n" + raw + "\n```"},
	)
	if len(s.history) > maxHistoryMessages {
		s.history = s.history[len(s.history)-maxHistoryMessages:]
	}
}

// showDiff 打印工作区相对最近一次保存的差异。
func (s *Session) showDiff() {
	d := Diff(s.origRaw, s.workRaw)
	if d == "" {
		s.printf("[expect] 无未保存的修改\n")
		return
	}
	s.printf("%s", d)
}

// save 把工作区写回配置文件,原内容备份为 <path>.bak。
func (s *Session) save() {
	if s.workRaw == s.origRaw {
		s.printf("[expect] 没有需要保存的修改\n")
		return
	}
	bak := s.path + ".bak"
	if err := os.WriteFile(bak, []byte(s.origRaw), 0o644); err != nil {
		s.printf("[expect] 写备份 %s 失败:%v(已放弃保存)\n", bak, err)
		return
	}
	if err := os.WriteFile(s.path, []byte(s.workRaw), 0o644); err != nil {
		s.printf("[expect] 写 %s 失败:%v\n", s.path, err)
		return
	}
	s.origRaw = s.workRaw
	s.printf("[expect] 已保存到 %s(原内容备份在 %s)\n", s.path, bak)
}

// quit 退出会话;有未保存修改时需要确认。
func (s *Session) quit() error {
	if s.workRaw != s.origRaw {
		if !s.confirm("有未保存的修改,丢弃并退出? [y/N]") {
			s.printf("[expect] 已取消退出;输入 save 可保存修改\n")
			return nil
		}
	}
	s.printf("再见\n")
	return nil
}

// confirm 从标准输入读取 y/n 确认。
func (s *Session) confirm(prompt string) bool {
	s.printf("%s ", prompt)
	line, err := s.in.ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes", "是":
		return true
	default:
		return false
	}
}

func (s *Session) printHelp() {
	s.printf(`
命令:
  show    打印当前工作区配置
  diff    打印相对最近一次保存的差异
  save    写回配置文件(备份为 <file>.bak)
  reset   丢弃未保存的修改
  quit    退出(有未保存修改时会确认)
其余输入都会作为修改指令发给 LLM,例如:
  加一个 RSS 源:名字"阮一峰的网络日志" url https://www.ruanyifeng.com/blog/atom.xml
  把 ai-agent-daily 的调度改成每周一和周四的 9 点
  再加一个推送目标:公司邮箱 work@corp.com,标题前缀 [Work]
`)
}

func (s *Session) printf(format string, a ...any) {
	fmt.Fprintf(s.out, format, a...)
}

// fenceRe 匹配 ```yaml … ``` / ``` … ``` 代码块。
var fenceRe = regexp.MustCompile("(?s)```(?:yaml)?[ \t]*\\r?\\n(.*?)```")

// extractYAML 从 LLM 回复中提取配置 YAML:优先取能通过解析校验的围栏代码块,
// 其次取第一个围栏块;没有围栏时将全文视为 YAML。
func extractYAML(content string) string {
	blocks := fenceRe.FindAllStringSubmatch(content, -1)
	if len(blocks) == 0 {
		return strings.TrimSpace(content)
	}
	for _, m := range blocks {
		candidate := strings.TrimSpace(m[1])
		if _, err := config.Parse([]byte(candidate)); err == nil {
			return candidate
		}
	}
	return strings.TrimSpace(blocks[0][1])
}
