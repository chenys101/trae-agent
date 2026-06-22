package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/bytedance/trae-agent/internal/agent"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/permission"
	"github.com/bytedance/trae-agent/internal/session"
	"github.com/chzyer/readline"
)

// SessionStore 会话存储接口（供测试 mock）。
type SessionStore interface {
	Save(sess *session.Session) error
	Load(id string) (*session.Session, error)
	List() ([]*session.Session, error)
}

// Compactor 上下文压缩接口（供测试 mock）。
type Compactor interface {
	Compact(ctx context.Context, messages []llm.Message) ([]llm.Message, error)
}

// REPL 交互式会话。
type REPL struct {
	agent      *agent.Agent
	rl         *readline.Instance
	renderer   *Renderer
	commands   *CommandRegistry
	messages   []llm.Message
	totalUsage llm.Usage
	ctxMgr     *agent.ContextManager
	compactor  Compactor
	store      SessionStore // Task 5 用，先定义
	sessionID  string       // Task 5 用，先定义
	ctx        context.Context
	policy     *permission.DefaultPolicy // M6 权限策略
	permStore  *permission.Store         // M6 权限规则持久化
}

// NewREPL 构造 REPL 实例。
func NewREPL(a *agent.Agent) (*REPL, error) {
	cm := agent.NewContextManager()
	store, err := session.NewStore()
	if err != nil {
		return nil, fmt.Errorf("init session store: %w", err)
	}
	return &REPL{
		agent:     a,
		commands:  NewCommandRegistry(),
		renderer:  NewRenderer(),
		ctxMgr:    cm,
		compactor: agent.NewCompactor(a.Provider(), cm, a.Model()),
		store:     store,
		sessionID: session.GenerateID(),
	}, nil
}

// Run 启动 REPL 主循环，阻塞直到用户退出。
func (r *REPL) Run(ctx context.Context) error {
	// 测试可注入 r.rl，跳过 readline 初始化
	if r.rl == nil {
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
	}
	r.ctx = ctx
	// 统一在退出时保存会话，覆盖所有退出路径（EOF / ErrInterrupt / /exit / readline error）
	defer r.saveSession()

	r.println(r.renderer.Welcome())

	interrupter := NewInterruptHandler()

	for {
		line, err := r.rl.Readline()
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
	defer cancel() // 避免 context 泄漏

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

	// 自动 compact 检查
	if r.ctxMgr.ShouldCompact(r.messages) {
		r.println("[auto-compacting...]")
		before, after, err := r.doCompact()
		if err != nil {
			r.println("auto-compact failed: " + err.Error())
		} else {
			r.println(fmt.Sprintf("[auto-compacted: %d → %d tokens]", before, after))
		}
	}
}

// doCompact 执行 compact，更新 r.messages。
func (r *REPL) doCompact() (beforeTokens, afterTokens int, err error) {
	ctx := r.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	beforeTokens = agent.EstimateTokens(r.messages)
	newMsgs, err := r.compactor.Compact(ctx, r.messages)
	if err != nil {
		return beforeTokens, beforeTokens, err
	}
	r.messages = newMsgs
	afterTokens = agent.EstimateTokens(r.messages)
	return beforeTokens, afterTokens, nil
}

// saveSession 保存当前会话到 store。
func (r *REPL) saveSession() {
	if r.store == nil || len(r.messages) == 0 {
		return
	}
	sess := &session.Session{
		ID:       r.sessionID,
		Messages: r.messages,
		Usage:    r.totalUsage,
	}
	if err := r.store.Save(sess); err != nil {
		if r.rl != nil {
			r.println("warning: failed to save session: " + err.Error())
		}
	}
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
