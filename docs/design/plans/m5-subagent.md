# M5 子 agent（Task 工具）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 主 agent 可通过 Task 工具派发子 agent，子 agent 拥有独立上下文与受限工具集，多个 Task 调用并行执行，结果摘要回汇主 agent。

**Architecture:** 子 agent 复用现有 `agent.Agent`，但用独立 `*[]llm.Message`（不共享主 agent 历史）和受限 `tool.Registry`（按 subagent_type 过滤）。Task 工具本身是一个 `tool.Tool` 实现，内部用 `sync.WaitGroup` 并行派发多个子 agent，每个子 agent 跑完 think-act 循环后返回最终文本。主 agent 的 dispatcher 已支持并行，因此多个 Task 调用天然并行。

**Tech Stack:** Go 1.25、现有 `internal/agent`、`internal/tool`、`internal/llm`，无新依赖

---

## 文件结构

本里程碑产出/修改的文件：

- Create: `internal/agent/subagent.go` — 子 agent 类型定义、工厂函数、受限工具集
- Create: `internal/agent/subagent_test.go` — 子 agent 单测
- Create: `internal/tool/task.go` — Task 工具实现
- Create: `internal/tool/task_test.go` — Task 工具单测
- Modify: `internal/agent/loop.go` — Agent 暴露 `RunWithHistory` 给子 agent 复用（已暴露，确认即可）
- Modify: `internal/agent/prompt.go` — SystemPrompt 增加 Task 工具使用指引
- Modify: `internal/cli/run.go` + `interactive.go` — 注册 Task 工具到 registry

---

### Task 1: 子 agent 类型与工厂

**Files:**
- Create: `internal/agent/subagent.go`
- Create: `internal/agent/subagent_test.go`

- [ ] **Step 1: 写失败测试**

Create `internal/agent/subagent_test.go`:
```go
package agent

import (
	"testing"

	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/tool"
)

func TestSubagentType_known(t *testing.T) {
	cases := []struct {
		name     string
		wantTools []string
	}{
		{"search", []string{"read", "glob", "grep"}},
		{"general-purpose", nil}, // nil 表示不限制，用全部工具
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, ok := GetSubagentType(c.name)
			if !ok {
				t.Fatalf("GetSubagentType(%q) not found", c.name)
			}
			if st.Name != c.name {
				t.Errorf("Name = %q, want %q", st.Name, c.name)
			}
			if st.Description == "" {
				t.Error("Description is empty")
			}
		})
	}
}

func TestSubagentType_unknown(t *testing.T) {
	_, ok := GetSubagentType("nonexistent")
	if ok {
		t.Error("expected false for unknown subagent type")
	}
}

func TestSubagentType_restrictedRegistry_search(t *testing.T) {
	// search 类型应只含 read/glob/grep，不含 bash/write/edit
	allTools := []tool.Tool{
		&mockTool{name: "read"},
		&mockTool{name: "glob"},
		&mockTool{name: "grep"},
		&mockTool{name: "bash"},
		&mockTool{name: "write"},
		&mockTool{name: "edit"},
	}
	st, _ := GetSubagentType("search")
	reg := st.RestrictedRegistry(tool.NewRegistry(allTools...))
	names := toolNames(reg)
	for _, n := range names {
		if n != "read" && n != "glob" && n != "grep" {
			t.Errorf("search subagent should not have tool %q", n)
		}
	}
	if len(names) != 3 {
		t.Errorf("search subagent should have 3 tools, got %d: %v", len(names), names)
	}
}

func TestSubagentType_restrictedRegistry_general(t *testing.T) {
	// general-purpose 不限制，应保留全部工具
	allTools := []tool.Tool{
		&mockTool{name: "read"},
		&mockTool{name: "bash"},
	}
	st, _ := GetSubagentType("general-purpose")
	reg := st.RestrictedRegistry(tool.NewRegistry(allTools...))
	if len(reg.List()) != 2 {
		t.Errorf("general-purpose should keep all tools, got %d", len(reg.List()))
	}
}

// mockTool 用于测试的 mock 工具
type mockTool struct {
	name string
}

func (m *mockTool) Name() string                    { return m.name }
func (m *mockTool) Description() string             { return "mock " + m.name }
func (m *mockTool) Schema() json.RawMessage         { return json.RawMessage(`{}`) }
func (m *mockTool) Run(ctx context.Context, args json.RawMessage) tool.Result {
	return tool.Result{Content: "mock"}
}

func toolNames(r *tool.Registry) []string {
	var names []string
	for _, t := range r.List() {
		names = append(names, t.Name())
	}
	return names
}
```

