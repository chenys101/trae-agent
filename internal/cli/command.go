package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/bytedance/trae-agent/internal/cost"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/session"
	"github.com/chzyer/readline"
)

// CommandResult 命令执行结果。
type CommandResult struct {
	Message    string // 输出消息（空则不打印）
	Exit       bool   // 是否退出 REPL
	AgentInput string // 非空时触发主循环执行 agent（复用渲染流程）
}

// Command 斜杠命令定义。
type Command struct {
	Name        string
	Description string
	Usage       string
	Handler     func(r *REPL, args []string) CommandResult
}

// CommandRegistry 命令注册表。
type CommandRegistry struct {
	commands map[string]*Command
	order    []string // 保持注册顺序，用于 /help
}

func NewCommandRegistry() *CommandRegistry {
	r := &CommandRegistry{commands: make(map[string]*Command)}
	r.registerBuiltin()
	r.loadCustom()
	return r
}

// loadCustom 从 .trae/commands/ 加载自定义斜杠命令。
// 内置命令优先：同名自定义命令不会覆盖已注册的内置命令。
func (r *CommandRegistry) loadCustom() {
	workDir, err := os.Getwd()
	if err != nil {
		return
	}
	cmds := LoadCustomCommands(workDir)
	for _, cc := range cmds {
		cmd := cc.ToCommand()
		// 内置命令优先，同名自定义命令跳过
		if _, exists := r.commands[cmd.Name]; exists {
			continue
		}
		r.Register(cmd)
	}
}

// Register 注册命令。重复命令名会覆盖 map 中的旧条目，但不重复 append order，
// 保持 /help 列表不出现重复项。
func (r *CommandRegistry) Register(cmd *Command) {
	if _, exists := r.commands[cmd.Name]; !exists {
		r.order = append(r.order, cmd.Name)
	}
	r.commands[cmd.Name] = cmd
}

func (r *CommandRegistry) Get(name string) (*Command, bool) {
	c, ok := r.commands[name]
	return c, ok
}

func (r *CommandRegistry) List() []*Command {
	out := make([]*Command, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.commands[name])
	}
	return out
}

// Completer 返回 readline 补全器。
// 构造 []PrefixCompleterInterface 后一次性传入 NewPrefixCompleter，
// 避免直接操作返回值的内部 Children 字段。
func (r *CommandRegistry) Completer() *readline.PrefixCompleter {
	items := make([]readline.PrefixCompleterInterface, 0, len(r.order))
	for _, cmd := range r.List() {
		items = append(items, readline.PcItem(cmd.Name))
	}
	return readline.NewPrefixCompleter(items...)
}

