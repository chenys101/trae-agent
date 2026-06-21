package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/bytedance/trae-agent/internal/agent"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/chzyer/readline"
)

// REPL 交互式会话。
type REPL struct {
	agent      *agent.Agent
	rl         *readline.Instance
	renderer   *Renderer
	commands   *CommandRegistry
	messages   []llm.Message
	totalUsage llm.Usage
}

// NewREPL 构造 REPL 实例。
func NewREPL(a *agent.Agent) *REPL {
	return &REPL{
		agent:    a,
		commands: NewCommandRegistry(),
		renderer: NewRenderer(),
	}
}

// Run 启动 REPL 主循环，阻塞直到用户退出。
func (r *REPL) Run(ctx context.Context) error {
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "trae> ",
		AutoComplete:    r.commands.Completer(),
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		return fmt.Errorf("init readline: %w", err)
	}
	defer rl.Close()
	r.rl = rl

	r.println(r.renderer.Welcome())

	interrupter := NewInterruptHandler()

	for {
		line, err := rl.Readline()
		if err == io.EOF {
			r.println("bye")
			return nil
		}
		if err == readline.ErrInterrupt {
			// readline 捕获了 SIGINT（等待输入时）
			if interrupter.Handle() {
				return nil
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("readline: %w", err)
		}

		interrupter.Reset()

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") {
			if exit := r.handleCommand(line); exit {
				return nil
			}
			continue
		}

		r.runAgent(ctx, line, interrupter)
	}
}

func (r *REPL) runAgent(ctx context.Context, userInput string, interrupter *InterruptHandler) {
	agentCtx, cancel := context.WithCancel(ctx)
	interrupter.SetCancel(cancel)
	stopListener := interrupter.StartAgentSignalListener()
	defer stopListener()
	defer interrupter.SetCancel(nil)

	// 先暂存消息，agent 成功后才提交到 r.messages
	pendingMessages := make([]llm.Message, len(r.messages))
	copy(pendingMessages, r.messages)
	pendingMessages = append(pendingMessages, llm.Message{
		Role:    llm.RoleUser,
		Content: userInput,
	})

	events := make(chan agent.Event, 64)
	errCh := make(chan error, 1)
	go func() {
		errCh <- r.agent.RunWithHistory(agentCtx, &pendingMessages, events)
	}()

	usage, renderErr := r.renderer.Render(events, r.rl.Stdout())
	if renderErr != nil {
		r.println("error: " + renderErr.Error())
		// 等待 goroutine 结束，避免后续对 pendingMessages 的写竞争
		<-errCh
		return
	}
	if err := <-errCh; err != nil {
		if agentCtx.Err() != nil {
			r.println("\n[interrupted]")
		} else {
			r.println("error: " + err.Error())
		}
		return
	}
	// agent 成功完成，提交消息历史
	r.messages = pendingMessages
	r.totalUsage.InputTokens += usage.InputTokens
	r.totalUsage.OutputTokens += usage.OutputTokens
}

func (r *REPL) println(s string) {
	fmt.Fprintln(r.rl.Stdout(), s)
}

func (r *REPL) handleCommand(line string) (exit bool) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return false
	}
	cmd, ok := r.commands.Get(parts[0])
	if !ok {
		r.println(fmt.Sprintf("unknown command: %s (type /help)", parts[0]))
		return false
	}
	result := cmd.Handler(r, parts[1:])
	if result.Exit {
		return true
	}
	if result.Message != "" {
		r.println(result.Message)
	}
	return false
}