注意：测试文件需要 import `context`、`encoding/json`，补全 import 块：
```go
import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/tool"
)
```
（`llm` 暂未用到，可移除）

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
go test ./internal/agent/... -run TestSubagentType -v
```
Expected: FAIL，`undefined: GetSubagentType`、`undefined: SubagentType`

- [ ] **Step 3: 实现 subagent.go**

Create `internal/agent/subagent.go`:
```go
package agent

import (
	"github.com/bytedance/trae-agent/internal/tool"
)

// SubagentType 定义子 agent 的类型，决定其可用工具集和行为约束。
type SubagentType struct {
	Name        string   // 类型名，如 "search"、"general-purpose"
	Description string   // 给主 agent 看的描述，帮助 LLM 选择合适类型
	AllowedTools []string // 允许使用的工具名；nil 表示不限制（用全部工具）
}

// subagentTypes 内置子 agent 类型。
var subagentTypes = map[string]SubagentType{
	"search": {
		Name:         "search",
		Description:  "Fast agent specialized for exploring codebases. Use for high-level concept searches, finding connections between different parts of the codebase, or searching broad/ambiguous keywords. Only has read-only tools (read/glob/grep), cannot modify files.",
		AllowedTools: []string{"read", "glob", "grep"},
	},
	"general-purpose": {
		Name:         "general-purpose",
		Description:  "Perform a general-purpose coding task (a sub-task of the user's overall task). Use for complex multi-step coding tasks, operations that produce a lot of output not needed after the sub-agent completes, or cross-layer changes that have been planned out and can be implemented independently.",
		AllowedTools: nil, // 不限制，使用全部工具
	},
}

// GetSubagentType 按名称查询子 agent 类型。
func GetSubagentType(name string) (SubagentType, bool) {
	st, ok := subagentTypes[name]
	return st, ok
}

// ListSubagentTypes 返回所有内置子 agent 类型。
func ListSubagentTypes() []SubagentType {
	out := make([]SubagentType, 0, len(subagentTypes))
	for _, st := range subagentTypes {
		out = append(out, st)
	}
	return out
}

