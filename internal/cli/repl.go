package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/bytedance/trae-agent/internal/agent"
	"github.com/bytedance/trae-agent/internal/consts"
	"github.com/bytedance/trae-agent/internal/errors"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/mcp"
	"github.com/bytedance/trae-agent/internal/permission"
	"github.com/bytedance/trae-agent/internal/session"
	"github.com/chzyer/readline"
	"github.com/mattn/go-isatty"
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
	store      SessionStore
	sessionID  string
	ctx        context.Context
	policy     *permission.DefaultPolicy
	permStore  *permission.Store
	mcpMgr     *mcp.Manager
	askMu      sync.Mutex
	mu         sync.Mutex // 保护 planMode 等可变状态
	planMode   bool       // 计划模式：agent 只规划不执行
	// fallback 输入：readline 不兼容（如 Git Bash/mintty）时用 bufio.Scanner
	// 读取 stdin。out 为输出目标，scanner 为 nil 表示使用 readline。
	out     io.Writer
	scanner *bufio.Scanner
}

// NewREPL 构造 REPL 实例。
// policy/permStore/mcpMgr 可为 nil。构造后覆盖 headless asker 为交互式 asker。
func NewREPL(a *agent.Agent, policy *permission.DefaultPolicy, permStore *permission.Store, mcpMgr *mcp.Manager) (*REPL, error) {
	cm := agent.NewContextManager()
	store, err := session.NewStore()
	if err != nil {
		return nil, fmt.Errorf("init session store: %w", err)
	}
	r := &REPL{
		agent:     a,
		commands:  NewCommandRegistry(),
		renderer:  NewRenderer(),
		ctxMgr:    cm,
		compactor: agent.NewCompactor(a.Provider(), cm, a.Model()),
		store:     store,
		sessionID: session.GenerateID(),
		policy:    policy,
		permStore: permStore,
		mcpMgr:    mcpMgr,
	}
	a.SetAsker(r)
	return r, nil
}

// stdout 返回当前输出目标。优先 r.out，其次 readline.Stdout()，最后 os.Stdout。
// fallback(scanner)模式下 r.rl 为 nil，用 r.out（=os.Stdout）。
func (r *REPL) stdout() io.Writer {
	if r.out != nil {
		return r.out
	}
	if r.rl != nil {
		return r.rl.Stdout()
	}
	return os.Stdout
}

// readLine 统一读取一行输入。
// readline 模式用 r.rl.Readline()；fallback 模式用 r.scanner。
// 返回的 err 为 io.EOF 表示用户结束输入。
func (r *REPL) readLine() (string, error) {
	if r.scanner != nil {
		// fallback 模式：先打印提示符，再扫描一行
		fmt.Fprint(r.stdout(), "trae> ")
		if !r.scanner.Scan() {
			if err := r.scanner.Err(); err != nil {
				return "", err
			}
			return "", io.EOF
		}
		return r.scanner.Text(), nil
	}
	return r.rl.Readline()
}

// Ask 实现 permission.Asker 接口。
func (r *REPL) Ask(ctx context.Context, tool, args, reason string) permission.Action {
	r.askMu.Lock()
	defer r.askMu.Unlock()

	out := r.stdout()
	fmt.Fprintf(out, "\n\033[33m[permission]\033[0m tool=%s\n", tool)
	fmt.Fprintf(out, "  reason: %s\n", reason)
	if len(args) > consts.ArgsDisplayLimit {
		args = args[:consts.ArgsDisplayLimit] + "..."
	}
	fmt.Fprintf(out, "  args: %s\n", args)
	fmt.Fprintf(out, "  allow? [y]es / [n]o / [a]lways-yes / [d]always-no: ")

	// readline 模式：临时清空提示符读取确认；fallback 模式直接用 scanner 读一行。
	// 注意：fallback 模式不调用 readLine（会打印 "trae> "），因为上面已打印 "allow?" 提示。
	var line string
	if r.scanner == nil && r.rl != nil {
		oldPrompt := r.rl.Config.Prompt
		r.rl.SetPrompt("")
		rl, rerr := r.rl.Readline()
		r.rl.SetPrompt(oldPrompt)
		if rerr != nil {
			return permission.ActionDeny
		}
		line = rl
	} else if r.scanner != nil {
		if !r.scanner.Scan() {
			return permission.ActionDeny
		}
		line = r.scanner.Text()
	}

	switch strings.TrimSpace(strings.ToLower(line)) {
	case "y", "yes":
		return permission.ActionAllow
	case "a", "always":
		r.persistRule(tool, args, permission.ActionAllowAlways)
		return permission.ActionAllowAlways
	case "d", "deny":
		r.persistRule(tool, args, permission.ActionDenyAlways)
		return permission.ActionDenyAlways
	default:
		return permission.ActionDeny
	}
}

// persistRule 添加用户规则到内存策略并持久化。
func (r *REPL) persistRule(tool, args string, action permission.Action) {
	if r.policy == nil {
		return
	}
	keyArg := permission.ExtractKeyArg(tool, json.RawMessage(args))
	r.policy.AddUserRule(permission.Rule{
		Tool:   tool,
		Args:   []string{keyArg},
		Action: action,
		Desc:   fmt.Sprintf("user rule: %s %s", tool, keyArg),
	})
	if r.permStore != nil {
		if err := r.permStore.Save(r.policy); err != nil {
			r.println("warning: failed to persist permission rule: " + err.Error())
		}
	}
}

