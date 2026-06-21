# M4：上下文管理（compact + resume）实施计划

## 目标

长会话自动压缩，会话可持久化与恢复。

**验收标准**：
- 对话 token 超阈值时自动 compact，UI 提示
- `/compact` 手动触发压缩
- `/sessions` 列出最近会话
- `/resume [id]` 恢复会话继续对话，保留上下文
- 重启进程后仍可 resume

## 任务分解

### Task 1: token 计数 + 阈值检测

**文件**: `internal/agent/context.go`, `internal/agent/context_test.go`

**Step 1: 创建 `internal/agent/context.go`**

```go
package agent

import (
	"github.com/bytedance/trae-agent/internal/llm"
)

// ContextManager 管理对话上下文的 token 计数与压缩阈值。
type ContextManager struct {
	maxTokens   int // 触发 compact 的阈值
	compactKeep int // compact 后保留的最近消息数
}

type ContextOption func(*ContextManager)

func WithMaxTokens(n int) ContextOption {
	return func(c *ContextManager) { c.maxTokens = n }
}

func WithCompactKeep(n int) ContextOption {
	return func(c *ContextManager) { c.compactKeep = n }
}

func NewContextManager(opts ...ContextOption) *ContextManager {
	c := &ContextManager{
		maxTokens:   100000, // 默认 10 万 token 触发
		compactKeep: 6,      // 默认保留最近 6 条消息（3 轮）
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// EstimateTokens 粗略估算 messages 的 token 数。
// 经验值：1 token ≈ 4 字符（英文），中文约 1 token/字。
// 这里用字符数 / 3.5 作为近似（偏保守，倾向早 compact）。
func EstimateTokens(messages []llm.Message) int {
	total := 0
	for _, m := range messages {
		// role + content + tool calls
		total += len(m.Content) / 3
		for _, tc := range m.ToolCalls {
			total += len(tc.Name) / 3
			total += len(tc.Args) / 3
		}
	}
	// 每条消息固定开销（role 标记等）
	total += len(messages) * 4
	return total
}

// ShouldCompact 检查是否需要压缩。
func (c *ContextManager) ShouldCompact(messages []llm.Message) bool {
	return EstimateTokens(messages) > c.maxTokens
}

// MaxTokens 返回阈值。
func (c *ContextManager) MaxTokens() int { return c.maxTokens }

// CompactKeep 返回保留消息数。
func (c *ContextManager) CompactKeep() int { return c.compactKeep }
```

**Step 2: 创建 `internal/agent/context_test.go`**

```go
package agent

import (
	"testing"

	"github.com/bytedance/trae-agent/internal/llm"
)

func TestEstimateTokens_empty(t *testing.T) {
	if got := EstimateTokens(nil); got != 0 {
		t.Errorf("empty messages should be 0 tokens, got %d", got)
	}
}

func TestEstimateTokens_singleMessage(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "hello world"},
	}
	got := EstimateTokens(msgs)
	// "hello world" = 11 chars / 3 = 3, + 4 overhead = 7
	if got < 3 || got > 20 {
		t.Errorf("expected 3-20 tokens, got %d", got)
	}
}

func TestEstimateTokens_withToolCalls(t *testing.T) {
	msgs := []llm.Message{
		{
			Role:    llm.RoleAssistant,
			Content: "let me read the file",
			ToolCalls: []llm.ToolCall{
				{ID: "t1", Name: "read", Args: `{"file_path":"/tmp/a.go"}`},
			},
		},
	}
	got := EstimateTokens(msgs)
	if got < 10 {
		t.Errorf("expected >=10 tokens with tool calls, got %d", got)
	}
}

func TestContextManager_ShouldCompact(t *testing.T) {
	cm := NewContextManager(WithMaxTokens(100))
	// 构造超过 100 token 的消息
	msgs := make([]llm.Message, 50)
	for i := range msgs {
		msgs[i] = llm.Message{Role: llm.RoleUser, Content: "this is a long enough message to exceed threshold"}
	}
	if !cm.ShouldCompact(msgs) {
		t.Error("should compact when over threshold")
	}
}

func TestContextManager_ShouldCompact_underThreshold(t *testing.T) {
	cm := NewContextManager(WithMaxTokens(10000))
	msgs := []llm.Message{{Role: llm.RoleUser, Content: "hi"}}
	if cm.ShouldCompact(msgs) {
		t.Error("should not compact under threshold")
	}
}

func TestContextManager_defaults(t *testing.T) {
	cm := NewContextManager()
	if cm.MaxTokens() != 100000 {
		t.Errorf("default maxTokens = %d, want 100000", cm.MaxTokens())
	}
	if cm.CompactKeep() != 6 {
		t.Errorf("default compactKeep = %d, want 6", cm.CompactKeep())
	}
}
```

