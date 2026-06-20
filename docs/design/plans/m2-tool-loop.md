# M2 工具闭环实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** agent 能多步 think-act，调用 read/write/edit/glob/grep/bash 工具完成简单编码任务，`trae run "在当前目录创建 hello.py 并运行"` 能闭环。

**Architecture:** Tool 接口统一抽象，dispatcher 并行/串行调度。agent loop 消费 StreamEvent（含新增 ToolCallDelta），收集 tool_calls 后 dispatcher 执行，结果回灌 messages 进入下一轮。provider 层扩展支持 tool_call 流式增量。

**Tech Stack:** 标准库（os/exec、path/filepath、regexp、bufio）、errgroup（并行工具）、encoding/json

---

## 文件结构

- Create: `internal/tool/tool.go` — Tool 接口、Result、JSON schema
- Create: `internal/tool/tool_test.go`
- Create: `internal/tool/read.go` — 读文件，行号、offset/limit
- Create: `internal/tool/read_test.go`
- Create: `internal/tool/write.go` — 写文件
- Create: `internal/tool/write_test.go`
- Create: `internal/tool/edit.go` — str_replace，唯一性校验
- Create: `internal/tool/edit_test.go`
- Create: `internal/tool/glob.go` — `**` glob 匹配
- Create: `internal/tool/glob_test.go`
- Create: `internal/tool/grep.go` — 正则搜索，行号、上下文
- Create: `internal/tool/grep_test.go`
- Create: `internal/tool/bash.go` — exec，超时、流式输出
- Create: `internal/tool/bash_test.go`
- Create: `internal/tool/dispatcher.go` — 并行/串行调度
- Create: `internal/tool/dispatcher_test.go`
- Modify: `internal/llm/provider.go` — 加 ToolCallDelta、ToolCall、ToolDef
- Modify: `internal/llm/anthropic.go` — 解析 tool_use 事件
- Modify: `internal/llm/openai.go` — 解析 tool_calls delta
- Modify: `internal/llm/anthropic_test.go` / `openai_test.go` — 补 tool_call 测试
- Create: `internal/agent/loop.go` — think-act 循环
- Create: `internal/agent/loop_test.go` — mock provider 集成测试
- Create: `internal/agent/prompt.go` — system prompt
- Modify: `internal/cli/run.go` — 接入 agent loop
- Modify: `internal/config/config.go` — MaxSteps 字段

---

### Task 1: Tool 接口与核心类型

**Files:**
- Create: `internal/tool/tool.go`
- Create: `internal/tool/tool_test.go`

- [ ] **Step 1: 写核心类型**

Create `internal/tool/tool.go`:
```go
package tool

import (
	"context"
	"encoding/json"
	"fmt"
)

// Result 工具执行结果。
type Result struct {
	Content string         // 文本结果（回灌给 LLM）
	IsError bool           // 是否为错误
	Meta    map[string]any // 元信息（行号、文件路径等，可选）
}

// Tool 工具接口。
type Tool interface {
	Name() string
	Description() string
	// Schema 返回 JSON schema 描述参数（暴露给 LLM）。
	Schema() json.RawMessage
	// Run 执行工具，args 是 LLM 传入的 JSON 参数。
	Run(ctx context.Context, args json.RawMessage) Result
}

// Registry 工具注册表。
type Registry struct {
	tools map[string]Tool
}

func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{tools: make(map[string]Tool, len(tools))}
	for _, t := range tools {
		r.tools[t.Name()] = t
	}
	return r
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) List() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

// ErrorResult 构造错误结果的快捷函数。
func ErrorResult(format string, args ...any) Result {
	return Result{Content: fmt.Sprintf(format, args...), IsError: true}
}
```

- [ ] **Step 2: 写测试**

Create `internal/tool/tool_test.go`:
```go
package tool

import (
	"context"
	"encoding/json"
	"testing"
)

type echoTool struct{}

func (echoTool) Name() string             { return "echo" }
func (echoTool) Description() string      { return "echo back the input" }
func (echoTool) Schema() json.RawMessage  { return json.RawMessage(`{"type":"object","properties":{"msg":{"type":"string"}}}`) }
func (echoTool) Run(ctx context.Context, args json.RawMessage) Result {
	var v struct{ Msg string `json:"msg"` }
	json.Unmarshal(args, &v)
	return Result{Content: v.Msg}
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry(echoTool{})
	got, ok := r.Get("echo")
	if !ok {
		t.Fatal("echo not found")
	}
	res := got.Run(context.Background(), json.RawMessage(`{"msg":"hi"}`))
	if res.Content != "hi" {
		t.Errorf("content = %q, want hi", res.Content)
	}
}

func TestRegistry_Get_notFound(t *testing.T) {
	r := NewRegistry(echoTool{})
	_, ok := r.Get("nope")
	if ok {
		t.Error("expected not found")
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry(echoTool{})
	tools := r.List()
	if len(tools) != 1 {
		t.Fatalf("got %d tools, want 1", len(tools))
	}
	if tools[0].Name() != "echo" {
		t.Errorf("name = %q, want echo", tools[0].Name())
	}
}

func TestErrorResult(t *testing.T) {
	r := ErrorResult("bad: %s", "x")
	if !r.IsError {
		t.Error("IsError should be true")
	}
	if r.Content != "bad: x" {
		t.Errorf("content = %q, want 'bad: x'", r.Content)
	}
}
```

- [ ] **Step 3: 运行测试**

Run:
```bash
go test ./internal/tool/... -v
```
Expected: 4 个测试 PASS

- [ ] **Step 4: Commit**

```bash
git add internal/tool/tool.go internal/tool/tool_test.go
git commit -m "feat(m2): add tool interface and registry"
```

---

### Task 2: read 工具

**Files:**
- Create: `internal/tool/read.go`
- Create: `internal/tool/read_test.go`

- [ ] **Step 1: 写失败测试**

Create `internal/tool/read_test.go`:
```go
package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRead_fullFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0o644)

	r := NewRead()
	args, _ := json.Marshal(map[string]string{"file_path": path})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "line1") {
		t.Errorf("missing line1: %s", res.Content)
	}
	// 应含行号前缀
	if !strings.Contains(res.Content, "1→") || !strings.Contains(res.Content, "2→") {
		t.Errorf("missing line numbers: %s", res.Content)
	}
}

func TestRead_withOffsetLimit(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	os.WriteFile(path, []byte("a\nb\nc\nd\ne\n"), 0o644)

	r := NewRead()
	args, _ := json.Marshal(map[string]any{"file_path": path, "offset": 2, "limit": 2})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "b") || !strings.Contains(res.Content, "c") {
		t.Errorf("missing b/c: %s", res.Content)
	}
	if strings.Contains(res.Content, "a") && strings.Contains(res.Content, "d") {
		t.Errorf("should not contain a/d: %s", res.Content)
	}
}

func TestRead_fileNotFound(t *testing.T) {
	r := NewRead()
	args, _ := json.Marshal(map[string]string{"file_path": "/nonexistent"})
	res := r.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for missing file")
	}
}

func TestRead_missingArg(t *testing.T) {
	r := NewRead()
	res := r.Run(context.Background(), json.RawMessage(`{}`))
	if !res.IsError {
		t.Error("expected error for missing file_path")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
go test ./internal/tool/... -run TestRead -v
```
Expected: FAIL（undefined: NewRead）