// Run 启动 REPL 主循环，阻塞直到用户退出。
func (r *REPL) Run(ctx context.Context) error {
	// realTTY 标记当前 os.Stdin 是否为真实终端。
	// 用于区分"真实终端下 readline 不兼容"（需 fallback）与
	// "测试 mock readline 立即 EOF"（应正常退出，不 fallback）。
	realTTY := isTerminal(os.Stdin.Fd())

	// 测试可注入 r.rl，跳过 readline 初始化与 TTY 检测
	if r.rl == nil && r.scanner == nil {
		// 非交互环境（管道/CI/重定向）下 readline 会立即收到 EOF，
		// 导致 "bye" 后静默退出，体验差。此处显式检测并给出引导。
		if !realTTY {
			return errors.New(errors.CodeInvalidArg,
				"interactive 模式需要终端(TTY)，但当前 stdin 不是终端。\n"+
					"可能原因：通过管道、重定向或 IDE 输出窗口运行。\n"+
					"非交互场景请使用: trae run \"<你的提问>\"")
		}
		rl, err := readline.NewEx(&readline.Config{
			Prompt:          "trae> ",
			AutoComplete:    r.commands.Completer(),
			InterruptPrompt: "^C",
			EOFPrompt:       "exit",
		})
		if err != nil {
			// readline 初始化失败（如 mintty 下），降级到 scanner
			slog.Warn("readline init failed, fallback to scanner", "err", err)
			r.initScannerFallback()
		} else {
			defer rl.Close()
			r.rl = rl
		}
	}
	r.ctx = ctx
	// 统一在退出时保存会话，覆盖所有退出路径（EOF / ErrInterrupt / /exit / readline error）
	defer r.saveSession()

	r.println(r.renderer.Welcome())

	interrupter := NewInterruptHandler()

	readCount := 0
	for {
		line, err := r.readLine()
		if err == io.EOF {
			// 首次读取即 EOF 且从未成功输入，且当前是真实终端：
			// 说明 readline 与终端不兼容（如 Git Bash/mintty），降级到 scanner。
			// 测试环境（realTTY=false）不触发 fallback，直接退出。
			if readCount == 0 && r.scanner == nil && r.rl != nil && realTTY {
				r.println("[已切换到简化输入模式，输入 / + Enter 查看命令]")
				r.initScannerFallback()
				continue
			}
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
		readCount++

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") {
			exit, agentInput := r.handleCommand(line)
			if exit {
				return nil
			}
			// /init 等命令返回 agentInput 时，触发 agent 执行
			if agentInput != "" {
				r.runAgent(ctx, agentInput, interrupter)
			}
			continue
		}

		r.runAgent(ctx, line, interrupter)
	}
}

// initScannerFallback 关闭 readline，切换到 bufio.Scanner 读取 stdin。
// 用于 readline 不兼容的终端（Git Bash/mintty）。
func (r *REPL) initScannerFallback() {
	if r.rl != nil {
		r.rl.Close()
		r.rl = nil
	}
	r.scanner = bufio.NewScanner(os.Stdin)
	// 增大 buffer，支持长输入
	r.scanner.Buffer(make([]byte, 0, consts.ScannerInitialBuf), consts.ScannerMaxBuf)
	r.out = os.Stdout
}

func (r *REPL) runAgent(ctx context.Context, userInput string, interrupter *InterruptHandler) {
	agentCtx, cancel := context.WithCancel(ctx)
	interrupter.SetCancel(cancel)
	stopListener := interrupter.StartAgentSignalListener()
	defer stopListener()
	defer interrupter.SetCancel(nil)
	defer cancel() // 避免 context 泄漏

	// plan 模式：在用户输入前追加 plan 指令
	r.mu.Lock()
	planOn := r.planMode
	r.mu.Unlock()
	if planOn {
		userInput = agent.PlanModePrefix + userInput
	}

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

	usage, renderErr := r.renderer.Render(events, r.stdout())
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
	fmt.Fprintln(r.stdout(), s)
}

// handleCommand 处理斜杠命令。
// 返回 exit 表示是否退出 REPL，agentInput 非空时触发主循环执行 agent。
func (r *REPL) handleCommand(line string) (exit bool, agentInput string) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return false, ""
	}

	// "/" 单独输入：显示所有命令（级联引导）
	if parts[0] == "/" {
		r.println(r.suggestCommands("/"))
		return false, ""
	}

	cmd, ok := r.commands.Get(parts[0])
	if !ok {
		// 命令未找到：显示前缀匹配 + 近似建议
		suggestion := r.suggestCommands(parts[0])
		if suggestion != "" {
			r.println(suggestion)
		} else {
			r.println(fmt.Sprintf("unknown command: %s (type /help for all commands)", parts[0]))
		}
		return false, ""
	}
	result := cmd.Handler(r, parts[1:])
	if result.Exit {
		return true, ""
	}
	if result.Message != "" {
		r.println(result.Message)
	}
	return false, result.AgentInput
}

// isTerminal 检测 fd 是否为交互式终端。
// 兼容原生终端与 Windows 下 Cygwin/MSYS 终端。
func isTerminal(fd uintptr) bool {
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