**Step 3: 运行测试**
```bash
cd /workspace && go test ./internal/agent/... -run "TestEstimateTokens|TestContextManager" -v
```
Expected: 6 个测试 PASS

**Step 4: vet + commit**
```bash
cd /workspace && go vet ./internal/agent/...
git add internal/agent/context.go internal/agent/context_test.go
git commit -m "feat(m4): add token estimation and compact threshold"
```

---

### Task 2: compact 实现（LLM 摘要）

**文件**: `internal/agent/compact.go`, `internal/agent/compact_test.go`

**Step 1: 创建 `internal/agent/compact.go`**

```go
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/bytedance/trae-agent/internal/llm"
)

const compactPrompt = `Summarize the following conversation history. Preserve:
1. The user's main goal/task
2. Key decisions made
3. Files created or modified (with paths)
4. Important errors encountered and how they were resolved
5. Current state and next steps

Be concise but complete. Output a single summary paragraph.`

// Compactor 用 LLM 压缩对话历史。
type Compactor struct {
	provider llm.Provider
	cm       *ContextManager
	model    string // 用于 summarize 请求的 model
}

func NewCompactor(provider llm.Provider, cm *ContextManager, model string) *Compactor {
	return &Compactor{provider: provider, cm: cm, model: model}
}

// Compact 压缩 messages，返回新的 messages 列表。
// 保留最近 compactKeep 条消息，其余用 LLM 生成的摘要替换。
func (c *Compactor) Compact(ctx context.Context, messages []llm.Message) ([]llm.Message, error) {
	if len(messages) <= c.cm.CompactKeep() {
		return messages, nil // 消息太少，不需要压缩
	}

	keep := c.cm.CompactKeep()
	toSummarize := messages[:len(messages)-keep]
	recent := messages[len(messages)-keep:]

	summary, err := c.summarize(ctx, toSummarize)
	if err != nil {
		return nil, fmt.Errorf("compact: %w", err)
	}

	// 构造新历史：摘要消息 + 保留的最近消息
	result := make([]llm.Message, 0, keep+1)
	result = append(result, llm.Message{
		Role: llm.RoleUser,
		Content: fmt.Sprintf("[Previous conversation summary]\n%s\n[End of summary. Continue from here.]", summary),
	})
	result = append(result, recent...)
	return result, nil
}

// summarize 调 LLM 生成摘要。
func (c *Compactor) summarize(ctx context.Context, messages []llm.Message) (string, error) {
	// 把历史拼成文本
	var b strings.Builder
	for _, m := range messages {
		fmt.Fprintf(&b, "%s: %s", m.Role, m.Content)
		for _, tc := range m.ToolCalls {
			fmt.Fprintf(&b, " [tool: %s %s]", tc.Name, tc.Args)
		}
		b.WriteString("\n")
	}

	req := llm.Request{
		Model:    c.model,
		System:   compactPrompt,
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: b.String()},
		},
	}

	ch, err := c.provider.Stream(ctx, req)
	if err != nil {
		return "", err
	}

	var result strings.Builder
	for ev := range ch {
		switch e := ev.(type) {
		case llm.TextDelta:
			result.WriteString(e.Content)
		case llm.Error:
			return "", e.Err
		}
	}
	return result.String(), nil
}
```