- [ ] **Step 3: 实现 read**

Create `internal/tool/read.go`:
```go
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Read struct{}

func NewRead() *Read { return &Read{} }

func (Read) Name() string        { return "read" }
func (Read) Description() string { return "Read a file with line numbers. Supports offset and limit." }

func (Read) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "file_path": {"type": "string", "description": "absolute path to the file"},
    "offset": {"type": "integer", "description": "1-based line number to start from"},
    "limit": {"type": "integer", "description": "max number of lines to read"}
  },
  "required": ["file_path"]
}`)
}

type readArgs struct {
	FilePath string `json:"file_path"`
	Offset   int    `json:"offset"`
	Limit    int    `json:"limit"`
}

func (Read) Run(ctx context.Context, args json.RawMessage) Result {
	var a readArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("parse args: %v", err)
	}
	if a.FilePath == "" {
		return ErrorResult("file_path is required")
	}

	data, err := os.ReadFile(a.FilePath)
	if err != nil {
		return ErrorResult("read file: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	start := 1
	if a.Offset > 0 {
		start = a.Offset
	}
	end := len(lines)
	if a.Limit > 0 && start-1+a.Limit < end {
		end = start - 1 + a.Limit
	}
	if start > len(lines) {
		return Result{Content: ""}
	}
	if start < 1 {
		start = 1
	}

	maxWidth := len(fmt.Sprintf("%d", end))
	var b strings.Builder
	for i := start - 1; i < end && i < len(lines); i++ {
		fmt.Fprintf(&b, "%*d→%s\n", maxWidth, i+1, lines[i])
	}
	return Result{Content: b.String()}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:
```bash
go test ./internal/tool/... -run TestRead -v
```
Expected: 4 个测试 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tool/read.go internal/tool/read_test.go
git commit -m "feat(m2): add read tool with line numbers and offset/limit"
```

---

### Task 3: write 工具

**Files:**
- Create: `internal/tool/write.go`
- Create: `internal/tool/write_test.go`

- [ ] **Step 1: 写失败测试**

Create `internal/tool/write_test.go`:
```go
package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWrite_createFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "new.txt")
	r := NewWrite()
	args, _ := json.Marshal(map[string]string{"file_path": path, "content": "hello"})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "hello" {
		t.Errorf("file content = %q, want hello", data)
	}
}

func TestWrite_overwriteExisting(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	os.WriteFile(path, []byte("old"), 0o644)

	r := NewWrite()
	args, _ := json.Marshal(map[string]string{"file_path": path, "content": "new"})
	r.Run(context.Background(), args)
	data, _ := os.ReadFile(path)
	if string(data) != "new" {
		t.Errorf("content = %q, want new", data)
	}
}

func TestWrite_createParentDir(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "sub", "dir", "f.txt")
	r := NewWrite()
	args, _ := json.Marshal(map[string]string{"file_path": path, "content": "x"})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}
}

func TestWrite_missingArg(t *testing.T) {
	r := NewWrite()
	res := r.Run(context.Background(), json.RawMessage(`{}`))
	if !res.IsError {
		t.Error("expected error for missing args")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
go test ./internal/tool/... -run TestWrite -v
```
Expected: FAIL

- [ ] **Step 3: 实现 write**

Create `internal/tool/write.go`:
```go
package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
)

type Write struct{}

func NewWrite() *Write { return &Write{} }

func (Write) Name() string        { return "write" }
func (Write) Description() string { return "Write content to a file, creating parent dirs if needed. Overwrites existing content." }

func (Write) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "file_path": {"type": "string", "description": "absolute path to the file"},
    "content": {"type": "string", "description": "content to write"}
  },
  "required": ["file_path", "content"]
}`)
}

type writeArgs struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

func (Write) Run(ctx context.Context, args json.RawMessage) Result {
	var a writeArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("parse args: %v", err)
	}
	if a.FilePath == "" {
		return ErrorResult("file_path is required")
	}

	dir := filepath.Dir(a.FilePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ErrorResult("mkdir: %v", err)
	}
	if err := os.WriteFile(a.FilePath, []byte(a.Content), 0o644); err != nil {
		return ErrorResult("write file: %v", err)
	}
	return Result{Content: "wrote " + a.FilePath}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:
```bash
go test ./internal/tool/... -run TestWrite -v
```
Expected: 4 个测试 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tool/write.go internal/tool/write_test.go
git commit -m "feat(m2): add write tool with parent dir creation"
```

---

### Task 4: edit 工具（str_replace）

**Files:**
- Create: `internal/tool/edit.go`
- Create: `internal/tool/edit_test.go`

- [ ] **Step 1: 写失败测试**

Create `internal/tool/edit_test.go`:
```go
package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestEdit_strReplace(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	os.WriteFile(path, []byte("foo bar baz"), 0o644)

	r := NewEdit()
	args, _ := json.Marshal(map[string]string{
		"file_path":  path,
		"old_string": "bar",
		"new_string": "QUX",
	})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "foo QUX baz" {
		t.Errorf("content = %q, want 'foo QUX baz'", data)
	}
}

func TestEdit_notUnique(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	os.WriteFile(path, []byte("a a a"), 0o644)

	r := NewEdit()
	args, _ := json.Marshal(map[string]string{
		"file_path":  path,
		"old_string": "a",
		"new_string": "b",
	})
	res := r.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for non-unique match")
	}
}

func TestEdit_notFound(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	os.WriteFile(path, []byte("hello"), 0o644)

	r := NewEdit()
	args, _ := json.Marshal(map[string]string{
		"file_path":  path,
		"old_string": "world",
		"new_string": "x",
	})
	res := r.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for not found")
	}
}

func TestEdit_replaceAll(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "f.txt")
	os.WriteFile(path, []byte("a a a"), 0o644)

	r := NewEdit()
	args, _ := json.Marshal(map[string]any{
		"file_path":   path,
		"old_string":  "a",
		"new_string":  "b",
		"replace_all": true,
	})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "b b b" {
		t.Errorf("content = %q, want 'b b b'", data)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
go test ./internal/tool/... -run TestEdit -v
```
Expected: FAIL

- [ ] **Step 3: 实现 edit**

Create `internal/tool/edit.go`:
```go
package tool

import (
	"context"
	"encoding/json"
	"os"
	"strings"
)

type Edit struct{}

func NewEdit() *Edit { return &Edit{} }

func (Edit) Name() string        { return "edit" }
func (Edit) Description() string { return "Replace a unique string in a file. Errors if old_string is not found or not unique (unless replace_all)." }

func (Edit) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "file_path": {"type": "string"},
    "old_string": {"type": "string", "description": "must be unique unless replace_all"},
    "new_string": {"type": "string"},
    "replace_all": {"type": "boolean", "description": "replace all occurrences"}
  },
  "required": ["file_path", "old_string", "new_string"]
}`)
}

