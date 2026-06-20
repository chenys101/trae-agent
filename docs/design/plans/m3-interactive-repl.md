# M3：交互式 REPL 实施计划

## 目标

`trae interactive` 进入流式交互界面，支持斜杠命令、Ctrl+C 中断、命令补全、lipgloss 样式渲染。

**验收标准**：
- 流式输出逐 token 显示，tool call 卡片样式
- `Ctrl+C` 中断当前 LLM 调用，回到提示符；连续两次退出
- `/help` `/clear` `/model` `/status` `/exit` `/cost` 命令可用
- `/` 触发命令补全
- 多轮对话保留上下文

## 依赖引入

- `github.com/chzyer/readline` — 行编辑、历史、补全
- `github.com/charmbracelet/lipgloss` — 样式渲染

## 任务分解

### Task 1: readline 集成 + REPL 主循环

**文件**: `internal/cli/repl.go`, `internal/cli/repl_test.go`

**Step 1: 创建 `internal/cli/repl.go`**

注意：REPL 的 `runAgent` 需要 InterruptHandler，但 Task 1 先不集成中断（Task 4/6 再加）。Task 1 的 REPL 用简化版 `runAgent`，Task 6 重写时集成 interrupter。

```go
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
	messages   []llm.Message // 多轮对话历史
	cancel     context.CancelFunc // 当前 agent 运行的 cancel
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

	for {
		line, err := rl.Readline()
		if err == io.EOF {
			r.println("bye")
			return nil
		}
		if err == readline.ErrInterrupt {
			// Task 1 简化：ErrInterrupt 直接退出。Task 6 改为中断 agent 或二次退出
			return nil
		}
		if err != nil {
			return fmt.Errorf("readline: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 斜杠命令
		if strings.HasPrefix(line, "/") {
			if exit := r.handleCommand(ctx, line); exit {
				return nil
			}
			continue
		}

		// agent 对话
		r.runAgent(ctx, line)
	}
}

// runAgent 执行一轮 agent 对话。Task 1 简化版（无中断），Task 6 重写集成 InterruptHandler。
func (r *REPL) runAgent(ctx context.Context, userInput string) {
	agentCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	defer func() { r.cancel = nil }()

	r.messages = append(r.messages, llm.Message{
		Role:    llm.RoleUser,
		Content: userInput,
	})

	events := make(chan agent.Event, 64)
	errCh := make(chan error, 1)
	go func() {
		errCh <- r.agent.RunWithHistory(agentCtx, &r.messages, events)
	}()

	usage, renderErr := r.renderer.Render(agentCtx, events, r.rl.Stdout())
	if renderErr != nil {
		r.println("error: " + renderErr.Error())
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
	r.totalUsage.InputTokens += usage.InputTokens
	r.totalUsage.OutputTokens += usage.OutputTokens
}

func (r *REPL) println(s string) {
	fmt.Fprintln(r.rl.Stdout(), s)
}

func (r *REPL) handleCommand(ctx context.Context, line string) (exit bool) {
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
```

**Step 2: 创建 `internal/cli/repl_test.go`**

```go
package cli

import (
	"testing"

	"github.com/bytedance/trae-agent/internal/llm"
)

func TestREPL_handleCommand_unknown(t *testing.T) {
	r := &REPL{
		commands: NewCommandRegistry(),
	}
	// 不应 panic，应返回 false（不退出）
	exit := r.handleCommand(nil, "/nope")
	if exit {
		t.Error("unknown command should not exit")
	}
}

func TestREPL_handleCommand_exit(t *testing.T) {
	r := &REPL{
		commands: NewCommandRegistry(),
	}
	exit := r.handleCommand(nil, "/exit")
	if !exit {
		t.Error("/exit should return true")
	}
}

func TestREPL_messagesAppend(t *testing.T) {
	r := &REPL{
		commands:  NewCommandRegistry(),
		renderer:  NewRenderer(),
		messages:  []llm.Message{},
	}
	// 模拟消息追加（不实际运行 agent）
	r.messages = append(r.messages, llm.Message{Role: llm.RoleUser, Content: "hi"})
	if len(r.messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(r.messages))
	}
}
```