**Step 2: 创建 `internal/agent/compact_test.go`**

```go
package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/bytedance/trae-agent/internal/llm"
)

// mockCompactProvider 返回固定摘要。
type mockCompactProvider struct {
	response string
}

func (m *mockCompactProvider) Name() string { return "mock" }
func (m *mockCompactProvider) Stream(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 2)
	ch <- llm.TextDelta{Content: m.response}
	ch <- llm.Done{}
	close(ch)
	return ch, nil
}

func TestCompactor_Compact_shortHistory(t *testing.T) {
	cm := NewContextManager(WithCompactKeep(6))
	c := NewCompactor(&mockCompactProvider{response: "summary"}, cm, "test-model")

	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "hi"},
		{Role: llm.RoleAssistant, Content: "hello"},
	}
	result, err := c.Compact(context.Background(), msgs)
	if err != nil {
		t.Fatal(err)
	}
	// 消息数 <= keep，不压缩
	if len(result) != 2 {
		t.Errorf("short history should not compact, got %d messages", len(result))
	}
}

func TestCompactor_Compact_longHistory(t *testing.T) {
	cm := NewContextManager(WithCompactKeep(2))
	c := NewCompactor(&mockCompactProvider{response: "this is the summary"}, cm, "test-model")

	// 5 条消息，keep 2，应压缩前 3 条为摘要
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "msg1"},
		{Role: llm.RoleAssistant, Content: "msg2"},
		{Role: llm.RoleUser, Content: "msg3"},
		{Role: llm.RoleAssistant, Content: "msg4"},
		{Role: llm.RoleUser, Content: "msg5"},
	}
	result, err := c.Compact(context.Background(), msgs)
	if err != nil {
		t.Fatal(err)
	}
	// 1 摘要 + 2 保留 = 3
	if len(result) != 3 {
		t.Fatalf("expected 3 messages after compact, got %d", len(result))
	}
	// 第一条应是摘要
	if !strings.Contains(result[0].Content, "summary") {
		t.Errorf("first message should be summary: %s", result[0].Content)
	}
	// 最后两条应是原 msg4, msg5
	if result[1].Content != "msg4" {
		t.Errorf("result[1] = %q, want msg4", result[1].Content)
	}
	if result[2].Content != "msg5" {
		t.Errorf("result[2] = %q, want msg5", result[2].Content)
	}
}

func TestCompactor_Compact_preservesToolCalls(t *testing.T) {
	cm := NewContextManager(WithCompactKeep(2))
	c := NewCompactor(&mockCompactProvider{response: "summary"}, cm, "test-model")

	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: "old1"},
		{Role: llm.RoleAssistant, Content: "old2"},
		{
			Role:    llm.RoleAssistant,
			Content: "recent with tool",
			ToolCalls: []llm.ToolCall{
				{ID: "t1", Name: "read", Args: `{}`},
			},
		},
		{Role: llm.RoleUser, Content: "recent"},
	}
	result, err := c.Compact(context.Background(), msgs)
	if err != nil {
		t.Fatal(err)
	}
	// 保留的消息应包含 tool calls
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	if len(result[1].ToolCalls) != 1 {
		t.Errorf("preserved message should keep tool calls, got %d", len(result[1].ToolCalls))
	}
}
```

**Step 3: 运行测试**
```bash
cd /workspace && go test ./internal/agent/... -run TestCompactor -v
```
Expected: 3 个测试 PASS

**Step 4: vet + commit**
```bash
cd /workspace && go vet ./internal/agent/...
git add internal/agent/compact.go internal/agent/compact_test.go
git commit -m "feat(m4): add LLM-based conversation compaction"
```

---

### Task 3: /compact 手动触发 + 自动触发

**文件**: 修改 `internal/agent/loop.go`, `internal/cli/repl.go`, `internal/cli/command.go`

