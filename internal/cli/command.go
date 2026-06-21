package cli

import (
	"fmt"
	"strings"

	"github.com/chzyer/readline"
)

// CommandResult 命令执行结果。
type CommandResult struct {
	Message string // 输出消息（空则不打印）
	Exit    bool   // 是否退出 REPL
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
	return r
}

func (r *CommandRegistry) Register(cmd *Command) {
	r.commands[cmd.Name] = cmd
	r.order = append(r.order, cmd.Name)
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
func (r *CommandRegistry) Completer() *readline.PrefixCompleter {
	pc := readline.NewPrefixCompleter()
	for _, cmd := range r.List() {
		pc.Children = append(pc.Children, readline.PcItem(cmd.Name))
	}
	return pc
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
			repl.messages = nil
			return CommandResult{Message: "conversation cleared"}
		},
	})
	r.Register(&Command{
		Name:        "/status",
		Description: "Show current status (provider, model, steps)",
		Usage:       "/status",
		Handler: func(repl *REPL, args []string) CommandResult {
			return CommandResult{Message: fmt.Sprintf(
				"messages: %d\nusage: in=%d out=%d",
				len(repl.messages), repl.totalUsage.InputTokens, repl.totalUsage.OutputTokens,
			)}
		},
	})
	r.Register(&Command{
		Name:        "/cost",
		Description: "Show total token usage",
		Usage:       "/cost",
		Handler: func(repl *REPL, args []string) CommandResult {
			return CommandResult{Message: fmt.Sprintf(
				"total tokens: input=%d output=%d",
				repl.totalUsage.InputTokens, repl.totalUsage.OutputTokens,
			)}
		},
	})
	r.Register(&Command{
		Name:        "/model",
		Description: "Show current model (switching not supported in M3)",
		Usage:       "/model",
		Handler: func(repl *REPL, args []string) CommandResult {
			if len(args) > 0 {
				return CommandResult{Message: "error: model switching not supported in M3, use --model flag at startup"}
			}
			return CommandResult{Message: "current model: (from config or --model flag)"}
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
}