**Step 3: 运行测试**
```bash
cd /workspace && go test ./internal/cli/... -run TestREPL -v
```
Expected: 3 个测试 PASS（依赖 Task 2 的 CommandRegistry，先写 stub）

**Step 4: vet + commit**
```bash
cd /workspace && go vet ./internal/cli/...
git add internal/cli/repl.go internal/cli/repl_test.go
git commit -m "feat(m3): add REPL main loop with readline"
```

---

### Task 2: 斜杠命令框架 + 内置命令

**文件**: `internal/cli/command.go`, `internal/cli/command_test.go`

**Step 1: 创建 `internal/cli/command.go`**

```go
package cli

import (
	"fmt"
	"strings"

	"github.com/bytedance/trae-agent/internal/llm"
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
		Description: "Show or set model (usage: /model [name])",
		Usage:       "/model [model-name]",
		Handler: func(repl *REPL, args []string) CommandResult {
			if len(args) == 0 {
				return CommandResult{Message: "current model: (from config)"}
			}
			// M3 简化：只显示，不实际切换（切换需要重建 agent）
			return CommandResult{Message: fmt.Sprintf("model switch not yet supported in M3, requested: %s", args[0])}
		},
	})
}
```

**Step 2: 创建 `internal/cli/command_test.go`**

```go
package cli

import (
	"strings"
	"testing"

	"github.com/bytedance/trae-agent/internal/llm"
)

func TestCommandRegistry_Get(t *testing.T) {
	r := NewCommandRegistry()
	cmd, ok := r.Get("/help")
	if !ok {
		t.Fatal("/help not found")
	}
	if cmd.Name != "/help" {
		t.Errorf("name = %q", cmd.Name)
	}
}

func TestCommandRegistry_Get_notFound(t *testing.T) {
	r := NewCommandRegistry()
	_, ok := r.Get("/nope")
	if ok {
		t.Error("expected not found")
	}
}

func TestCommandRegistry_List_order(t *testing.T) {
	r := NewCommandRegistry()
	list := r.List()
	if len(list) < 5 {
		t.Errorf("expected >=5 commands, got %d", len(list))
	}
	// 第一个注册的应是 /help
	if list[0].Name != "/help" {
		t.Errorf("first command = %q, want /help", list[0].Name)
	}
}

func TestCommand_help(t *testing.T) {
	r := NewCommandRegistry()
	cmd, _ := r.Get("/help")
	repl := &REPL{commands: r}
	result := cmd.Handler(repl, nil)
	if !strings.Contains(result.Message, "/help") {
		t.Errorf("help output missing /help: %s", result.Message)
	}
	if !strings.Contains(result.Message, "/exit") {
		t.Errorf("help output missing /exit: %s", result.Message)
	}
}

func TestCommand_exit(t *testing.T) {
	r := NewCommandRegistry()
	cmd, _ := r.Get("/exit")
	result := cmd.Handler(&REPL{commands: r}, nil)
	if !result.Exit {
		t.Error("/exit should return Exit=true")
	}
}

func TestCommand_clear(t *testing.T) {
	r := NewCommandRegistry()
	cmd, _ := r.Get("/clear")
	repl := &REPL{
		commands: r,
		messages: []llm.Message{{Role: llm.RoleUser, Content: "x"}},
	}
	result := cmd.Handler(repl, nil)
	if len(repl.messages) != 0 {
		t.Error("/clear should empty messages")
	}
	if result.Message != "conversation cleared" {
		t.Errorf("message = %q", result.Message)
	}
}

func TestCommand_status(t *testing.T) {
	r := NewCommandRegistry()
	cmd, _ := r.Get("/status")
	repl := &REPL{commands: r}
	result := cmd.Handler(repl, nil)
	if !strings.Contains(result.Message, "messages:") {
		t.Errorf("status missing messages: %s", result.Message)
	}
}

func TestCommand_cost(t *testing.T) {
	r := NewCommandRegistry()
	cmd, _ := r.Get("/cost")
	repl := &REPL{commands: r}
	result := cmd.Handler(repl, nil)
	if !strings.Contains(result.Message, "total tokens") {
		t.Errorf("cost missing tokens: %s", result.Message)
	}
}

func TestCommandRegistry_completer(t *testing.T) {
	r := NewCommandRegistry()
	c := r.Completer()
	if c == nil {
		t.Fatal("completer is nil")
	}
	if len(c.Children) < 5 {
		t.Errorf("expected >=5 completer children, got %d", len(c.Children))
	}
}
```