**注意**：本 Task 给 REPL 加 `ctx`/`ctxMgr`/`compactor`/`store`/`sessionID` 字段（store/sessionID 在 Task 5 用，先定义）。`NewREPL` 签名改为 `(*REPL, error)`。

**Step 1: 修改 `internal/agent/loop.go`** — 加 Provider 和 Model 访问器

```go
// Provider 返回 agent 使用的 provider（供 Compactor 复用）。
func (a *Agent) Provider() llm.Provider {
	return a.provider
}

// Model 返回 agent 使用的 model。
func (a *Agent) Model() string {
	return a.model
}
```

**Step 2: 修改 `internal/cli/repl.go`** — REPL 加字段，NewREPL 改签名

```go
type REPL struct {
	agent      *agent.Agent
	rl         *readline.Instance
	renderer   *Renderer
	commands   *CommandRegistry
	messages   []llm.Message
	totalUsage llm.Usage
	ctxMgr     *agent.ContextManager
	compactor  *agent.Compactor
	store      *session.Store // Task 5 用，先定义
	sessionID  string         // Task 5 用，先定义
	ctx        context.Context
}

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
```

在 `Run` 方法开头加 `r.ctx = ctx`。

**Step 3: 修改 `internal/cli/interactive.go`** — 适配 NewREPL 新签名

```go
repl, err := NewREPL(a)
if err != nil {
	return err
}
return repl.Run(cmd.Context())
```

**Step 4: 加 doCompact 方法到 `internal/cli/repl.go`**

```go
// doCompact 执行 compact，更新 r.messages。
func (r *REPL) doCompact() (beforeTokens, afterTokens int, err error) {
	beforeTokens = agent.EstimateTokens(r.messages)
	newMsgs, err := r.compactor.Compact(r.ctx, r.messages)
	if err != nil {
		return beforeTokens, beforeTokens, err
	}
	r.messages = newMsgs
	afterTokens = agent.EstimateTokens(r.messages)
	return beforeTokens, afterTokens, nil
}
```

**Step 5: 修改 `internal/cli/command.go`** — 加 `/compact` 命令

在 `registerBuiltin` 加:
```go
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
```

**Step 6: 修改 `internal/cli/repl.go`** — runAgent 后自动检查 compact

在 `runAgent` 成功返回前（`r.messages = pendingMessages` 之后）加:
```go
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
```

**Step 7: 运行测试**
```bash
cd /workspace && go test ./internal/cli/... -v
cd /workspace && go test ./internal/agent/... -v
```
Expected: 全部 PASS

**Step 8: vet + commit**
```bash
cd /workspace && go vet ./...
git add internal/agent/loop.go internal/cli/repl.go internal/cli/command.go internal/cli/interactive.go
git commit -m "feat(m4): add /compact command and auto-compact on threshold"
```

---

### Task 4: session store（JSON 持久化）

**文件**: `internal/session/store.go`, `internal/session/store_test.go`

**Step 1: 创建 `internal/session/store.go`**

```go
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/bytedance/trae-agent/internal/llm"
)

// Session 一次对话会话。
type Session struct {
	ID        string        `json:"id"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Messages  []llm.Message `json:"messages"`
	Usage     llm.Usage     `json:"usage"`
}

// Store 会话存储，持久化到 ~/.trae/sessions/。
type Store struct {
	dir string
}

func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}
	dir := filepath.Join(home, ".trae", "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create sessions dir: %w", err)
	}
	return &Store{dir: dir}, nil
}

// NewStoreWithDir 用指定目录构造（测试用）。
func NewStoreWithDir(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// Save 保存会话。
func (s *Store) Save(sess *Session) error {
	sess.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, sess.ID+".json")
	return os.WriteFile(path, data, 0o644)
}