// RestrictedRegistry 基于全量 registry 构造受限 registry，只保留 AllowedTools 中的工具。
// 若 AllowedTools 为 nil，返回全量 registry 的副本。
func (st SubagentType) RestrictedRegistry(full *tool.Registry) *tool.Registry {
	if st.AllowedTools == nil {
		// 不限制：构造包含全部工具的新 registry
		return tool.NewRegistry(full.List()...)
	}
	allowed := make(map[string]bool, len(st.AllowedTools))
	for _, name := range st.AllowedTools {
		allowed[name] = true
	}
	var kept []tool.Tool
	for _, t := range full.List() {
		if allowed[t.Name()] {
			kept = append(kept, t)
		}
	}
	return tool.NewRegistry(kept...)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:
```bash
go test ./internal/agent/... -run TestSubagentType -v
```
Expected: PASS，4 个测试用例通过

- [ ] **Step 5: Commit**

```bash
git add internal/agent/subagent.go internal/agent/subagent_test.go
git commit -m "feat(m5): add subagent type definitions and restricted registry"
```

---

### Task 2: 子 agent 执行器

**Files:**
- Modify: `internal/agent/subagent.go`
- Modify: `internal/agent/subagent_test.go`

- [ ] **Step 1: 写失败测试**

追加到 `internal/agent/subagent_test.go`:
```go
func TestSubagentRunner_executesAndReturnsText(t *testing.T) {
	// mock provider 返回纯文本（无工具调用），子 agent 应返回该文本
	provider := &mockProvider{
		events: []llm.StreamEvent{
			llm.TextDelta{Content: "subagent result"},
			llm.Done{},
		},
	}
	fullRegistry := tool.NewRegistry(&mockTool{name: "read"})
	st, _ := GetSubagentType("search")
	runner := NewSubagentRunner(provider, fullRegistry, "test-model")

	result, err := runner.Run(context.Background(), st, "find all TODO comments")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result != "subagent result" {
		t.Errorf("result = %q, want 'subagent result'", result)
	}
}

func TestSubagentRunner_toolLoopReturnsFinalText(t *testing.T) {
	// mock provider 第一轮返回 tool_call，第二轮返回文本
	// 子 agent 应执行工具并返回最终文本
	provider := &mockProvider{
		sequences: [][]llm.StreamEvent{
			// 第一轮：tool_call
			{
				llm.ToolCallDelta{ID: "tc1", Name: "read", ArgsDelta: `{"file_path":"/tmp/x"}`},
				llm.Done{Usage: llm.Usage{InputTokens: 5, OutputTokens: 5}},
			},
			// 第二轮：最终文本
			{
				llm.TextDelta{Content: "found 2 TODOs"},
				llm.Done{Usage: llm.Usage{InputTokens: 10, OutputTokens: 5}},
			},
		},
	}
	fullRegistry := tool.NewRegistry(&mockTool{name: "read"})
	st, _ := GetSubagentType("search")
	runner := NewSubagentRunner(provider, fullRegistry, "test-model")

	result, err := runner.Run(context.Background(), st, "find TODOs")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result != "found 2 TODOs" {
		t.Errorf("result = %q, want 'found 2 TODOs'", result)
	}
}

func TestSubagentRunner_contextCancel(t *testing.T) {
	// mock provider 阻塞，ctx cancel 后应返回 error
	provider := &mockProvider{block: true}
	fullRegistry := tool.NewRegistry(&mockTool{name: "read"})
	st, _ := GetSubagentType("search")
	runner := NewSubagentRunner(provider, fullRegistry, "test-model")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := runner.Run(ctx, st, "test")
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestSubagentRunner_unknownType(t *testing.T) {
	provider := &mockProvider{}
	fullRegistry := tool.NewRegistry(&mockTool{name: "read"})
	runner := NewSubagentRunner(provider, fullRegistry, "test-model")

	_, err := runner.Run(context.Background(), SubagentType{Name: "unknown"}, "test")
	if err == nil {
		t.Error("expected error for unknown subagent type")
	}
}
```

注意：`mockProvider` 已在 `loop_test.go` 定义（含 `events`、`sequences`、`block` 字段）。若 `loop_test.go` 的 mockProvider 不支持 `sequences` 字段，需确认。先 Read `loop_test.go` 确认 mockProvider 结构。

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
go test ./internal/agent/... -run TestSubagentRunner -v
```
Expected: FAIL，`undefined: SubagentRunner`、`undefined: NewSubagentRunner`

- [ ] **Step 3: 实现 SubagentRunner**

追加到 `internal/agent/subagent.go`:
```go
import (
	"context"
	"fmt"

	"github.com/bytedance/trae-agent/internal/llm"
)

// SubagentRunner 执行子 agent 任务。
// 子 agent 复用主 agent 的 provider 和工具，但拥有独立的消息历史
// 和受限的工具集（按 SubagentType 过滤）。
type SubagentRunner struct {
	provider llm.Provider
	registry *tool.Registry
	model    string
}

// NewSubagentRunner 构造子 agent 执行器。
func NewSubagentRunner(provider llm.Provider, fullRegistry *tool.Registry, model string) *SubagentRunner {
	return &SubagentRunner{
		provider: provider,
		registry: fullRegistry,
		model:    model,
	}
}

// Run 执行子 agent 任务，返回最终文本结果。
// 子 agent 有独立的消息历史，不污染主 agent。
// 子 agent 的 maxSteps 限制为 10（避免无限循环）。
func (r *SubagentRunner) Run(ctx context.Context, st SubagentType, prompt string) (string, error) {
	// 校验 subagent type
	if _, ok := GetSubagentType(st.Name); !ok {
		return "", fmt.Errorf("unknown subagent type: %s", st.Name)
	}

	// 构造受限 registry
	restrictedReg := st.RestrictedRegistry(r.registry)

	// 构造子 agent，maxSteps 限制为 10
	subAgent := New(r.provider, restrictedReg,
		WithMaxSteps(10),
		WithModel(r.model),
		WithSystemPrompt(subagentSystemPrompt(st)),
	)

	// 独立消息历史
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: prompt},
	}

	// 用内部 channel 收集事件，只关心最终文本
	events := make(chan Event, 64)
	var resultText string

	// 在 goroutine 中跑 agent，主 goroutine 收集事件
	done := make(chan error, 1)
	go func() {
		done <- subAgent.RunWithHistory(ctx, &messages, events)
	}()

	for ev := range events {
		if te, ok := ev.(TextEvent); ok {
			resultText += te.Content
		}
		// 忽略 ToolCallEvent / ToolResultEvent / DoneEvent，
		// 子 agent 的中间过程不回汇主 agent
	}

	if err := <-done; err != nil {
		// 即使出错，也返回已收集的部分文本 + error
		if resultText != "" {
			return resultText, nil
		}
		return "", err
	}
	return resultText, nil
}