**Step 3: 运行测试**
```bash
cd /workspace && go test ./internal/cli/... -run "TestCommand|TestCommandRegistry" -v
```
Expected: 7 个测试 PASS

**Step 4: vet + commit**
```bash
cd /workspace && go vet ./internal/cli/...
git add internal/cli/command.go internal/cli/command_test.go
git commit -m "feat(m3): add slash command framework with builtin commands"
```

---

### Task 3: 流式渲染 (lipgloss)

**文件**: `internal/cli/render.go`, `internal/cli/render_test.go`

**Step 1: 创建 `internal/cli/render.go`**

```go
package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/bytedance/trae-agent/internal/agent"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/charmbracelet/lipgloss"
)

// Renderer 流式渲染 agent 事件到终端。
type Renderer struct {
	// lipgloss 样式
	toolCallStyle lipgloss.Style
	resultStyle   lipgloss.Style
	errorStyle    lipgloss.Style
	infoStyle     lipgloss.Style
}

func NewRenderer() *Renderer {
	return &Renderer{
		toolCallStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Bold(true),
		resultStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		errorStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
		infoStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("36")),
	}
}

// Welcome 返回欢迎信息。
func (r *Renderer) Welcome() string {
	return r.infoStyle.Render("trae interactive — type /help for commands, Ctrl+C to interrupt, Ctrl+D to exit")
}

// Render 消费 agent 事件流，渲染到 out。返回 token 用量。
func (r *Renderer) Render(ctx context.Context, events <-chan agent.Event, out io.Writer) (llm.Usage, error) {
	var usage llm.Usage
	for ev := range events {
		switch e := ev.(type) {
		case agent.TextEvent:
			fmt.Fprint(out, e.Content)
		case agent.ToolCallEvent:
			fmt.Fprintln(out)
			fmt.Fprintln(out, r.toolCallStyle.Render(fmt.Sprintf("▶ %s %s", e.Name, e.Args)))
		case agent.ToolResultEvent:
			content := e.Result.Content
			if len(content) > 500 {
				content = content[:500] + "... (truncated)"
			}
			label := "✓"
			if e.Result.IsError {
				label = "✗"
			}
			fmt.Fprintln(out, r.resultStyle.Render(fmt.Sprintf("%s %s", label, content)))
		case agent.DoneEvent:
			usage = e.Usage
		}
	}
	fmt.Fprintln(out) // 结束后换行
	return usage, nil
}
```

**Step 2: 创建 `internal/cli/render_test.go`**