type editArgs struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

func (Edit) Run(ctx context.Context, args json.RawMessage) Result {
	var a editArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("parse args: %v", err)
	}
	if a.FilePath == "" {
		return ErrorResult("file_path is required")
	}
	if a.OldString == "" {
		return ErrorResult("old_string is required")
	}

	data, err := os.ReadFile(a.FilePath)
	if err != nil {
		return ErrorResult("read file: %v", err)
	}
	content := string(data)

	if a.ReplaceAll {
		newContent := strings.ReplaceAll(content, a.OldString, a.NewString)
		if newContent == content {
			return ErrorResult("old_string not found")
		}
		if err := os.WriteFile(a.FilePath, []byte(newContent), 0o644); err != nil {
			return ErrorResult("write file: %v", err)
		}
		return Result{Content: "replaced all in " + a.FilePath}
	}

	count := strings.Count(content, a.OldString)
	if count == 0 {
		return ErrorResult("old_string not found in %s", a.FilePath)
	}
	if count > 1 {
		return ErrorResult("old_string appears %d times, must be unique (use replace_all)", count)
	}

	newContent := strings.Replace(content, a.OldString, a.NewString, 1)
	if err := os.WriteFile(a.FilePath, []byte(newContent), 0o644); err != nil {
		return ErrorResult("write file: %v", err)
	}
	return Result{Content: "edited " + a.FilePath}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:
```bash
go test ./internal/tool/... -run TestEdit -v
```
Expected: 4 个测试 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tool/edit.go internal/tool/edit_test.go
git commit -m "feat(m2): add edit tool with str_replace and uniqueness check"
```

---

### Task 5: glob + grep 工具

**Files:**
- Create: `internal/tool/glob.go`
- Create: `internal/tool/glob_test.go`
- Create: `internal/tool/grep.go`
- Create: `internal/tool/grep_test.go`

- [ ] **Step 1: 写 glob 测试**

Create `internal/tool/glob_test.go`:
```go
package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlob_match(t *testing.T) {
	tmp := t.TempDir()
	os.MkdirAll(filepath.Join(tmp, "sub"), 0o755)
	os.WriteFile(filepath.Join(tmp, "a.go"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(tmp, "b.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(tmp, "sub", "c.go"), []byte("x"), 0o644)

	r := NewGlob()
	args, _ := json.Marshal(map[string]string{"pattern": "**/*.go", "path": tmp})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "a.go") {
		t.Errorf("missing a.go: %s", res.Content)
	}
	if !strings.Contains(res.Content, "c.go") {
		t.Errorf("missing c.go: %s", res.Content)
	}
	if strings.Contains(res.Content, "b.txt") {
		t.Errorf("should not contain b.txt: %s", res.Content)
	}
}

func TestGlob_noMatch(t *testing.T) {
	tmp := t.TempDir()
	r := NewGlob()
	args, _ := json.Marshal(map[string]string{"pattern": "**/*.xyz", "path": tmp})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Errorf("no match should not be error: %s", res.Content)
	}
	if res.Content != "" {
		t.Errorf("expected empty, got: %s", res.Content)
	}
}
```

- [ ] **Step 2: 实现 glob**

Create `internal/tool/glob.go`:
```go
package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Glob struct{}

func NewGlob() *Glob { return &Glob{} }

func (Glob) Name() string        { return "glob" }
func (Glob) Description() string { return "Find files matching a glob pattern (supports **)." }

func (Glob) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "pattern": {"type": "string", "description": "glob pattern, e.g. **/*.go"},
    "path": {"type": "string", "description": "directory to search, default cwd"}
  },
  "required": ["pattern"]
}`)
}

type globArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

func (Glob) Run(ctx context.Context, args json.RawMessage) Result {
	var a globArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("parse args: %v", err)
	}
	if a.Pattern == "" {
		return ErrorResult("pattern is required")
	}
	root := a.Path
	if root == "" {
		root, _ = os.Getwd()
	}

	// 把 ** 转为 filepath.Walk 的全匹配
	var matches []string
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if matchGlob(a.Pattern, rel) {
			matches = append(matches, p)
		}
		return nil
	})

	return Result{Content: strings.Join(matches, "\n")}
}

// matchGlob 支持 ** 通配。
func matchGlob(pattern, name string) bool {
	// 简化实现：把 ** 替换为 *，再用 filepath.Match
	// 更完整实现应处理 ** 跨目录，这里用 Match + 逐段
	parts := strings.Split(pattern, "/")
	nameParts := strings.Split(name, string(filepath.Separator))
	return matchSegments(parts, nameParts)
}

func matchSegments(pat, name []string) bool {
	if len(pat) == 0 {
		return len(name) == 0
	}
	if pat[0] == "**" {
		// ** 匹配 0 或多段
		for i := 0; i <= len(name); i++ {
			if matchSegments(pat[1:], name[i:]) {
				return true
			}
		}
		return false
	}
	if len(name) == 0 {
		return false
	}
	ok, _ := filepath.Match(pat[0], name[0])
	return ok && matchSegments(pat[1:], name[1:])
}
```

- [ ] **Step 3: 写 grep 测试**

Create `internal/tool/grep_test.go`:
```go
package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrep_match(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("foo\nbar\nbaz\n"), 0o644)
	os.WriteFile(filepath.Join(tmp, "b.txt"), []byte("qux\nfoo\n"), 0o644)

	r := NewGrep()
	args, _ := json.Marshal(map[string]string{"pattern": "foo", "path": tmp})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "a.txt") {
		t.Errorf("missing a.txt: %s", res.Content)
	}
	if !strings.Contains(res.Content, "b.txt") {
		t.Errorf("missing b.txt: %s", res.Content)
	}
}

func TestGrep_regex(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("test123\ntest\n"), 0o644)

	r := NewGrep()
	args, _ := json.Marshal(map[string]string{"pattern": "test\\d+", "path": tmp})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "test123") {
		t.Errorf("missing test123: %s", res.Content)
	}
	if strings.Contains(res.Content, "\ntest\n") {
		t.Errorf("should not match plain 'test': %s", res.Content)
	}
}

func TestGrep_noMatch(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("hello"), 0o644)

	r := NewGrep()
	args, _ := json.Marshal(map[string]string{"pattern": "world", "path": tmp})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Errorf("no match should not be error: %s", res.Content)
	}
}
```

- [ ] **Step 4: 实现 grep**

Create `internal/tool/grep.go`:
```go
package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Grep struct{}

func NewGrep() *Grep { return &Grep{} }

func (Grep) Name() string        { return "grep" }
func (Grep) Description() string { return "Search file contents with regex. Returns matches with file:line:content." }

func (Grep) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "pattern": {"type": "string", "description": "regex pattern"},
    "path": {"type": "string", "description": "directory or file to search"}
  },
  "required": ["pattern"]
}`)
}