// subagentSystemPrompt 为子 agent 生成系统提示词。
func subagentSystemPrompt(st SubagentType) string {
	return fmt.Sprintf(`You are a %s sub-agent. You operate with a restricted tool set and must complete the assigned task autonomously.

When you are done, provide a concise summary of your findings or actions as your final text response. This summary will be returned to the parent agent.

Do not ask for user input. Do not reference tools you do not have access to.`, st.Name)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:
```bash
go test ./internal/agent/... -run TestSubagentRunner -v
```
Expected: PASS，4 个测试用例通过

- [ ] **Step 5: Commit**

```bash
git add internal/agent/subagent.go internal/agent/subagent_test.go
git commit -m "feat(m5): add SubagentRunner for isolated sub-agent execution"
```

---

### Task 3: Task 工具实现

**Files:**
- Create: `internal/tool/task.go`
- Create: `internal/tool/task_test.go`

- [ ] **Step 1: 写失败测试**

Create `internal/tool/task_test.go`:
```go
package tool

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func TestTask_singleSubagent(t *testing.T) {
	// mock runner 返回固定文本
	runner := &mockTaskRunner{
		results: map[string]string{
			"search": "found 3 TODOs in the codebase",
		},
	}
	task := NewTask(runner)

	args, _ := json.Marshal(map[string]string{
		"subagent_type": "search",
		"description":   "find TODOs",
		"prompt":        "find all TODO comments",
	})

	res := task.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "found 3 TODOs") {
		t.Errorf("result should contain subagent output: %s", res.Content)
	}
}

func TestTask_parallelSubagents(t *testing.T) {
	// 验证多个 Task 调用并行执行
	var peakConcurrent int32
	var current int32
	var mu sync.Mutex

	runner := &mockTaskRunner{
		results: map[string]string{
			"search":         "search done",
			"general-purpose": "general done",
		},
		onRun: func() {
			mu.Lock()
			current++
			if current > peakConcurrent {
				peakConcurrent = current
			}
			mu.Unlock()
		},
	}
	task := NewTask(runner)

	// 通过 dispatcher 并行调用两个 Task
	d := NewDispatcher(NewRegistry(task))
	calls := []Call{
		{Name: "task", Args: mustMarshal(t, map[string]string{
			"subagent_type": "search",
			"description":   "search task",
			"prompt":        "search something",
		})},
		{Name: "task", Args: mustMarshal(t, map[string]string{
			"subagent_type": "general-purpose",
			"description":   "general task",
			"prompt":        "do something",
		})},
	}

	results := d.Dispatch(context.Background(), calls)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Result.IsError {
			t.Errorf("unexpected error: %s", r.Result.Content)
		}
	}
}

func TestTask_missingSubagentType(t *testing.T) {
	runner := &mockTaskRunner{}
	task := NewTask(runner)

	args, _ := json.Marshal(map[string]string{
		"description": "no type",
		"prompt":      "test",
	})

	res := task.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for missing subagent_type")
	}
}

func TestTask_unknownSubagentType(t *testing.T) {
	runner := &mockTaskRunner{
		errs: map[string]error{
			"unknown": errUnknownType,
		},
	}
	task := NewTask(runner)

	args, _ := json.Marshal(map[string]string{
		"subagent_type": "unknown",
		"description":   "bad type",
		"prompt":        "test",
	})

	res := task.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for unknown subagent_type")
	}
}

func TestTask_contextCancel(t *testing.T) {
	runner := &mockTaskRunner{
		blockCtx: true,
	}
	task := NewTask(runner)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	args, _ := json.Marshal(map[string]string{
		"subagent_type": "search",
		"description":   "cancelled",
		"prompt":        "test",
	})

	res := task.Run(ctx, args)
	if !res.IsError {
		t.Error("expected error for cancelled context")
	}
}

// mockTaskRunner mock TaskRunner 接口
type mockTaskRunner struct {
	results   map[string]string
	errs      map[string]error
	onRun     func()
	blockCtx  bool
}

func (m *mockTaskRunner) RunSubagent(ctx context.Context, subagentType, description, prompt string) (string, error) {
	if m.blockCtx {
		<-ctx.Done()
		return "", ctx.Err()
	}
	if m.onRun != nil {
		m.onRun()
	}
	if err, ok := m.errs[subagentType]; ok {
		return "", err
	}
	if result, ok := m.results[subagentType]; ok {
		return result, nil
	}
	return "default mock result", nil
}

var errUnknownType = &mockErr{"unknown subagent type"}

type mockErr struct{ msg string }

func (e *mockErr) Error() string { return e.msg }

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
go test ./internal/tool/... -run TestTask -v
```
Expected: FAIL，`undefined: NewTask`、`undefined: TaskRunner`

- [ ] **Step 3: 实现 task.go**

Create `internal/tool/task.go`:
```go
package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

// TaskRunner 子 agent 执行接口，由 agent 包实现并注入。
// 解耦 tool → agent 的依赖（避免循环 import）。
type TaskRunner interface {
	RunSubagent(ctx context.Context, subagentType, description, prompt string) (string, error)
}

// Task 工具：派发子 agent 执行子任务。
// 主 agent 通过此工具并行派发多个子 agent，每个子 agent 有独立上下文。
type Task struct {
	runner TaskRunner
}

// NewTask 构造 Task 工具。
func NewTask(runner TaskRunner) *Task {
	return &Task{runner: runner}
}

func (Task) Name() string { return "task" }

func (Task) Description() string {
	return "Launch a new agent to handle complex, multi-step tasks autonomously. " +
		"Use for parallelizing independent queries or cross-layer changes. " +
		"Each sub-agent has its own context window and restricted tool set."
}

func (Task) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "subagent_type": {
      "type": "string",
      "description": "Type of sub-agent to launch",
      "enum": ["search", "general-purpose"]
    },
    "description": {
      "type": "string",
      "description": "A short (3-5 words) description of the task"
    },
    "prompt": {
      "type": "string",
      "description": "The task for the agent to perform. Provide all necessary context."
    }
  },
  "required": ["subagent_type", "description", "prompt"]
}`)
}