```go
package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/bytedance/trae-agent/internal/agent"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/tool"
)

func TestRenderer_textEvent(t *testing.T) {
	r := NewRenderer()
	events := make(chan agent.Event, 1)
	events <- agent.TextEvent{Content: "hello"}
	close(events)

	var buf bytes.Buffer
	usage, err := r.Render(context.Background(), events, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if buf.String() != "hello\n" {
		t.Errorf("output = %q, want 'hello\\n'", buf.String())
	}
	if usage.InputTokens != 0 {
		t.Errorf("usage should be zero: %+v", usage)
	}
}

func TestRenderer_toolCallEvent(t *testing.T) {
	r := NewRenderer()
	events := make(chan agent.Event, 2)
	events <- agent.ToolCallEvent{Name: "read", Args: `{"file_path":"/tmp/a"}`}
	events <- agent.DoneEvent{Usage: llm.Usage{InputTokens: 5, OutputTokens: 3}}
	close(events)

	var buf bytes.Buffer
	usage, _ := r.Render(context.Background(), events, &buf)
	out := buf.String()
	if !contains(out, "read") {
		t.Errorf("missing tool name: %s", out)
	}
	if !contains(out, "/tmp/a") {
		t.Errorf("missing tool args: %s", out)
	}
	if usage.InputTokens != 5 {
		t.Errorf("usage = %+v", usage)
	}
}

func TestRenderer_toolResultEvent(t *testing.T) {
	r := NewRenderer()
	events := make(chan agent.Event, 2)
	events <- agent.ToolResultEvent{
		Name:   "read",
		Result: tool.Result{Content: "file content here"},
	}
	close(events)

	var buf bytes.Buffer
	r.Render(context.Background(), events, &buf)
	out := buf.String()
	if !contains(out, "file content here") {
		t.Errorf("missing result content: %s", out)
	}
}

func TestRenderer_toolResultError(t *testing.T) {
	r := NewRenderer()
	events := make(chan agent.Event, 1)
	events <- agent.ToolResultEvent{
		Name:   "bash",
		Result: tool.Result{Content: "command failed", IsError: true},
	}
	close(events)

	var buf bytes.Buffer
	r.Render(context.Background(), events, &buf)
	out := buf.String()
	if !contains(out, "command failed") {
		t.Errorf("missing error content: %s", out)
	}
}

func TestRenderer_truncation(t *testing.T) {
	r := NewRenderer()
	longContent := make([]byte, 600)
	for i := range longContent {
		longContent[i] = 'x'
	}
	events := make(chan agent.Event, 1)
	events <- agent.ToolResultEvent{
		Name:   "read",
		Result: tool.Result{Content: string(longContent)},
	}
	close(events)

	var buf bytes.Buffer
	r.Render(context.Background(), events, &buf)
	out := buf.String()
	if !contains(out, "truncated") {
		t.Errorf("long content should be truncated: %s", out[:50])
	}
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
```

**Step 3: 运行测试**
```bash
cd /workspace && go test ./internal/cli/... -run TestRenderer -v
```
Expected: 5 个测试 PASS

**Step 4: vet + commit**
```bash
cd /workspace && go vet ./internal/cli/...
git add internal/cli/render.go internal/cli/render_test.go
git commit -m "feat(m3): add lipgloss-based stream renderer"
```

---

### Task 4: Ctrl+C 中断处理

**文件**: `internal/cli/interrupt.go`, `internal/cli/interrupt_test.go`

**Step 1: 创建 `internal/cli/interrupt.go`**