func (r *CommandRegistry) registerBuiltin() {
	r.Register(&Command{
		Name:        "/help",
		Description: "Show available commands",
		Usage:       "/help",
		Handler: func(repl *REPL, args []string) CommandResult {
			var b strings.Builder
			b.WriteString("Available commands:\n")
			for _, cmd := range r.List() {
				fmt.Fprintf(&b, "  %-12s %s\n", cmd.Name, cmd.Description)
			}
			return CommandResult{Message: b.String()}
		},
	})
	r.Register(&Command{
		Name:        "/exit",
		Description: "Exit the REPL",
		Usage:       "/exit",
		Handler: func(repl *REPL, args []string) CommandResult {
			return CommandResult{Exit: true}
		},
	})
	r.Register(&Command{
		Name:        "/clear",
		Description: "Clear conversation history",
		Usage:       "/clear",
		Handler: func(repl *REPL, args []string) CommandResult {
			if len(repl.messages) > 0 {
				repl.saveSession()
			}
			repl.messages = nil
			repl.totalUsage = llm.Usage{}
			repl.sessionID = session.GenerateID()
			return CommandResult{Message: "conversation cleared (previous session saved)"}
		},
	})
	r.Register(&Command{
		Name:        "/status",
		Description: "Show session status (session, model, messages)",
		Usage:       "/status",
		Handler: func(repl *REPL, args []string) CommandResult {
			// /status 专注会话状态，token 用量由 /cost 提供
			model := "(none)"
			if repl.agent != nil {
				model = repl.agent.Model()
			}
			return CommandResult{Message: fmt.Sprintf(
				"session: %s\nmodel: %s\nmessages: %d",
				repl.sessionID, model, len(repl.messages),
			)}
		},
	})
	r.Register(&Command{
		Name:        "/cost",
		Description: "Show total token usage and estimated cost",
		Usage:       "/cost",
		Handler: func(repl *REPL, args []string) CommandResult {
			msg := fmt.Sprintf(
				"total tokens: input=%d output=%d",
				repl.totalUsage.InputTokens, repl.totalUsage.OutputTokens,
			)
			if repl.agent != nil {
				model := repl.agent.Model()
				if estCost, ok := cost.Estimate(model, repl.totalUsage); ok {
					msg += fmt.Sprintf(" | estimated cost: %s (model: %s)", cost.FormatCost(estCost), model)
				}
			}
			return CommandResult{Message: msg}
		},
	})
	r.Register(&Command{
		Name:        "/model",
		Description: "Show current model (switching not supported in M3)",
		Usage:       "/model",
		Handler: func(repl *REPL, args []string) CommandResult {
			if len(args) > 0 {
				return CommandResult{Message: "error: model switching not supported, use --model flag at startup"}
			}
			return CommandResult{Message: fmt.Sprintf("current model: %s", repl.agent.Model())}
		},
	})
	r.Register(&Command{
		Name:        "/compact",
		Description: "Manually compact conversation history",
		Usage:       "/compact",
		Handler: func(repl *REPL, args []string) CommandResult {
			if len(repl.messages) == 0 {
				return CommandResult{Message: "no messages to compact"}
			}
			before, after, err := repl.doCompact()
			if err != nil {
				return CommandResult{Message: fmt.Sprintf("compact failed: %v", err)}
			}
			return CommandResult{Message: fmt.Sprintf("compacted: %d → %d tokens (%d messages)", before, after, len(repl.messages))}
		},
	})
	r.Register(&Command{
		Name:        "/sessions",
		Description: "List recent sessions",
		Usage:       "/sessions",
		Handler: func(repl *REPL, args []string) CommandResult {
			if repl.store == nil {
				return CommandResult{Message: "session store unavailable"}
			}
			list, err := repl.store.List()
			if err != nil {
				return CommandResult{Message: "list sessions: " + err.Error()}
			}
			if len(list) == 0 {
				return CommandResult{Message: "no saved sessions"}
			}
			var b strings.Builder
			b.WriteString("Recent sessions:\n")
			for i, s := range list {
				if i >= 10 {
					break
				}
				fmt.Fprintf(&b, "  %s  %d msgs  %s\n",
					s.ID, len(s.Messages), s.UpdatedAt.Format("2006-01-02 15:04"))
			}
			return CommandResult{Message: b.String()}
		},
	})
	r.Register(&Command{
		Name:        "/resume",
		Description: "Resume a session (usage: /resume [session-id])",
		Usage:       "/resume [session-id]",
		Handler: func(repl *REPL, args []string) CommandResult {
			if repl.store == nil {
				return CommandResult{Message: "session store unavailable"}
			}
			if len(args) == 0 {
				return CommandResult{Message: "usage: /resume <session-id> (use /sessions to list)"}
			}
			sess, err := repl.store.Load(args[0])
			if err != nil {
				return CommandResult{Message: "load session: " + err.Error()}
			}
			// 保存当前会话（若有内容），避免覆盖丢失
			if len(repl.messages) > 0 {
				repl.saveSession()
			}
			repl.messages = sess.Messages
			repl.totalUsage = sess.Usage
			// 生成新 sessionID，避免后续保存覆盖原会话
			repl.sessionID = session.GenerateID()
			return CommandResult{Message: fmt.Sprintf("resumed session %s (%d messages, saved as new session %s)", sess.ID, len(sess.Messages), repl.sessionID)}
		},
	})
	r.Register(NewPermissionsCmd())
	r.Register(NewMCPCmd())
	r.Register(NewInitCmd())
	r.Register(NewPlanCmd())
}