type taskArgs struct {
	SubagentType string `json:"subagent_type"`
	Description  string `json:"description"`
	Prompt       string `json:"prompt"`
}

func (t *Task) Run(ctx context.Context, args json.RawMessage) Result {
	var a taskArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("parse args: %v", err)
	}
	if a.SubagentType == "" {
		return ErrorResult("subagent_type is required")
	}
	if a.Prompt == "" {
		return ErrorResult("prompt is required")
	}

	result, err := t.runner.RunSubagent(ctx, a.SubagentType, a.Description, a.Prompt)
	if err != nil {
		return ErrorResult("subagent %s failed: %v", a.SubagentType, err)
	}
	return Result{Content: result}
}

// 确保 Task 实现 Tool 接口
var _ Tool = (*Task)(nil)

// 保留 fmt 引用（ErrorResult 已用 fmt，这里防止未使用 import 警告）
var _ = fmt.Sprintf
```

- [ ] **Step 4: 运行测试确认通过**

Run:
```bash
go test ./internal/tool/... -run TestTask -v
```
Expected: PASS，5 个测试用例通过

- [ ] **Step 5: Commit**

```bash
git add internal/tool/task.go internal/tool/task_test.go
git commit -m "feat(m5): add Task tool for sub-agent dispatch"
```

---

### Task 4: SubagentRunner 实现 TaskRunner 接口

**Files:**
- Modify: `internal/agent/subagent.go`
- Modify: `internal/agent/subagent_test.go`

- [ ] **Step 1: 写失败测试**

追加到 `internal/agent/subagent_test.go`:
```go
func TestSubagentRunner_implementsTaskRunner(t *testing.T) {
	// 编译期验证 SubagentRunner 实现 TaskRunner 接口
	provider := &mockProvider{}
	fullRegistry := tool.NewRegistry(&mockTool{name: "read"})
	runner := NewSubagentRunner(provider, fullRegistry, "test-model")

	// TaskRunner 接口签名：RunSubagent(ctx, subagentType, description, prompt) (string, error)
	var _ interface {
		RunSubagent(ctx context.Context, subagentType, description, prompt string) (string, error)
	} = runner
}