```go
package cli

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// InterruptHandler 管理 Ctrl+C 信号。
// 第一次 Ctrl+C 中断当前 agent 运行，第二次退出 REPL。
type InterruptHandler struct {
	mu       sync.Mutex
	cancel   context.CancelFunc // 当前 agent 的 cancel
	pressed  bool               // 上次是否已按过一次
}

func NewInterruptHandler() *InterruptHandler {
	return &InterruptHandler{}
}

// SetCancel 设置当前 agent 运行的 cancel 函数。
// agent 开始时调 SetCancel(cancel)，结束时调 SetCancel(nil)。
func (h *InterruptHandler) SetCancel(cancel context.CancelFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cancel = cancel
	h.pressed = false
}

// Handle 处理一次 Ctrl+C 信号。
// 返回 true 表示应退出 REPL，false 表示仅中断当前 agent。
func (h *InterruptHandler) Handle() bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.cancel != nil {
		// agent 正在运行，中断它
		h.cancel()
		h.cancel = nil
		h.pressed = true
		return false
	}

	// agent 未运行
	if h.pressed {
		// 第二次 Ctrl+C，退出
		return true
	}
	h.pressed = true
	return false
}

// Reset 重置 pressed 状态（用户开始输入新内容时调）。
func (h *InterruptHandler) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pressed = false
}

// Start 启动信号监听，返回停止函数。
func (h *InterruptHandler) Start(ctx context.Context) func() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-sigCh:
				if h.Handle() {
					// 退出信号：取消整个 ctx
					// 由调用方检查 ctx
					return
				}
			}
		}
	}()
	return func() {
		signal.Stop(sigCh)
	}
}
```

**Step 2: 创建 `internal/cli/interrupt_test.go`**

```go
package cli

import (
	"context"
	"testing"
)

func TestInterruptHandler_noAgent(t *testing.T) {
	h := NewInterruptHandler()
	// 无 agent 运行，第一次不退出
	if h.Handle() {
		t.Error("first press without agent should not exit")
	}
	// 第二次退出
	if !h.Handle() {
		t.Error("second press should exit")
	}
}

func TestInterruptHandler_withAgent(t *testing.T) {
	h := NewInterruptHandler()
	cancelled := false
	cancel := func() { cancelled = true }
	h.SetCancel(cancel)

	// 第一次：中断 agent，不退出
	if h.Handle() {
		t.Error("first press with agent should not exit")
	}
	if !cancelled {
		t.Error("agent should be cancelled")
	}

	// 第二次：agent 已停，退出
	if !h.Handle() {
		t.Error("second press should exit")
	}
}

func TestInterruptHandler_reset(t *testing.T) {
	h := NewInterruptHandler()
	h.Handle() // pressed = true
	h.Reset()
	// reset 后第一次不退出
	if h.Handle() {
		t.Error("after reset, first press should not exit")
	}
}

func TestInterruptHandler_setCancelClearsPressed(t *testing.T) {
	h := NewInterruptHandler()
	h.Handle() // pressed = true
	h.SetCancel(func() {})
	// SetCancel 应重置 pressed
	if h.Handle() {
		t.Error("after SetCancel, Handle should cancel agent not exit")
	}
}

func TestInterruptHandler_startStop(t *testing.T) {
	h := NewInterruptHandler()
	ctx, cancel := context.WithCancel(context.Background())
	stop := h.Start(ctx)
	cancel()
	stop()
	// 不 panic 即可
}
```

**Step 3: 运行测试**
```bash
cd /workspace && go test ./internal/cli/... -run TestInterruptHandler -v
```
Expected: 5 个测试 PASS

**Step 4: vet + commit**
```bash
cd /workspace && go vet ./internal/cli/...
git add internal/cli/interrupt.go internal/cli/interrupt_test.go
git commit -m "feat(m3): add Ctrl+C interrupt handler (once=cancel, twice=exit)"
```

---

### Task 5: 命令补全

**文件**: 修改 `internal/cli/command.go` 的 Completer（已在 Task 2 实现），补充文件路径补全。

**Step 1: 在 `internal/cli/command.go` 加文件路径补全**

Completer 已在 Task 2 用 `readline.PcItem` 注册了斜杠命令。这里补充：当用户输入非 `/` 开头时不补全（交给 readline 默认行为）。

实际上 readline 的 `PrefixCompleter` 只在输入匹配前缀时触发。斜杠命令补全已由 Task 2 覆盖。M3 不做文件路径补全（留给 M4+），因为 agent 自己用 glob/grep 工具找文件，用户不需要在 REPL 输入文件路径。

**此 Task 实际上已在 Task 2 完成，无需额外代码。**