type grepArgs struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path"`
}

func (Grep) Run(ctx context.Context, args json.RawMessage) Result {
	var a grepArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("parse args: %v", err)
	}
	if a.Pattern == "" {
		return ErrorResult("pattern is required")
	}
	re, err := regexp.Compile(a.Pattern)
	if err != nil {
		return ErrorResult("compile regex: %v", err)
	}
	root := a.Path
	if root == "" {
		root, _ = os.Getwd()
	}

	var b strings.Builder
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			if re.MatchString(scanner.Text()) {
				fmt.Fprintf(&b, "%s:%d:%s\n", p, lineNum, scanner.Text())
			}
		}
		return nil
	})
	return Result{Content: b.String()}
}
```

- [ ] **Step 5: 运行测试**

Run:
```bash
go test ./internal/tool/... -run "TestGlob|TestGrep" -v
```
Expected: 5 个测试 PASS

- [ ] **Step 6: Commit**

```bash
git add internal/tool/glob.go internal/tool/glob_test.go internal/tool/grep.go internal/tool/grep_test.go
git commit -m "feat(m2): add glob and grep tools"
```

---

### Task 6: bash 工具

**Files:**
- Create: `internal/tool/bash.go`
- Create: `internal/tool/bash_test.go`

- [ ] **Step 1: 写失败测试**

Create `internal/tool/bash_test.go`:
```go
package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBash_simpleCommand(t *testing.T) {
	r := NewBash(10 * time.Second)
	args, _ := json.Marshal(map[string]string{"command": "echo hello"})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "hello") {
		t.Errorf("missing hello: %s", res.Content)
	}
}

func TestBash_exitCode(t *testing.T) {
	r := NewBash(10 * time.Second)
	args, _ := json.Marshal(map[string]string{"command": "exit 3"})
	res := r.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected error for non-zero exit")
	}
}

func TestBash_timeout(t *testing.T) {
	r := NewBash(100 * time.Millisecond)
	args, _ := json.Marshal(map[string]string{"command": "sleep 5"})
	res := r.Run(context.Background(), args)
	if !res.IsError {
		t.Error("expected timeout error")
	}
}

func TestBash_contextCancel(t *testing.T) {
	r := NewBash(10 * time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	args, _ := json.Marshal(map[string]string{"command": "sleep 5"})
	res := r.Run(ctx, args)
	if !res.IsError {
		t.Error("expected cancel error")
	}
}

func TestBash_workdir(t *testing.T) {
	r := NewBash(10 * time.Second)
	args, _ := json.Marshal(map[string]string{"command": "pwd", "workdir": "/tmp"})
	res := r.Run(context.Background(), args)
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "/tmp") {
		t.Errorf("pwd should contain /tmp: %s", res.Content)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
go test ./internal/tool/... -run TestBash -v
```
Expected: FAIL

- [ ] **Step 3: 实现 bash**

Create `internal/tool/bash.go`:
```go
package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

type Bash struct {
	defaultTimeout time.Duration
}

func NewBash(defaultTimeout time.Duration) *Bash {
	if defaultTimeout <= 0 {
		defaultTimeout = 120 * time.Second
	}
	return &Bash{defaultTimeout: defaultTimeout}
}

func (Bash) Name() string        { return "bash" }
func (Bash) Description() string { return "Execute a bash command. Returns combined stdout+stderr." }

func (Bash) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "command": {"type": "string", "description": "bash command to execute"},
    "workdir": {"type": "string", "description": "working directory, default cwd"},
    "timeout_ms": {"type": "integer", "description": "timeout in milliseconds"}
  },
  "required": ["command"]
}`)
}

type bashArgs struct {
	Command   string `json:"command"`
	Workdir   string `json:"workdir"`
	TimeoutMs int    `json:"timeout_ms"`
}

func (b *Bash) Run(ctx context.Context, args json.RawMessage) Result {
	var a bashArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return ErrorResult("parse args: %v", err)
	}
	if a.Command == "" {
		return ErrorResult("command is required")
	}

	timeout := b.defaultTimeout
	if a.TimeoutMs > 0 {
		timeout = time.Duration(a.TimeoutMs) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", a.Command)
	if a.Workdir != "" {
		cmd.Dir = a.Workdir
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	output := buf.String()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return ErrorResult("timeout after %s\n%s", timeout, output)
		}
		return ErrorResult("exit: %v\n%s", err, output)
	}
	return Result{Content: output}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:
```bash
go test ./internal/tool/... -run TestBash -v
```
Expected: 5 个测试 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tool/bash.go internal/tool/bash_test.go
git commit -m "feat(m2): add bash tool with timeout and context cancel"
```

---

### Task 7: dispatcher（并行调度）

**Files:**
- Create: `internal/tool/dispatcher.go`
- Create: `internal/tool/dispatcher_test.go`

- [ ] **Step 1: 写失败测试**

Create `internal/tool/dispatcher_test.go`:
```go
package tool

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"
)

func TestDispatcher_sequential(t *testing.T) {
	r := NewRegistry(echoTool{})
	d := NewDispatcher(r)

	calls := d.Dispatch(context.Background(), []Call{
		{Name: "echo", Args: json.RawMessage(`{"msg":"a"}`)},
		{Name: "echo", Args: json.RawMessage(`{"msg":"b"}`)},
	})
	if len(calls) != 2 {
		t.Fatalf("got %d results, want 2", len(calls))
	}
	if calls[0].Result.Content != "a" {
		t.Errorf("result[0] = %q, want a", calls[0].Result.Content)
	}
	if calls[1].Result.Content != "b" {
		t.Errorf("result[1] = %q, want b", calls[1].Result.Content)
	}
}

func TestDispatcher_parallel(t *testing.T) {
	// 用慢工具验证并行
	slow := &slowTool{delay: 100 * time.Millisecond}
	r := NewRegistry(slow)
	d := NewDispatcher(r)

	start := time.Now()
	calls := d.Dispatch(context.Background(), []Call{
		{Name: "slow", Args: json.RawMessage(`{}`)},
		{Name: "slow", Args: json.RawMessage(`{}`)},
		{Name: "slow", Args: json.RawMessage(`{}`)},
	})
	elapsed := time.Since(start)
	if len(calls) != 3 {
		t.Fatalf("got %d results, want 3", len(calls))
	}
	// 并行 3 个 100ms 应 < 300ms（串行会 > 300ms）
	if elapsed > 250*time.Millisecond {
		t.Errorf("not parallel: elapsed %v", elapsed)
	}
}

func TestDispatcher_unknownTool(t *testing.T) {
	r := NewRegistry(echoTool{})
	d := NewDispatcher(r)
	calls := d.Dispatch(context.Background(), []Call{
		{Name: "nope", Args: json.RawMessage(`{}`)},
	})
	if len(calls) != 1 {
		t.Fatal("expected 1 result")
	}
	if !calls[0].Result.IsError {
		t.Error("expected error for unknown tool")
	}
}

type slowTool struct {
	delay time.Duration
	calls int32
}

func (s *slowTool) Name() string             { return "slow" }
func (s *slowTool) Description() string      { return "slow tool" }
func (s *slowTool) Schema() json.RawMessage  { return json.RawMessage(`{}`) }
func (s *slowTool) Run(ctx context.Context, args json.RawMessage) Result {
	atomic.AddInt32(&s.calls, 1)
	time.Sleep(s.delay)
	return Result{Content: "done"}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
go test ./internal/tool/... -run TestDispatcher -v
```
Expected: FAIL