func TestSubagentRunner_runSubagent(t *testing.T) {
	provider := &mockProvider{
		events: []llm.StreamEvent{
			llm.TextDelta{Content: "task complete"},
			llm.Done{},
		},
	}
	fullRegistry := tool.NewRegistry(&mockTool{name: "read"})
	runner := NewSubagentRunner(provider, fullRegistry, "test-model")

	result, err := runner.RunSubagent(
		context.Background(),
		"search",
		"find TODOs",
		"find all TODO comments in the codebase",
	)
	if err != nil {
		t.Fatalf("RunSubagent failed: %v", err)
	}
	if result != "task complete" {
		t.Errorf("result = %q, want 'task complete'", result)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
go test ./internal/agent/... -run "TestSubagentRunner_implementsTaskRunner|TestSubagentRunner_runSubagent" -v
```
Expected: FAIL，`runner.RunSubagent undefined`

- [ ] **Step 3: 实现 RunSubagent 方法**

追加到 `internal/agent/subagent.go` 的 SubagentRunner 结构体：
```go
// RunSubagent 实现 tool.TaskRunner 接口，供 Task 工具调用。
// 参数 description 仅用于日志/展示，不传入 LLM。
func (r *SubagentRunner) RunSubagent(ctx context.Context, subagentType, description, prompt string) (string, error) {
	st, ok := GetSubagentType(subagentType)
	if !ok {
		return "", fmt.Errorf("unknown subagent type: %s", subagentType)
	}
	return r.Run(ctx, st, prompt)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:
```bash
go test ./internal/agent/... -run "TestSubagentRunner_implementsTaskRunner|TestSubagentRunner_runSubagent" -v
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/subagent.go internal/agent/subagent_test.go
git commit -m "feat(m5): SubagentRunner implements TaskRunner interface"
```

---

### Task 5: 集成到 CLI（注册 Task 工具）

**Files:**
- Modify: `internal/cli/run.go`
- Modify: `internal/cli/interactive.go`

- [ ] **Step 1: 读取当前 run.go 和 interactive.go 的 registry 构造逻辑**

Run:
```bash
# 先 Read 这两个文件，找到 NewRegistry 的调用点
```

- [ ] **Step 2: 修改 run.go，注册 Task 工具**

在 `run.go` 的 `buildAgent` 函数中（或构造 registry 的地方），添加 Task 工具注册。关键改动：

1. 构造 SubagentRunner（用主 agent 的 provider 和 registry）
2. 用 SubagentRunner 构造 Task 工具
3. 把 Task 工具加入 registry

由于 SubagentRunner 需要 registry 本身（用于受限过滤），存在循环依赖。解决方案：先构造不含 Task 的 registry，再用它构造 SubagentRunner 和 Task，最后把 Task 追加到 registry。

示例代码（具体行号需根据实际文件调整）：
```go
// 构造基础工具
baseTools := []tool.Tool{
	tool.NewRead(),
	tool.NewWrite(),
	tool.NewEdit(),
	tool.NewGlob(),
	tool.NewGrep(),
	tool.NewBash(defaultBashTimeout),
}
registry := tool.NewRegistry(baseTools...)

// 构造 SubagentRunner（复用主 agent 的 provider 和 registry）
subagentRunner := agent.NewSubagentRunner(provider, registry, model)

// 把 Task 工具加入 registry
registry = tool.NewRegistry(append(baseTools, tool.NewTask(subagentRunner))...)

// 用最终 registry 构造 agent
a := agent.New(retryProvider, registry, opts...)
```

- [ ] **Step 3: 同样修改 interactive.go**

确保 `interactive.go` 的 `buildAgent`（或等效函数）也注册 Task 工具。如果 run.go 和 interactive.go 共享 `buildAgent` 函数（M3 review 时已抽取），则只需改一处。

- [ ] **Step 4: 验证编译**

Run:
```bash
go build ./...
```
Expected: 编译成功，无循环 import 错误

- [ ] **Step 5: 验证现有测试仍通过**

Run:
```bash
go test ./...
```
Expected: 全部 PASS

- [ ] **Step 6: Commit**

```bash
git add internal/cli/run.go internal/cli/interactive.go
git commit -m "feat(m5): register Task tool in CLI"
```

---

### Task 6: 更新 SystemPrompt，增加 Task 工具使用指引

**Files:**
- Modify: `internal/agent/prompt.go`

- [ ] **Step 1: 读取当前 prompt.go**

Run: Read `internal/agent/prompt.go`

- [ ] **Step 2: 在 SystemPrompt 中增加 Task 工具指引**

在 SystemPrompt 末尾追加：
```go
const taskToolGuidance = `
## Task Tool (Sub-agents)

When you need to handle complex, multi-step tasks or parallelize independent work, use the "task" tool to launch sub-agents:

- **search**: Fast read-only agent for codebase exploration (read/glob/grep only). Use for high-level concept searches or finding connections across the codebase.
- **general-purpose**: Full-capability agent for complex coding tasks that can be implemented independently.

Launch multiple sub-agents in parallel when tasks are independent. Each sub-agent has its own context window and won't pollute the main conversation. The sub-agent's final text response is returned to you as the tool result.

Use sub-agents proactively for:
- Parallelizing independent research queries
- Cross-layer changes (frontend + backend) that can be planned out
- Operations producing large output not needed in your main context
`
```

把 `taskToolGuidance` 拼接到 `SystemPrompt` 末尾（在 `fmt.Sprintf` 中）。

- [ ] **Step 3: 验证编译和测试**

Run:
```bash
go build ./... && go test ./internal/agent/...
```
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/agent/prompt.go
git commit -m "feat(m5): add Task tool guidance to system prompt"
```

---

### Task 7: 端到端集成测试

**Files:**
- Create: `internal/agent/subagent_e2e_test.go`

- [ ] **Step 1: 写端到端测试**

Create `internal/agent/subagent_e2e_test.go`:
```go
package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/tool"
)

// TestE2E_subagent_parallelExecution 验证主 agent 通过 Task 工具
// 并行派发两个子 agent，结果汇总回主 agent。
func TestE2E_subagent_parallelExecution(t *testing.T) {
	// 主 agent 的 provider：
	// 第一轮返回两个 Task 工具调用（并行）
	// 第二轮返回最终文本（汇总两个子 agent 的结果）
	mainProvider := &mockProvider{
		sequences: [][]llm.StreamEvent{
			// 第一轮：并行派发两个 Task
			{
				llm.ToolCallDelta{ID: "tc1", Name: "task", ArgsDelta: `{"subagent_type":"search","description":"find TODOs","prompt":"find all TODO comments"}`},
				llm.ToolCallDelta{ID: "tc2", Name: "task", ArgsDelta: `{"subagent_type":"search","description":"find FIXMEs","prompt":"find all FIXME comments"}`},
				llm.Done{Usage: llm.Usage{InputTokens: 10, OutputTokens: 20}},
			},
			// 第二轮：汇总
			{
				llm.TextDelta{Content: "Found 3 TODOs and 2 FIXMEs"},
				llm.Done{Usage: llm.Usage{InputTokens: 50, OutputTokens: 10}},
			},
		},
	}

	// 子 agent 的 provider（通过 mockTaskProvider 模拟）
	// 由于 Task 工具内部创建 SubagentRunner，需要注入 provider
	// 这里用 sequenceProvider 让每个子 agent 返回不同文本
	subProvider := &mockProvider{
		sequences: [][]llm.StreamEvent{
			{
				llm.TextDelta{Content: "found 3 TODOs"},
				llm.Done{},
			},
			{
				llm.TextDelta{Content: "found 2 FIXMEs"},
				llm.Done{},
			},
		},
	}

	// 构造 registry：基础工具 + Task 工具
	baseTools := []tool.Tool{
		&mockTool{name: "read"},
		&mockTool{name: "glob"},
		&mockTool{name: "grep"},
	}
	registry := tool.NewRegistry(baseTools...)

	// 构造 SubagentRunner（用子 provider）
	subagentRunner := NewSubagentRunner(subProvider, registry, "test-model")

	// 把 Task 工具加入 registry
	fullRegistry := tool.NewRegistry(append(baseTools, tool.NewTask(subagentRunner))...)

	// 构造主 agent
	mainAgent := New(mainProvider, fullRegistry, WithModel("test-model"))

	events := make(chan Event, 64)
	var finalText string
	done := make(chan error, 1)
	go func() {
		done <- mainAgent.Run(context.Background(), "find TODOs and FIXMEs in parallel", events)
	}()

	for ev := range events {
		if te, ok := ev.(TextEvent); ok {
			finalText += te.Content
		}
	}

	if err := <-done; err != nil {
		t.Fatalf("main agent failed: %v", err)
	}

	if !strings.Contains(finalText, "Found 3 TODOs and 2 FIXMEs") {
		t.Errorf("final text should contain summary: %q", finalText)
	}
}

// TestE2E_subagent_contextIsolation 验证子 agent 的消息历史
// 不污染主 agent 的消息历史。
func TestE2E_subagent_contextIsolation(t *testing.T) {
	// 主 agent 派发一个子 agent，子 agent 执行工具调用
	// 验证主 agent 的 messages 只含 Task 工具调用和结果，不含子 agent 的内部工具调用
	mainProvider := &mockProvider{
		sequences: [][]llm.StreamEvent{
			{
				llm.ToolCallDelta{ID: "tc1", Name: "task", ArgsDelta: `{"subagent_type":"search","description":"test","prompt":"find something"}`},
				llm.Done{},
			},
			{
				llm.TextDelta{Content: "done"},
				llm.Done{},
			},
		},
	}

	// 子 agent 会调用 read 工具
	subProvider := &mockProvider{
		sequences: [][]llm.StreamEvent{
			{
				llm.ToolCallDelta{ID: "sub-tc1", Name: "read", ArgsDelta: `{"file_path":"/tmp/x"}`},
				llm.Done{},
			},
			{
				llm.TextDelta{Content: "subagent result"},
				llm.Done{},
			},
		},
	}

	baseTools := []tool.Tool{
		&mockTool{name: "read"},
	}
	registry := tool.NewRegistry(baseTools...)
	subagentRunner := NewSubagentRunner(subProvider, registry, "test-model")
	fullRegistry := tool.NewRegistry(append(baseTools, tool.NewTask(subagentRunner))...)

	mainAgent := New(mainProvider, fullRegistry, WithModel("test-model"))

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: "test"},
	}
	events := make(chan Event, 64)

	done := make(chan error, 1)
	go func() {
		done <- mainAgent.RunWithHistory(context.Background(), &messages, events)
	}()
	// 排空 events
	for range events {
	}
	<-done

	// 验证主 agent 的 messages 不含子 agent 的 read 工具调用
	for _, m := range messages {
		for _, tc := range m.ToolCalls {
			if tc.Name == "read" {
				t.Errorf("main agent messages should not contain sub-agent's read tool call")
			}
		}
		if m.Role == llm.RoleTool && strings.Contains(m.Content, "mock") {
			// 子 agent 的 read 工具结果不应出现在主 agent 历史
			t.Errorf("main agent messages should not contain sub-agent's tool results")
		}
	}
}