**验证测试**（加到 command_test.go）:
```go
func TestCommandRegistry_completer(t *testing.T) {
	r := NewCommandRegistry()
	c := r.Completer()
	if c == nil {
		t.Fatal("completer is nil")
	}
	// PcItem 的 Children 应包含注册的命令
	if len(c.Children) < 5 {
		t.Errorf("expected >=5 completer children, got %d", len(c.Children))
	}
}
```

**Step 2: 运行测试**
```bash
cd /workspace && go test ./internal/cli/... -run TestCommandRegistry_completer -v
```
Expected: PASS

**Step 3: commit（如果加了测试）**
```bash
git add internal/cli/command_test.go
git commit -m "test(m3): verify command completer"
```

---

### Task 6: agent 扩展 RunWithHistory + interactive 子命令

**文件**: 修改 `internal/agent/loop.go`，创建 `internal/cli/interactive.go`

**Step 1: 在 `internal/agent/loop.go` 加 RunWithHistory 方法**

现有 `Run(ctx, userInput, events)` 每次从单条 user 消息开始。REPL 需要多轮对话，传入已有 messages 数组。

**关键**：用 `*[]llm.Message` 指针传递，避免 append 后调用方 slice 不更新。

```go
// RunWithHistory 用已有消息历史执行 agent 循环。
// messages 是指针，agent 会追加 assistant 和 tool 消息，调用方通过同一指针看到更新。
func (a *Agent) RunWithHistory(ctx context.Context, messages *[]llm.Message, events chan<- Event) error {
	defer close(events)

	for step := 0; step < a.maxSteps; step++ {
		req := llm.Request{
			Model:    a.model,
			System:   SystemPrompt,
			Messages: *messages,
			Tools:    a.tools,
		}

		ch, err := a.provider.Stream(ctx, req)
		if err != nil {
			return fmt.Errorf("step %d stream: %w", step, err)
		}

		var textBuf string
		var toolCalls []llm.ToolCall
		var usage llm.Usage

		for ev := range ch {
			switch e := ev.(type) {
			case llm.TextDelta:
				textBuf += e.Content
				select {
				case events <- TextEvent{Content: e.Content}:
				case <-ctx.Done():
					return ctx.Err()
				}
			case llm.ToolCallDelta:
				tc := llm.ToolCall{ID: e.ID, Name: e.Name, Args: e.ArgsDelta}
				toolCalls = append(toolCalls, tc)
				select {
				case events <- ToolCallEvent{Name: tc.Name, Args: tc.Args}:
				case <-ctx.Done():
					return ctx.Err()
				}
			case llm.Done:
				usage = e.Usage
			case llm.Error:
				return e.Err
			}
		}

		assistantMsg := llm.Message{Role: llm.RoleAssistant, Content: textBuf, ToolCalls: toolCalls}
		*messages = append(*messages, assistantMsg)

		if len(toolCalls) == 0 {
			select {
			case events <- DoneEvent{Usage: usage}:
			case <-ctx.Done():
			}
			return nil
		}

		calls := make([]tool.Call, len(toolCalls))
		for i, tc := range toolCalls {
			calls[i] = tool.Call{Name: tc.Name, Args: []byte(tc.Args)}
		}
		results := a.dispatcher.Dispatch(ctx, calls)
		for i, res := range results {
			select {
			case events <- ToolResultEvent{Name: res.Name, Result: res.Result}:
			case <-ctx.Done():
				return ctx.Err()
			}
			*messages = append(*messages, llm.Message{
				Role:       llm.RoleTool,
				Content:    res.Result.Content,
				ToolCallID: toolCalls[i].ID,
			})
		}
	}

	return fmt.Errorf("max steps (%d) exceeded", a.maxSteps)
}
```

重构 `Run` 调用 `RunWithHistory`：

```go
// Run 执行 agent 循环（单轮，无历史保留）。
func (a *Agent) Run(ctx context.Context, userInput string, events chan<- Event) error {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: userInput},
	}
	return a.RunWithHistory(ctx, &messages, events)
}
```