- [ ] **Step 3: 实现 dispatcher**

Create `internal/tool/dispatcher.go`:
```go
package tool

import (
	"context"
	"sync"
)

// Call 单个工具调用请求。
type Call struct {
	Name string
	Args []byte // json.RawMessage
}

// CallResult 工具调用结果，保持与 Call 的顺序。
type CallResult struct {
	Name   string
	Result Result
}

// Dispatcher 调度工具执行，支持并行。
type Dispatcher struct {
	registry *Registry
}

func NewDispatcher(r *Registry) *Dispatcher {
	return &Dispatcher{registry: r}
}

// Dispatch 并行执行所有调用，返回结果（顺序与输入一致）。
func (d *Dispatcher) Dispatch(ctx context.Context, calls []Call) []CallResult {
	results := make([]CallResult, len(calls))
	var wg sync.WaitGroup
	for i, c := range calls {
		wg.Add(1)
		go func(idx int, call Call) {
			defer wg.Done()
			results[idx] = CallResult{
				Name:   call.Name,
				Result: d.execute(ctx, call),
			}
		}(i, c)
	}
	wg.Wait()
	return results
}

func (d *Dispatcher) execute(ctx context.Context, call Call) Result {
	t, ok := d.registry.Get(call.Name)
	if !ok {
		return ErrorResult("unknown tool: %s", call.Name)
	}
	return t.Run(ctx, call.Args)
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:
```bash
go test ./internal/tool/... -run TestDispatcher -v
```
Expected: 3 个测试 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tool/dispatcher.go internal/tool/dispatcher_test.go
git commit -m "feat(m2): add parallel tool dispatcher"
```

---

### Task 8: 扩展 LLM 层支持 tool_call

**Files:**
- Modify: `internal/llm/provider.go`
- Modify: `internal/llm/anthropic.go`
- Modify: `internal/llm/openai.go`
- Modify: `internal/llm/anthropic_test.go`
- Modify: `internal/llm/openai_test.go`

- [ ] **Step 1: 扩展 provider.go 类型**

在 `internal/llm/provider.go` 加：
```go
// RoleTool 工具结果消息角色（OpenAI 用）。
const RoleTool Role = "tool"

// ToolCallDelta 工具调用增量（流式）。
type ToolCallDelta struct {
	ID        string
	Name      string
	ArgsDelta string
}

func (ToolCallDelta) isStreamEvent() {}

// ToolCall 完整的工具调用（非流式，agent loop 用）。
type ToolCall struct {
	ID   string
	Name string
	Args string // JSON 字符串
}

// ToolDef 暴露给 LLM 的工具定义。
type ToolDef struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

// Message 扩展：ToolCalls 和 ToolCallID
// 修改 Message struct：
type Message struct {
	Role      Role   `json:"role"`
	Content   string `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"` // role=tool 时关联的 call id
}

// Request 扩展：Tools
type Request struct {
	Model       string
	Messages    []Message
	System      string
	MaxTokens   int
	Temperature float64
	Tools       []ToolDef
}
```

注意：Message 和 Request 已存在，需要修改而不是新增。把 ToolCalls/ToolCallID 加到 Message，Tools 加到 Request。

- [ ] **Step 2: 扩展 anthropic.go（请求序列化 + 流式解析）**

**2a. 请求结构加 Tools：**
```go
type anthropicRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	Messages    []anthropicMsg  `json:"messages"`
	System      string          `json:"system,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	Stream      bool            `json:"stream"`
	Tools       []anthropicTool `json:"tools,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}
```

**2b. anthropicMsg 改为支持 content blocks（assistant 的 tool_use 和 user 的 tool_result）：**

Anthropic 的消息格式与 OpenAI 不同：assistant 消息含 tool_use 时，content 是数组（text block + tool_use block）；tool 结果是 user 消息，content 是数组（tool_result block）。

```go
// anthropicContentBlock 支持 text / tool_use / tool_result 三种块。
type anthropicContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   any             `json:"content,omitempty"` // tool_result 的内容，string 或 []block
}