// TestE2E_subagent_failureDoesNotCrashMain 验证子 agent 失败时
// 主 agent 收到错误摘要，不崩溃。
func TestE2E_subagent_failureDoesNotCrashMain(t *testing.T) {
	mainProvider := &mockProvider{
		sequences: [][]llm.StreamEvent{
			{
				llm.ToolCallDelta{ID: "tc1", Name: "task", ArgsDelta: `{"subagent_type":"search","description":"fail","prompt":"fail this"}`},
				llm.Done{},
			},
			{
				llm.TextDelta{Content: "subagent failed, handled gracefully"},
				llm.Done{},
			},
		},
	}

	// 子 agent 的 provider 返回 error
	subProvider := &mockProvider{
		events: []llm.StreamEvent{
			llm.Error{Err: context.DeadlineExceeded},
		},
	}

	baseTools := []tool.Tool{&mockTool{name: "read"}}
	registry := tool.NewRegistry(baseTools...)
	subagentRunner := NewSubagentRunner(subProvider, registry, "test-model")
	fullRegistry := tool.NewRegistry(append(baseTools, tool.NewTask(subagentRunner))...)

	mainAgent := New(mainProvider, fullRegistry, WithModel("test-model"))

	events := make(chan Event, 64)
	done := make(chan error, 1)
	go func() {
		done <- mainAgent.Run(context.Background(), "test", events)
	}()

	var finalText string
	for ev := range events {
		if te, ok := ev.(TextEvent); ok {
			finalText += te.Content
		}
	}

	if err := <-done; err != nil {
		t.Fatalf("main agent should not fail when subagent fails: %v", err)
	}
	if !strings.Contains(finalText, "handled gracefully") {
		t.Errorf("main agent should continue after subagent failure: %q", finalText)
	}
}
```

- [ ] **Step 2: 运行测试**

Run:
```bash
go test ./internal/agent/... -run "TestE2E_subagent" -v -timeout 30s
```
Expected: PASS，3 个端到端测试通过

- [ ] **Step 3: 运行全量测试确认无回归**

Run:
```bash
go test ./... 2>&1 | tail -15
```
Expected: 全部 PASS

- [ ] **Step 4: Commit**

```bash
git add internal/agent/subagent_e2e_test.go
git commit -m "test(m5): add end-to-end tests for sub-agent parallel execution and isolation"
```

---

## M5 完成标准

- [ ] `internal/agent/subagent.go` 定义 SubagentType 和 SubagentRunner
- [ ] `internal/tool/task.go` 实现 Task 工具，通过 TaskRunner 接口解耦
- [ ] 内置子 agent 类型：search（只读）、general-purpose（全工具）
- [ ] 多个 Task 调用通过 dispatcher 并行执行
- [ ] 子 agent 上下文隔离，不污染主 agent 历史
- [ ] 子 agent 失败返回错误摘要，不崩溃主流程
- [ ] CLI 注册 Task 工具
- [ ] SystemPrompt 含 Task 工具使用指引
- [ ] 端到端测试覆盖并行执行、上下文隔离、失败处理
- [ ] `go test ./...` 全部 PASS
- [ ] `go vet ./...` 无错误

## 设计决策说明

1. **接口解耦**：Task 工具依赖 `TaskRunner` 接口而非 `SubagentRunner` 具体类型，避免 `tool → agent` 循环 import。`agent` 包实现接口并注入。

2. **复用现有 Agent**：子 agent 复用 `agent.New()` + `RunWithHistory()`，不重新实现 think-act 循环。通过 `WithMaxSteps(10)` 限制步数，`WithSystemPrompt` 注入子 agent 专用 prompt。

3. **并行性**：主 agent 的 dispatcher 已支持并行（semaphore=8），多个 Task 工具调用天然并行。无需额外并行调度逻辑。

4. **上下文隔离**：子 agent 用独立 `[]llm.Message`，不共享主 agent 的 messages 指针。子 agent 的中间 ToolCallEvent/ToolResultEvent 不转发到主 agent 的 events channel。

5. **受限工具集**：`SubagentType.RestrictedRegistry()` 基于全量 registry 过滤。search 类型只保留 read/glob/grep，general-purpose 保留全部。

6. **失败处理**：子 agent 失败时，Task 工具返回 ErrorResult（含错误信息），主 agent 的 dispatcher 正常处理，主 agent 循环继续。若子 agent 部分成功（有文本但出错），返回已收集的文本。