**Step 2: 创建 `internal/cli/interactive.go`**

```go
package cli

import (
	"fmt"
	"time"

	"github.com/bytedance/trae-agent/internal/agent"
	"github.com/bytedance/trae-agent/internal/config"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/tool"
	"github.com/spf13/cobra"
)

func NewInteractiveCmd() *cobra.Command {
	var providerFlag string
	var modelFlag string
	cmd := &cobra.Command{
		Use:   "interactive",
		Short: "Start interactive REPL session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.LoadOptions{})
			if err != nil {
				return err
			}

			providerName := providerFlag
			if providerName == "" {
				providerName = cfg.DefaultProvider
			}
			if providerName == "" {
				return fmt.Errorf("no provider specified: set default_provider in config or use --provider")
			}

			provCfg, ok := cfg.Providers[providerName]
			if !ok {
				return fmt.Errorf("provider %q not found in config", providerName)
			}

			providerType := provCfg.Provider
			if providerType == "" {
				providerType = providerName
			}
			llmProvider, err := llm.NewProvider(providerType, provCfg.APIKey, provCfg.BaseURL, provCfg.DefaultModel)
			if err != nil {
				return err
			}
			llmProvider = llm.NewRetryable(llmProvider, 3, 500*time.Millisecond)

			registry := tool.NewRegistry(
				tool.NewRead(),
				tool.NewWrite(),
				tool.NewEdit(),
				tool.NewGlob(),
				tool.NewGrep(),
				tool.NewBash(120*time.Second),
			)

			maxSteps := cfg.MaxSteps
			if maxSteps == 0 {
				maxSteps = 20
			}
			a := agent.New(llmProvider, registry, agent.WithMaxSteps(maxSteps), agent.WithModel(modelFlag))

			repl := NewREPL(a)
			return repl.Run(cmd.Context())
		},
	}
	cmd.Flags().StringVar(&providerFlag, "provider", "", "LLM provider (anthropic/openai)")
	cmd.Flags().StringVar(&modelFlag, "model", "", "model name override")
	return cmd
}
```

**Step 3: 修改 `internal/cli/repl.go` 集成 InterruptHandler**

替换 Task 1 的 `Run` 和 `runAgent` 方法，加入 InterruptHandler：

```go
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
	stopInterrupt := interrupter.Start(ctx)
	defer stopInterrupt()

	for {
		line, err := rl.Readline()
		if err == io.EOF {
			r.println("bye")
			return nil
		}
		if err == readline.ErrInterrupt {
			// readline 捕获了 SIGINT（等待输入时）
			if interrupter.Handle() {
				return nil // 二次 Ctrl+C 退出
			}
			continue // 一次 Ctrl+C，继续等待输入
		}
		if err != nil {
			return fmt.Errorf("readline: %w", err)
		}

		interrupter.Reset() // 用户输入了内容，重置中断计数

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") {
			if exit := r.handleCommand(ctx, line); exit {
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
	defer interrupter.SetCancel(nil)

	r.messages = append(r.messages, llm.Message{
		Role:    llm.RoleUser,
		Content: userInput,
	})

	events := make(chan agent.Event, 64)
	errCh := make(chan error, 1)
	go func() {
		errCh <- r.agent.RunWithHistory(agentCtx, &r.messages, events)
	}()

	usage, renderErr := r.renderer.Render(agentCtx, events, r.rl.Stdout())
	if renderErr != nil {
		r.println("error: " + renderErr.Error())
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
	r.totalUsage.InputTokens += usage.InputTokens
	r.totalUsage.OutputTokens += usage.OutputTokens
}
```