type anthropicMsg struct {
	Role    string                 `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}
```

**2c. 重写 toAnthropicMsgs 处理 ToolCalls 和 ToolCallID：**
```go
func toAnthropicMsgs(msgs []Message) []anthropicMsg {
	out := make([]anthropicMsg, len(msgs))
	for i, m := range msgs {
		var blocks []anthropicContentBlock
		role := string(m.Role)
		if m.ToolCallID != "" {
			// 工具结果消息：anthropic 用 role=user + tool_result block
			role = "user"
			blocks = []anthropicContentBlock{{
				Type:      "tool_result",
				ToolUseID: m.ToolCallID,
				Content:   m.Content,
			}}
		} else if len(m.ToolCalls) > 0 {
			// assistant 含 tool_use：先 text block（若有），再 tool_use blocks
			if m.Content != "" {
				blocks = append(blocks, anthropicContentBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, anthropicContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: json.RawMessage(tc.Args),
				})
			}
		} else {
			// 纯文本
			blocks = []anthropicContentBlock{{Type: "text", Text: m.Content}}
		}
		out[i] = anthropicMsg{Role: role, Content: blocks}
	}
	return out
}
```

**2d. Stream 里构造 tools 并传入请求：**
```go
var tools []anthropicTool
for _, t := range req.Tools {
	tools = append(tools, anthropicTool{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: t.Schema,
	})
}
// 加入 anthropicRequest 的 Tools 字段
```

**2e. pumpSSE 解析 tool_use（完整实现，非简化）：**

在 pumpSSE 里维护 `map[int]*toolCallAccum`，content_block_start 初始化 id+name，input_json_delta 累加 partial_json，message_stop 前发完整 ToolCallDelta。

```go
type toolCallAccum struct {
	ID   string
	Name string
	Args strings.Builder
}

// 在 pumpSSE 的 evt 结构加：
var evt struct {
	Type          string          `json:"type"`
	Index         int             `json:"index"`
	Delta         json.RawMessage `json:"delta"`
	ContentBlock  json.RawMessage `json:"content_block"`
	Usage         struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Message struct {
		Usage struct {
			InputTokens int `json:"input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// 在 pumpSSE 函数体加（与现有 case 并列）：
toolAccums := map[int]*toolCallAccum{}

case "content_block_start":
	var blk struct {
		Index int `json:"index"`
		ContentBlock struct {
			Type string `json:"type"`
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"content_block"`
	}
	json.Unmarshal([]byte(data), &blk)
	if blk.ContentBlock.Type == "tool_use" {
		toolAccums[blk.Index] = &toolCallAccum{
			ID:   blk.ContentBlock.ID,
			Name: blk.ContentBlock.Name,
		}
	}

case "content_block_delta":
	var d struct {
		Type         string `json:"type"`
		Text         string `json:"text"`
		PartialJSON  string `json:"partial_json"`
	}
	json.Unmarshal(evt.Delta, &d)
	if d.Type == "text_delta" {
		select {
		case ch <- TextDelta{Content: d.Text}:
		case <-ctx.Done():
			return
		}
	} else if d.Type == "input_json_delta" {
		// 累加到对应 index 的 toolCallAccum
		if acc, ok := toolAccums[evt.Index]; ok {
			acc.Args.WriteString(d.PartialJSON)
		}
	}

// 在 message_stop 前，把所有累积的 tool calls 发出：
case "message_stop":
	for i := 0; i < len(toolAccums); i++ { // index 从 0 递增
		if acc, ok := toolAccums[i]; ok {
			select {
			case ch <- ToolCallDelta{ID: acc.ID, Name: acc.Name, ArgsDelta: acc.Args.String()}:
			case <-ctx.Done():
				return
			}
		}
	}
	select {
	case ch <- Done{Usage: Usage{InputTokens: inputTokens, OutputTokens: outputTokens}, StopReason: stopReason}:
	case <-ctx.Done():
	}
	return
```

注意：原 content_block_delta 的 text_delta 处理要合并到上面的新 case 里（替换原代码），不要重复。原 message_stop case 也替换为上面的新版本。evt 结构的 Index 字段要加（原代码没有）。

- [ ] **Step 3: 扩展 openai.go（请求序列化 + 流式解析）**

**3a. 请求结构加 Tools：**
```go
type openaiRequest struct {
	Model         string            `json:"model"`
	Messages      []openaiMsg       `json:"messages"`
	MaxTokens     int               `json:"max_tokens,omitempty"`
	Temperature   float64           `json:"temperature,omitempty"`
	Stream        bool              `json:"stream"`
	StreamOptions *openaiStreamOpts `json:"stream_options,omitempty"`
	Tools         []openaiTool      `json:"tools,omitempty"`
}

type openaiTool struct {
	Type     string         `json:"type"`
	Function openaiToolFunc `json:"function"`
}

type openaiToolFunc struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}
```

**3b. openaiMsg 加 ToolCalls 和 ToolCallID：**
```go
type openaiMsg struct {
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type openaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}
```

**3c. 重写 toOpenAIMsgs 处理 ToolCalls 和 ToolCallID：**
```go
func toOpenAIMsgs(msgs []Message) []openaiMsg {
	out := make([]openaiMsg, len(msgs))
	for i, m := range msgs {
		om := openaiMsg{
			Role:       string(m.Role),
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		for _, tc := range m.ToolCalls {
			oc := openaiToolCall{ID: tc.ID, Type: "function"}
			oc.Function.Name = tc.Name
			oc.Function.Arguments = tc.Args
			om.ToolCalls = append(om.ToolCalls, oc)
		}
		out[i] = om
	}
	return out
}
```

注意：tool 结果消息 role 应为 "tool"（不是 "user"）。agent loop 构造时设 Role=RoleTool。需要在 provider.go 加 `RoleTool Role = "tool"` 常量。

**3d. Stream 里构造 tools：**
```go
var tools []openaiTool
for _, t := range req.Tools {
	tools = append(tools, openaiTool{
		Type: "function",
		Function: openaiToolFunc{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Schema,
		},
	})
}
```

**3e. pumpSSE 解析 tool_calls delta（完整实现）：**

OpenAI 的 tool_calls 是增量式：第一个 chunk 给 id+name，后续 chunk 给 arguments 增量。用 `map[int]*toolCallAccum` 累积。

```go
type toolCallAccum struct {
	ID   string
	Name string
	Args strings.Builder
}

// 在解析 choices[].delta 时加 ToolCalls 字段：
type openaiDelta struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	ToolCalls []struct {
		Index    int    `json:"index"`
		ID       string `json:"id"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_calls"`
}

// 在 pumpSSE 维护：
toolAccums := map[int]*toolCallAccum{}

// 处理 delta：
if len(delta.ToolCalls) > 0 {
	for _, tc := range delta.ToolCalls {
		acc, ok := toolAccums[tc.Index]
		if !ok {
			acc = &toolCallAccum{}
			toolAccums[tc.Index] = acc
		}
		if tc.ID != "" {
			acc.ID = tc.ID
		}
		if tc.Function.Name != "" {
			acc.Name = tc.Function.Name
		}
		if tc.Function.Arguments != "" {
			acc.Args.WriteString(tc.Function.Arguments)
		}
	}
}

// 在收到 [DONE] 或 finish_reason="tool_calls" 时，发所有累积的 tool calls：
for i := 0; i < len(toolAccums); i++ {
	if acc, ok := toolAccums[i]; ok {
		select {
		case ch <- ToolCallDelta{ID: acc.ID, Name: acc.Name, ArgsDelta: acc.Args.String()}:
		case <-ctx.Done():
			return
		}
	}
}
```

- [ ] **Step 4: 写 tool_call 测试**

在 `internal/llm/anthropic_test.go` 加：
```go
func TestAnthropic_Stream_toolCall(t *testing.T) {
	sseBody := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"usage":{"input_tokens":5}}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_1","name":"read"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"file_path\":\"/tmp/a\"}"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":10}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
		``,
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.Write([]byte(sseBody))
	}))
	defer srv.Close()

	a := NewAnthropic("k", srv.URL, "m")
	ch, err := a.Stream(context.Background(), Request{
		Model:    "m",
		Messages: []Message{{Role: RoleUser, Content: "read /tmp/a"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var toolCalls []ToolCall
	for e := range ch {
		if tc, ok := e.(ToolCallDelta); ok {
			toolCalls = append(toolCalls, ToolCall{
				ID:   tc.ID,
				Name: tc.Name,
				Args: tc.ArgsDelta,
			})
		}
	}
	if len(toolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(toolCalls))
	}
	if toolCalls[0].Name != "read" {
		t.Errorf("name = %q, want read", toolCalls[0].Name)
	}
	if toolCalls[0].ID != "tool_1" {
		t.Errorf("id = %q, want tool_1", toolCalls[0].ID)
	}
	if !strings.Contains(toolCalls[0].Args, "/tmp/a") {
		t.Errorf("args = %q, want /tmp/a", toolCalls[0].Args)
	}
}
```

在 `internal/llm/openai_test.go` 加类似测试。

- [ ] **Step 5: 运行测试**

Run:
```bash
go test ./internal/llm/... -v
```
Expected: 所有测试 PASS（含新增 tool_call 测试）

- [ ] **Step 6: Commit**

```bash
git add internal/llm/
git commit -m "feat(m2): extend llm layer with tool_call streaming support"
```

---

### Task 9: agent loop

**Files:**
- Create: `internal/agent/loop.go`
- Create: `internal/agent/loop_test.go`
- Create: `internal/agent/prompt.go`

- [ ] **Step 1: 写 system prompt**

Create `internal/agent/prompt.go`:
```go
package agent

const SystemPrompt = `You are trae, a CLI coding agent. You help users with software engineering tasks.

You have access to tools. Call tools by emitting tool_use blocks. After receiving tool results, continue reasoning or call more tools until the task is complete.

Available tools:
- read: Read a file with line numbers
- write: Write content to a file
- edit: Replace a unique string in a file
- glob: Find files matching a pattern
- grep: Search file contents with regex
- bash: Execute a bash command

Rules:
- Prefer reading before editing.
- Verify changes with bash (e.g. run tests) when applicable.
- Keep responses concise.
- When the task is done, respond with a short summary without calling more tools.`
```

- [ ] **Step 2: 写 agent loop**

Create `internal/agent/loop.go`:
```go
package agent

import (
	"context"
	"fmt"
	"io"

	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/tool"
)

// Agent 多步 think-act 循环。
type Agent struct {
	provider   llm.Provider
	dispatcher *tool.Dispatcher
	tools      []llm.ToolDef
	maxSteps   int
}

type Option func(*Agent)

func WithMaxSteps(n int) Option {
	return func(a *Agent) { a.maxSteps = n }
}

func New(provider llm.Provider, registry *tool.Registry, opts ...Option) *Agent {
	a := &Agent{
		provider:   provider,
		dispatcher: tool.NewDispatcher(registry),
		maxSteps:   20,
	}
	for _, o := range opts {
		o(a)
	}
	// 构造 tool defs
	for _, t := range registry.List() {
		a.tools = append(a.tools, llm.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			Schema:      t.Schema(),
		})
	}
	return a
}

// Event agent 循环产生的事件，供 UI 渲染。
type Event interface{ isAgentEvent() }

type TextEvent struct{ Content string }
type ToolCallEvent struct {
	Name string
	Args string
}
type ToolResultEvent struct {
	Name   string
	Result tool.Result
}
type DoneEvent struct{ Usage llm.Usage }

func (TextEvent) isAgentEvent()       {}
func (ToolCallEvent) isAgentEvent()   {}
func (ToolResultEvent) isAgentEvent() {}
func (DoneEvent) isAgentEvent()       {}

// Run 执行 agent 循环，处理用户输入，通过 events channel 推送事件。
func (a *Agent) Run(ctx context.Context, userInput string, events chan<- Event) error {
	defer close(events)

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: userInput},
	}

	for step := 0; step < a.maxSteps; step++ {
		req := llm.Request{
			Model:    "", // 用 provider 默认
			System:   SystemPrompt,
			Messages: messages,
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
				tc := llm.ToolCall{
					ID:   e.ID,
					Name: e.Name,
					Args: e.ArgsDelta,
				}
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

		// assistant 消息回灌
		assistantMsg := llm.Message{Role: llm.RoleAssistant, Content: textBuf, ToolCalls: toolCalls}
		messages = append(messages, assistantMsg)

		// 没有工具调用，循环结束
		if len(toolCalls) == 0 {
			select {
			case events <- DoneEvent{Usage: usage}:
			case <-ctx.Done():
			}
			return nil
		}

		// 执行工具
		calls := make([]tool.Call, len(toolCalls))
		for i, tc := range toolCalls {
			calls[i] = tool.Call{Name: tc.Name, Args: []byte(tc.Args)}
		}
		results := a.dispatcher.Dispatch(ctx, calls)
		for i, r := range results {
			select {
			case events <- ToolResultEvent{Name: r.Name, Result: r.Result}:
			case <-ctx.Done():
				return ctx.Err()
			}
			// 工具结果回灌：用 ToolCallID 关联，provider 层负责序列化为
			// anthropic tool_result block 或 openai role=tool 消息
			messages = append(messages, llm.Message{
				Role:       llm.RoleTool,
				Content:    r.Result.Content,
				ToolCallID: toolCalls[i].ID,
			})
		}
	}

	select {
	case events <- DoneEvent{Usage: llm.Usage{}}:
	case <-ctx.Done():
	}
	return fmt.Errorf("max steps (%d) exceeded", a.maxSteps)
}