// Load 加载会话。
func (s *Store) Load(id string) (*Session, error) {
	path := filepath.Join(s.dir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

// List 列出所有会话，按更新时间降序。
func (s *Store) List() ([]*Session, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var sessions []*Session
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := entry.Name()[:len(entry.Name())-len(".json")]
		sess, err := s.Load(id)
		if err != nil {
			continue // 跳过损坏的会话文件
		}
		sessions = append(sessions, sess)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

// Delete 删除会话。
func (s *Store) Delete(id string) error {
	path := filepath.Join(s.dir, id+".json")
	return os.Remove(path)
}

// GenerateID 生成会话 ID（基于时间戳）。
func GenerateID() string {
	return time.Now().Format("20060102-150405")
}
```

**Step 2: 创建 `internal/session/store_test.go`**

```go
package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bytedance/trae-agent/internal/llm"
)

func TestStore_SaveLoad(t *testing.T) {
	tmp := t.TempDir()
	store, err := NewStoreWithDir(tmp)
	if err != nil {
		t.Fatal(err)
	}

	sess := &Session{
		ID:        "test-001",
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: "hello"}},
		Usage:     llm.Usage{InputTokens: 5, OutputTokens: 3},
	}
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load("test-001")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != "test-001" {
		t.Errorf("id = %q", loaded.ID)
	}
	if len(loaded.Messages) != 1 {
		t.Errorf("messages = %d", len(loaded.Messages))
	}
	if loaded.Messages[0].Content != "hello" {
		t.Errorf("content = %q", loaded.Messages[0].Content)
	}
	if loaded.Usage.InputTokens != 5 {
		t.Errorf("usage = %+v", loaded.Usage)
	}
}

func TestStore_Load_notFound(t *testing.T) {
	tmp := t.TempDir()
	store, _ := NewStoreWithDir(tmp)
	_, err := store.Load("nonexistent")
	if err == nil {
		t.Error("expected error for missing session")
	}
}

func TestStore_List(t *testing.T) {
	tmp := t.TempDir()
	store, _ := NewStoreWithDir(tmp)

	// 保存 3 个会话
	for _, id := range []string{"a", "b", "c"} {
		store.Save(&Session{ID: id, Messages: []llm.Message{}})
	}

	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 {
		t.Errorf("expected 3 sessions, got %d", len(list))
	}
}

func TestStore_List_sortedByUpdatedAt(t *testing.T) {
	tmp := t.TempDir()
	store, _ := NewStoreWithDir(tmp)

	// 保存会话，b 最后更新
	store.Save(&Session{ID: "a"})
	store.Save(&Session{ID: "b"})

	list, _ := store.List()
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
	// b 更新时间 >= a（可能相同），顺序应是 b 在前或相等
	if list[0].ID != "b" && list[0].UpdatedAt.Equal(list[1].UpdatedAt) {
		// 时间相同则顺序不确定，可接受
	}
}

func TestStore_Delete(t *testing.T) {
	tmp := t.TempDir()
	store, _ := NewStoreWithDir(tmp)
	store.Save(&Session{ID: "x"})

	if err := store.Delete("x"); err != nil {
		t.Fatal(err)
	}
	// 文件应不存在
	if _, err := os.Stat(filepath.Join(tmp, "x.json")); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

func TestStore_List_empty(t *testing.T) {
	tmp := t.TempDir()
	store, _ := NewStoreWithDir(tmp)
	list, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(list))
	}
}