**注意**：readline 在 agent 运行时（非 Readline 阻塞）不会捕获 SIGINT，此时 InterruptHandler 的 signal.Notify goroutine 会收到信号并调 `cancel()` 中断 agent。readline 在等待输入时自己处理 SIGINT 返回 ErrInterrupt。这两种情况都由 InterruptHandler.Handle 统一决策。

**Step 4: 在 root.go 注册 interactive 命令**

修改 `internal/cli/root.go`，加：
```go
root.AddCommand(NewInteractiveCmd())
```

**Step 5: 运行测试**
```bash
cd /workspace && go test ./internal/cli/... -v
cd /workspace && go test ./internal/agent/... -v
```
Expected: 全部 PASS（M2 的 agent 测试因 Run 重构仍应通过）

**Step 6: vet + commit**
```bash
cd /workspace && go vet ./...
git add internal/agent/loop.go internal/cli/interactive.go internal/cli/repl.go internal/cli/root.go
git commit -m "feat(m3): add interactive command with multi-turn agent loop"
```

---

### Task 7: 端到端验收

**Step 1: 全量测试**
```bash
cd /workspace && make test
cd /workspace && make vet
cd /workspace && make build
```

**Step 2: 验证 interactive 命令**
```bash
./bin/trae interactive --help
# 应显示用法
```

**Step 3: 验证斜杠命令（mock）**
```bash
# 用 mock provider 测试 REPL 逻辑（需要真实 LLM 才能完整测，这里只验证命令分发）
echo "/help" | ./bin/trae interactive --provider anthropic 2>&1 | head -20
echo "/exit" | ./bin/trae interactive --provider anthropic 2>&1
```

**Step 4: 测试统计**
```bash
cd /workspace && go test ./... -v 2>&1 | grep -c "^--- PASS"
```
Expected: M2 的 64 + M3 新增约 20 = ~84 个测试

**Step 5: commit**
```bash
git add -A
git commit -m "test(m3): e2e acceptance"
git push origin solo-go
```

---

## 测试目标

| 包 | M2 测试数 | M3 新增 | M3 总计 |
|----|----------|---------|---------|
| internal/cli | 3 | ~15 | ~18 |
| internal/agent | 2 | 0 (重构) | 2 |
| internal/tool | 29 | 0 | 29 |
| internal/llm | 16 | 0 | 16 |
| 其他 | 14 | 0 | 14 |
| **合计** | **64** | **~15** | **~79** |

## 风险与注意事项

1. **readline + signal 冲突**：readline 内部捕获 SIGINT，与 InterruptHandler 的 signal.Notify 可能冲突。Task 6 的方案：readline 处理等待输入时的 SIGINT（返回 ErrInterrupt），InterruptHandler 处理 agent 运行时的 SIGINT（调 cancel）。两者通过 `InterruptHandler.Handle()` 统一决策。需实测验证不互相干扰。如果冲突，备选方案：去掉 InterruptHandler.Start 的 signal.Notify，只靠 readline 的 ErrInterrupt + runAgent 内的 ctx cancel。
2. **lipgloss 颜色检测**：非 TTY 环境（管道、测试 buffer）lipgloss 自动禁用颜色，测试中用 bytes.Buffer 验证文本内容即可。
3. **RunWithHistory 重构**：现有 `Run` 改为调用 `RunWithHistory`，确保 M2 的 `TestAgent_singleTextResponse` 和 `TestAgent_toolCallLoop` 仍 PASS。注意 `RunWithHistory` 用 `*[]llm.Message` 指针，append 后调用方通过指针看到更新。
4. **多轮对话上下文**：REPL 的 `r.messages` 在 `/clear` 时清空。agent 每轮通过 `&r.messages` 指针追加 assistant + tool 消息。`/clear` 直接置 `r.messages = nil` 即可。
5. **readline 在非 TTY 的行为**：CI 环境无 TTY，readline 可能行为异常。测试用 mock，不实际启动 readline。REPL 的 Run 方法难以单测，只测 handleCommand 和 runAgent 的逻辑分支。