// RenderEvents 消费 agent 事件，渲染到 out（简单文本版）。
func RenderEvents(ctx context.Context, events <-chan Event, out io.Writer) (llm.Usage, error) {
	var usage llm.Usage
	for ev := range events {
		switch e := ev.(type) {
		case TextEvent:
			fmt.Fprint(out, e.Content)
		case ToolCallEvent:
			fmt.Fprintf(out, "\n[tool: %s %s]\n", e.Name, e.Args)
		case ToolResultEvent:
			fmt.Fprintf(out, "[result: %s]\n%s\n", e.Name, e.Result.Content)
		case DoneEvent:
			usage = e.Usage
		}
	}
	return usage, nil
}
```

- [ ] **Step 3: 写集成测试（mock provider）**

Create `internal/agent/loop_test.go`:
```go
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/tool"
)

// mockProvider 按预设脚本返回事件流。
type mockProvider struct {
	scripts [][]llm.StreamEvent // 每次调用返回一个脚本
	calls   int
}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) Stream(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
	script := m.scripts[m.calls]
	m.calls++
	ch := make(chan llm.StreamEvent, len(script))
	go func() {
		defer close(ch)
		for _, e := range script {
			select {
			case <-ctx.Done():
				return
			case ch <- e:
			}
		}
	}()
	return ch, nil
}

func TestAgent_singleTextResponse(t *testing.T) {
	provider := &mockProvider{scripts: [][]llm.StreamEvent{
		{
			llm.TextDelta{Content: "hello"},
			llm.Done{Usage: llm.Usage{InputTokens: 5, OutputTokens: 3}, StopReason: "end_turn"},
		},
	}}
	registry := tool.NewRegistry()
	a := New(provider, registry, WithMaxSteps(5))

	events := make(chan Event, 10)
	go func() {
		a.Run(context.Background(), "hi", events)
	}()

	var buf bytes.Buffer
	usage, err := RenderEvents(context.Background(), consumeChan(events), &buf)
	if err != nil {
		t.Fatal(err)
	}
	if buf.String() != "hello" {
		t.Errorf("output = %q, want hello", buf.String())
	}
	if usage.InputTokens != 5 {
		t.Errorf("usage = %+v", usage)
	}
}

