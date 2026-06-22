package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// Todo TodoWrite 工具：维护任务清单，帮助 agent 跟踪多步任务进度。
// 每次调用用传入的 todos 完整替换当前清单（与 Claude Code 行为一致）。
// 状态保存在内存中，生命周期与工具实例（即 agent 会话）一致。
type Todo struct {
	mu    sync.Mutex
	items []TodoItem
}

// TodoItem 单条任务。
type TodoItem struct {
	Content  string `json:"content"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

// NewTodo 构造 TodoWrite 工具。
func NewTodo() *Todo { return &Todo{} }

func (*Todo) Name() string { return "todo_write" }

func (*Todo) Description() string {
	return "Manage a task list for the current session. " +
		"Each call replaces the entire list. Use this to plan multi-step work, " +
		"track progress, and show the user what's pending. " +
		"Only one task should be in_progress at a time."
}

func (*Todo) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "todos": {
      "type": "array",
      "description": "The complete task list. Replaces the previous list on every call.",
      "items": {
        "type": "object",
        "properties": {
          "content": {"type": "string", "description": "task description"},
          "status": {"type": "string", "enum": ["pending", "in_progress", "completed"]},
          "priority": {"type": "string", "enum": ["high", "medium", "low"]}
        },
        "required": ["content", "status", "priority"]
      }
    }
  },
  "required": ["todos"]
}`)
}

type todoArgs struct {
	Todos []TodoItem `json:"todos"`
}

func (t *Todo) Run(ctx context.Context, args json.RawMessage) Result {
	var a todoArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("parse args: %v", err)
	}
	// 校验
	inProgress := 0
	for i, item := range a.Todos {
		if strings.TrimSpace(item.Content) == "" {
			return ErrorResult("todos[%d].content is required", i)
		}
		switch item.Status {
		case "pending", "in_progress", "completed":
		default:
			return ErrorResult("todos[%d].status must be pending/in_progress/completed, got %q", i, item.Status)
		}
		switch item.Priority {
		case "high", "medium", "low":
		default:
			return ErrorResult("todos[%d].priority must be high/medium/low, got %q", i, item.Priority)
		}
		if item.Status == "in_progress" {
			inProgress++
		}
	}
	if inProgress > 1 {
		return ErrorResult("at most one todo can be in_progress, got %d", inProgress)
	}

	t.mu.Lock()
	t.items = a.Todos
	t.mu.Unlock()

	return Result{Content: t.format(a.Todos), Meta: map[string]any{"count": len(a.Todos)}}
}

// format 渲染任务清单为可读文本。
func (t *Todo) format(items []TodoItem) string {
	if len(items) == 0 {
		return "Todo list is empty."
	}
	var b strings.Builder
	statusMark := map[string]string{
		"pending":     "[ ]",
		"in_progress": "[~]",
		"completed":   "[x]",
	}
	for i, item := range items {
		mark := statusMark[item.Status]
		fmt.Fprintf(&b, "%d. %s %s (%s) %s\n", i+1, mark, item.Priority, item.Status, item.Content)
	}
	return b.String()
}

// 确保 Todo 实现 Tool 接口
var _ Tool = (*Todo)(nil)