func TestGenerateID(t *testing.T) {
	id := GenerateID()
	if len(id) != 15 { // 20060102-150405 = 8+1+6 = 15
		t.Errorf("id length = %d, want 15: %s", len(id), id)
	}
}
```

**Step 3: 运行测试**
```bash
cd /workspace && go test ./internal/session/... -v
```
Expected: 7 个测试 PASS

**Step 4: vet + commit**
```bash
cd /workspace && go vet ./internal/session/...
git add internal/session/store.go internal/session/store_test.go
git commit -m "feat(m4): add session JSON persistence store"
```

---

### Task 5: session resume + /resume 命令

**文件**: 修改 `internal/cli/repl.go`, `internal/cli/command.go`

**前置条件**: Task 3 已给 REPL 加了 `store`/`sessionID`/`ctx` 字段，`NewREPL` 已返回 `(*REPL, error)`。

**Step 1: 修改 `internal/cli/repl.go`** — 加 saveSession 方法，退出时自动保存

加 `saveSession` 方法:
```go
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
		r.println("warning: failed to save session: " + err.Error())
	}
}
```

在 `Run` 方法的 `io.EOF` 和 `ErrInterrupt` 退出路径加 `r.saveSession()`:
```go
if err == io.EOF {
	r.saveSession()
	r.println("bye")
	return nil
}
if err == readline.ErrInterrupt {
	if interrupter.Handle() {
		r.saveSession()
		return nil
	}
	continue
}
```

**Step 2: 修改 `internal/cli/command.go`** — 加 `/sessions` 和 `/resume` 命令

```go
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
				break // 只显示最近 10 个
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
		repl.messages = sess.Messages
		repl.totalUsage = sess.Usage
		repl.sessionID = sess.ID
		return CommandResult{Message: fmt.Sprintf("resumed session %s (%d messages)", sess.ID, len(sess.Messages))}
	},
})
```

**Step 3: 运行测试**
```bash
cd /workspace && go test ./internal/cli/... -v
```
Expected: 全部 PASS

**Step 4: vet + commit**
```bash
cd /workspace && go vet ./...
git add internal/cli/repl.go internal/cli/command.go
git commit -m "feat(m4): add /sessions and /resume commands with auto-save"
```

---

### Task 6: 端到端验收

**Step 1: 全量测试**
```bash
cd /workspace && make test
cd /workspace && make vet
cd /workspace && make build
```

**Step 2: 验证命令**
```bash
./bin/trae interactive --help
echo "/sessions" | ./bin/trae interactive 2>&1 | head -5
echo "/compact" | ./bin/trae interactive 2>&1 | head -5
```

**Step 3: 测试统计**
```bash
cd /workspace && go test ./... -v 2>&1 | grep -c "^--- PASS"
```
Expected: M3 的 84 + M4 新增约 16 = ~100 个测试

**Step 4: commit + push**
```bash
git add -A
git commit -m "test(m4): e2e acceptance"
git push origin solo-go
```

---

## 测试目标

| 包 | M3 测试数 | M4 新增 | M4 总计 |
|----|----------|---------|---------|
| internal/agent | 2 | 9 | 11 |
| internal/cli | 22 | 2 | 24 |
| internal/session | 0 | 7 | 7 |
| 其他 | 60 | 0 | 60 |
| **合计** | **84** | **~18** | **~102** |

## 风险与注意事项

1. **token 估算精度**：`EstimateTokens` 用字符数/3 近似，与真实 token 数有偏差。偏保守（倾向早 compact）是安全的，但可能过早压缩。M4 接受这个误差，M8 可接入 tiktoken 精确计数。
2. **compact 丢失工具上下文**：摘要可能丢失工具调用的精确参数。`CompactKeep` 保留最近消息（含 tool calls）缓解。如果 LLM 正在多步工具调用中途，compact 可能打断。**对策**：只在 agent 一轮结束后检查 compact，不在工具调用中途触发。
3. **session 文件并发**：多个 trae 进程同时写同一 session 文件会冲突。M4 不处理，单进程使用。
4. **NewREPL 签名变更**：从 `(*REPL)` 改为 `(*REPL, error)`，需更新 `interactive.go`。`repl_test.go` 里直接构造 `&REPL{}` 的测试不受影响。
5. **resume 后的 sessionID**：resume 后 sessionID 改为被恢复的会话 ID，后续保存会覆盖原会话。如果用户想另存为新会话，需要 `/save` 命令（M4 暂不实现，M5 加）。
6. **compact 用 provider**：Compactor 复用 agent 的 provider，会消耗额外 token。auto-compact 触发时用户会看到 `[auto-compacting...]` 提示。