func TestAgent_toolCallLoop(t *testing.T) {
	// step1: 调 read 工具
	// step2: 收到工具结果后，输出文本结束
	provider := &mockProvider{scripts: [][]llm.StreamEvent{
		{
			llm.ToolCallDelta{ID: "t1", Name: "echo", ArgsDelta: `{"msg":"hi"}`},
			llm.Done{StopReason: "tool_use"},
		},
		{
			llm.TextDelta{Content: "done"},
			llm.Done{StopReason: "end_turn"},
		},
	}}

	echo := testEchoTool{}
	registry := tool.NewRegistry(echo)
	a := New(provider, registry, WithMaxSteps(5))

	events := make(chan Event, 20)
	go func() {
		a.Run(context.Background(), "call echo", events)
	}()

	var buf bytes.Buffer
	RenderEvents(context.Background(), consumeChan(events), &buf)

	out := buf.String()
	if !strings.Contains(out, "echo") {
		t.Errorf("missing tool call: %s", out)
	}
	if !strings.Contains(out, "done") {
		t.Errorf("missing final text: %s", out)
	}
}

// testEchoTool 测试用 echo 工具
type testEchoTool struct{}

func (testEchoTool) Name() string             { return "echo" }
func (testEchoTool) Description() string      { return "echo" }
func (testEchoTool) Schema() json.RawMessage  { return json.RawMessage(`{}`) }
func (testEchoTool) Run(ctx context.Context, args json.RawMessage) tool.Result {
	var v struct{ Msg string `json:"msg"` }
	json.Unmarshal(args, &v)
	return tool.Result{Content: v.Msg}
}

// consumeChan 把 chan Event 转为 <-chan Event
func consumeChan(ch chan Event) <-chan Event {
	return ch
}
```

- [ ] **Step 4: 运行测试**

Run:
```bash
go test ./internal/agent/... -v
```
Expected: 2 个测试 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/
git commit -m "feat(m2): add agent loop with think-act cycle"
```

---

### Task 10: run 命令接入 agent

**Files:**
- Modify: `internal/cli/run.go`
- Modify: `internal/config/config.go` — 加 MaxSteps 字段
- Modify: `internal/cli/run_test.go`

- [ ] **Step 1: 扩展 config**

在 Config struct 加：
```go
type Config struct {
	DefaultProvider string                    `yaml:"default_provider"`
	Providers       map[string]ProviderConfig `yaml:"model_providers"`
	LogLevel        string                    `yaml:"log_level"`
	SystemPrompt    string                    `yaml:"system_prompt"`
	MaxSteps        int                       `yaml:"max_steps"`
}
```

- [ ] **Step 2: 修改 run.go**

把 run 命令的 RunE 改为构造 agent 并运行：
```go
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
		return fmt.Errorf("no provider specified")
	}

	provCfg, ok := cfg.Providers[providerName]
	if !ok {
		return fmt.Errorf("provider %q not found", providerName)
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

	// 构造工具
	registry := tool.NewRegistry(
		tool.NewRead(),
		tool.NewWrite(),
		tool.NewEdit(),
		tool.NewGlob(),
		tool.NewGrep(),
		tool.NewBash(120 * time.Second),
	)

	maxSteps := cfg.MaxSteps
	if maxSteps == 0 {
		maxSteps = 20
	}
	a := agent.New(llmProvider, registry, agent.WithMaxSteps(maxSteps))

	events := make(chan agent.Event, 64)
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Run(cmd.Context(), args[0], events)
	}()

	usage, renderErr := agent.RenderEvents(cmd.Context(), events, cmd.OutOrStdout())
	if renderErr != nil {
		return renderErr
	}
	if err := <-errCh; err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "\n[tokens: in=%d out=%d]\n", usage.InputTokens, usage.OutputTokens)
	return nil
}
```

注意：需要 import `github.com/bytedance/trae-agent/internal/agent` 和 `github.com/bytedance/trae-agent/internal/tool`。

- [ ] **Step 3: 更新 run_test.go**

原 `TestRunCommand_streamsToStdout` 用的是单轮 mock SSE，现在走 agent loop，mock SSE 需要返回不带 tool_call 的纯文本响应。原测试的 SSE body 只发 text_delta + done（stop_reason=end_turn），应该能直接通过 agent loop（无 tool call，单步结束）。

验证测试是否仍 PASS，如失败则调整。

- [ ] **Step 4: 运行测试**

Run:
```bash
go test ./internal/cli/... -v
```
Expected: 全部 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cli/run.go internal/cli/run_test.go internal/config/config.go
git commit -m "feat(m2): integrate agent loop into run command"
```

---

### Task 11: 端到端验收

**Files:** 无新增，验证整体

- [ ] **Step 1: 全量测试**

Run:
```bash
make test
```
Expected: 全部 PASS（M0 9 + M1 18 + M2 新增约 30 = 57 个测试）

- [ ] **Step 2: vet 检查**

Run:
```bash
make vet
```
Expected: 无错误

- [ ] **Step 3: 构建验证**

Run:
```bash
make build && ./bin/trae version
```

- [ ] **Step 4: 工具闭环验证（mock LLM）**

由于没有真实 API key，用 mock server 验证 agent loop 闭环。创建一个 mock SSE server，返回 tool_call 让 agent 调 read 工具，再返回结束文本。

```bash
# 写一个临时测试脚本或用 go test 跑集成测试
go test ./internal/agent/... -run TestAgent_toolCallLoop -v
```

- [ ] **Step 5: 真实 API 验证（如有 key）**

```bash
# 配置真实 provider
mkdir -p ~/.trae
cat > ~/.trae/config.yaml <<'EOF'
default_provider: anthropic
model_providers:
  anthropic:
    api_key: $ANTHROPIC_API_KEY
    default_model: claude-3-5-sonnet-20241022
EOF

# 测试多步任务
./bin/trae run "在 /tmp 创建 hello.txt 写入 'hello world'，然后读取它确认内容"
```

- [ ] **Step 6: 验证 max_steps 限制**

```bash
mkdir -p /tmp/trae-m2-test/.trae
cat > /tmp/trae-m2-test/.trae/config.yaml <<'EOF'
default_provider: anthropic
max_steps: 2
model_providers:
  anthropic:
    api_key: test
    base_url: http://localhost:9999
    default_model: m
EOF
HOME=/tmp/trae-m2-test ./bin/trae run "hi" 2>&1 | head -5
rm -rf /tmp/trae-m2-test
```

- [ ] **Step 7: 推送**

```bash
git push origin solo-go
```

---

## M2 完成标准

- [ ] 6 个工具实现（read/write/edit/glob/grep/bash），各有单测
- [ ] dispatcher 并行调度，测试验证并行性
- [ ] LLM 层支持 tool_call 流式（anthropic + openai）
- [ ] agent loop 多步 think-act，mock provider 集成测试通过
- [ ] `trae run "prompt"` 走 agent loop（非单轮）
- [ ] `make test` 全部 PASS
- [ ] `make vet` 无错误
- [ ] max_steps 限制生效

## 留到 M3 的项

- 交互式 REPL（斜杠命令、流式渲染、Ctrl+C）
- tool call 卡片式 UI 渲染
- 权限门（M6）
- compact（M4）
