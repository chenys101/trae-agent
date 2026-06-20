# M1 LLM 打通实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `trae run "你好"` 能流式打印 LLM 回复，支持 Anthropic + OpenAI 两个 provider，中途断网自动重试 3 次，token usage 统计正确。

**Architecture:** Provider 接口统一抽象流式输出，anthropic/openai 各自实现 SSE 解析与格式适配。headless runner 消费 StreamEvent channel 渲染到 stdout。retry 包装器用指数退避重试可重试错误。

**Tech Stack:** net/http + bufio.Scanner（SSE 解析，不引第三方 SDK）、context、channel

---

## 文件结构

- Create: `internal/llm/provider.go` — Provider 接口、StreamEvent、Request/Usage/Message
- Create: `internal/llm/provider_test.go` — 接口契约测试 helpers
- Create: `internal/llm/anthropic.go` — Anthropic SSE 实现
- Create: `internal/llm/anthropic_test.go` — httptest mock SSE
- Create: `internal/llm/openai.go` — OpenAI SSE 实现
- Create: `internal/llm/openai_test.go` — httptest mock SSE
- Create: `internal/llm/retry.go` — 指数退避重试包装器
- Create: `internal/llm/retry_test.go`
- Create: `internal/headless/runner.go` — 单轮流式打印
- Create: `internal/headless/runner_test.go`
- Create: `internal/cli/run.go` — `trae run` 子命令
- Create: `internal/cli/run_test.go`
- Modify: `internal/cli/root.go` — 注册 run 命令、接入 logger
- Modify: `internal/config/config.go` — 补充 SystemPrompt 字段（可选）

---

### Task 1: Provider 接口与核心类型

**Files:**
- Create: `internal/llm/provider.go`
- Create: `internal/llm/provider_test.go`

- [ ] **Step 1: 写核心类型**

Create `internal/llm/provider.go`:
```go
package llm

import (
	"context"
	"encoding/json"
)

// Role 消息角色。
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message 单条消息。M1 只用 Content 文本，ToolCalls 留 M2。
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// Request LLM 请求。
type Request struct {
	Model       string
	Messages    []Message
	System      string // 系统提示（Anthropic 单独字段，OpenAI 拼到 messages 头）
	MaxTokens   int
	Temperature float64
}

// Usage token 用量统计。
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// StreamEvent 流式事件接口。M1 只需 TextDelta/Done/Error，ToolCallDelta 留 M2。
type StreamEvent interface{ isStreamEvent() }

// TextDelta 文本增量。
type TextDelta struct {
	Content string
}

func (TextDelta) isStreamEvent() {}

// Done 流结束，携带 usage 和停止原因。
type Done struct {
	Usage      Usage
	StopReason string
}

func (Done) isStreamEvent() {}

// Error 流中错误（含可重试错误）。
type Error struct {
	Err error
}

func (Error) isStreamEvent() {}

// Provider LLM 供应商抽象。
type Provider interface {
	Name() string
	Stream(ctx context.Context, req Request) (<-chan StreamEvent, error)
}

// NewProvider 按 provider 名构造实例（M1 只支持 anthropic/openai）。
func NewProvider(name, apiKey, baseURL, defaultModel string) (Provider, error) {
	switch name {
	case "anthropic":
		return NewAnthropic(apiKey, baseURL, defaultModel), nil
	case "openai":
		return NewOpenAI(apiKey, baseURL, defaultModel), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", name)
	}
}
```

注意：`NewProvider` 用到 `fmt`，import 要加。修正版 import：
```go
import (
	"context"
	"fmt"
)
```
（`encoding/json` 暂时不用，去掉。）

- [ ] **Step 2: 写接口契约测试**

Create `internal/llm/provider_test.go`:
```go
package llm

import (
	"context"
	"errors"
	"testing"
)

// fakeProvider 用于测试上层消费逻辑。
type fakeProvider struct {
	events []StreamEvent
}

func (f *fakeProvider) Name() string { return "fake" }
func (f *fakeProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, len(f.events))
	go func() {
		defer close(ch)
		for _, e := range f.events {
			select {
			case <-ctx.Done():
				return
			case ch <- e:
			}
		}
	}()
	return ch, nil
}

func TestStreamEvent_implements(t *testing.T) {
	var e StreamEvent
	e = TextDelta{Content: "hi"}
	e = Done{Usage: Usage{InputTokens: 1, OutputTokens: 2}}
	e = Error{Err: errors.New("boom")}
	_ = e
}

func TestFakeProvider_drainsEvents(t *testing.T) {
	fp := &fakeProvider{events: []StreamEvent{
		TextDelta{Content: "a"},
		TextDelta{Content: "b"},
		Done{Usage: Usage{InputTokens: 10, OutputTokens: 5}},
	}}
	ch, err := fp.Stream(context.Background(), Request{})
	if err != nil {
		t.Fatal(err)
	}
	var got []StreamEvent
	for e := range ch {
		got = append(got, e)
	}
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
}

func TestNewProvider_unknown(t *testing.T) {
	_, err := NewProvider("unknown", "k", "", "m")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}
```

- [ ] **Step 3: 运行测试**

Run:
```bash
go test ./internal/llm/... -v
```
Expected: PASS（3 个测试）。`NewAnthropic`/`NewOpenAI` 还未定义会导致编译错误——所以 Task 1 先不调用 NewProvider 的 anthropic/openai 分支，或先建空壳。

**修正方案**：Task 1 的 `provider.go` 里 `NewProvider` 暂时只返回 error for all（不引用 NewAnthropic/NewOpenAI），Task 2/3 实现后再回填。或者 Task 1 直接建 anthropic.go/openai.go 空壳。

采用空壳方案：Task 1 同时创建 anthropic.go 和 openai.go 的最小空壳（只实现 Name()），Task 2/3 再填充 Stream。

修正 `provider.go` 的 `NewProvider`：
```go
func NewProvider(name, apiKey, baseURL, defaultModel string) (Provider, error) {
	switch name {
	case "anthropic":
		return NewAnthropic(apiKey, baseURL, defaultModel), nil
	case "openai":
		return NewOpenAI(apiKey, baseURL, defaultModel), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", name)
	}
}
```

Create `internal/llm/anthropic.go`（空壳）:
```go
package llm

import "context"

type Anthropic struct {
	apiKey       string
	baseURL      string
	defaultModel string
}

func NewAnthropic(apiKey, baseURL, defaultModel string) *Anthropic {
	return &Anthropic{apiKey: apiKey, baseURL: baseURL, defaultModel: defaultModel}
}

func (a *Anthropic) Name() string { return "anthropic" }

func (a *Anthropic) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	return nil, fmt.Errorf("anthropic.Stream not implemented")
}
```

Create `internal/llm/openai.go`（空壳）:
```go
package llm

import "context"

type OpenAI struct {
	apiKey       string
	baseURL      string
	defaultModel string
}

func NewOpenAI(apiKey, baseURL, defaultModel string) *OpenAI {
	return &OpenAI{apiKey: apiKey, baseURL: baseURL, defaultModel: defaultModel}
}

func (o *OpenAI) Name() string { return "openai" }

func (o *OpenAI) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	return nil, fmt.Errorf("openai.Stream not implemented")
}
```

注意：anthropic.go 和 openai.go 空壳用到了 `fmt`，import 要加。

- [ ] **Step 4: Commit**

```bash
git add internal/llm/
git commit -m "feat(m1): add provider interface and core types"
```

---

### Task 2: Anthropic Provider（SSE 流式）

**Files:**
- Create: `internal/llm/anthropic_test.go`
- Modify: `internal/llm/anthropic.go`

- [ ] **Step 1: 写失败测试（httptest mock SSE）**

Create `internal/llm/anthropic_test.go`:
```go
package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropic_Stream_text(t *testing.T) {
	// 模拟 Anthropic SSE 响应
	sseBody := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":10}}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":" world"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
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

	a := NewAnthropic("test-key", srv.URL, "claude-3-5-sonnet-20241022")
	ch, err := a.Stream(context.Background(), Request{
		Model:    "claude-3-5-sonnet-20241022",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	var text string
	var usage Usage
	var stopReason string
	for e := range ch {
		switch ev := e.(type) {
		case TextDelta:
			text += ev.Content
		case Done:
			usage = ev.Usage
			stopReason = ev.StopReason
		case Error:
			t.Fatalf("stream error: %v", ev.Err)
		}
	}
	if text != "Hello world" {
		t.Errorf("text = %q, want 'Hello world'", text)
	}
	if usage.InputTokens != 10 || usage.OutputTokens != 5 {
		t.Errorf("usage = %+v, want {10, 5}", usage)
	}
	if stopReason != "end_turn" {
		t.Errorf("stopReason = %q, want end_turn", stopReason)
	}
}

func TestAnthropic_Stream_apiError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	}))
	defer srv.Close()

	a := NewAnthropic("bad-key", srv.URL, "claude-3-5-sonnet-20241022")
	_, err := a.Stream(context.Background(), Request{
		Model:    "claude-3-5-sonnet-20241022",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
}

func TestAnthropic_Stream_contextCancel(t *testing.T) {
	// 慢响应，测试 context 取消
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)
		flusher.Flush()
		// 不写任何数据，阻塞
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	a := NewAnthropic("k", srv.URL, "m")
	ch, err := a.Stream(ctx, Request{Model: "m", Messages: []Message{{Role: RoleUser, Content: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	// channel 应该关闭
	for range ch {
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
go test ./internal/llm/... -run TestAnthropic -v
```
Expected: FAIL，`anthropic.Stream not implemented`

- [ ] **Step 3: 实现 Anthropic SSE**

Overwrite `internal/llm/anthropic.go`:
```go
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Anthropic struct {
	apiKey       string
	baseURL      string
	defaultModel string
}

func NewAnthropic(apiKey, baseURL, defaultModel string) *Anthropic {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	return &Anthropic{apiKey: apiKey, baseURL: baseURL, defaultModel: defaultModel}
}

func (a *Anthropic) Name() string { return "anthropic" }

// anthropicRequest Anthropic Messages API 请求体。
type anthropicRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	Messages    []anthropicMsg  `json:"messages"`
	System      string          `json:"system,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	Stream      bool            `json:"stream"`
}

type anthropicMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (a *Anthropic) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	model := req.Model
	if model == "" {
		model = a.defaultModel
	}
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	body, err := json.Marshal(anthropicRequest{
		Model:       model,
		MaxTokens:   maxTokens,
		Messages:    toAnthropicMsgs(req.Messages),
		System:      req.System,
		Temperature: req.Temperature,
		Stream:      true,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", a.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic api error (status %d): %s", resp.StatusCode, errBody)
	}

	ch := make(chan StreamEvent, 16)
	go a.pumpSSE(ctx, resp.Body, ch)
	return ch, nil
}

func (a *Anthropic) pumpSSE(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
	defer body.Close()
	defer close(ch)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var inputTokens, outputTokens int
	var stopReason string

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- Error{Err: ctx.Err()}
			return
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "" || data == "[DONE]" {
			continue
		}

		var evt struct {
			Type  string          `json:"type"`
			Delta json.RawMessage `json:"delta"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			Message struct {
				Usage struct {
					InputTokens int `json:"input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			continue
		}

		switch evt.Type {
		case "message_start":
			inputTokens = evt.Message.Usage.InputTokens
		case "content_block_delta":
			var d struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(evt.Delta, &d); err == nil && d.Type == "text_delta" {
				select {
				case ch <- TextDelta{Content: d.Text}:
				case <-ctx.Done():
					return
				}
			}
		case "message_delta":
			if evt.Usage.OutputTokens > 0 {
				outputTokens = evt.Usage.OutputTokens
			}
			var d struct {
				StopReason string `json:"stop_reason"`
			}
			json.Unmarshal(evt.Delta, &d)
			if d.StopReason != "" {
				stopReason = d.StopReason
			}
		case "message_stop":
			select {
			case ch <- Done{Usage: Usage{InputTokens: inputTokens, OutputTokens: outputTokens}, StopReason: stopReason}:
			case <-ctx.Done():
			}
			return
		case "error":
			var e struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			json.Unmarshal([]byte(data), &e)
			select {
			case ch <- Error{Err: fmt.Errorf("anthropic stream error: %s", e.Error.Message)}:
			case <-ctx.Done():
			}
			return
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		select {
		case ch <- Error{Err: fmt.Errorf("read sse: %w", err)}:
		case <-ctx.Done():
		}
	}
}

func toAnthropicMsgs(msgs []Message) []anthropicMsg {
	out := make([]anthropicMsg, len(msgs))
	for i, m := range msgs {
		out[i] = anthropicMsg{Role: string(m.Role), Content: m.Content}
	}
	return out
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:
```bash
go test ./internal/llm/... -run TestAnthropic -v
```
Expected: 3 个测试 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/llm/anthropic.go internal/llm/anthropic_test.go
git commit -m "feat(m1): implement anthropic provider with SSE streaming"
```

---

### Task 3: OpenAI Provider（SSE 流式）

**Files:**
- Create: `internal/llm/openai_test.go`
- Modify: `internal/llm/openai.go`

- [ ] **Step 1: 写失败测试**

Create `internal/llm/openai_test.go`:
```go
package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAI_Stream_text(t *testing.T) {
	// 模拟 OpenAI SSE 响应
	sseBody := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}`,
		``,
		`data: {"choices":[{"delta":{"content":" world"},"finish_reason":null}]}`,
		``,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`,
		``,
		`data: [DONE]`,
		``,
		``,
	}, "\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.Write([]byte(sseBody))
	}))
	defer srv.Close()

	o := NewOpenAI("test-key", srv.URL, "gpt-4o")
	ch, err := o.Stream(context.Background(), Request{
		Model:    "gpt-4o",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	var text string
	var usage Usage
	var stopReason string
	for e := range ch {
		switch ev := e.(type) {
		case TextDelta:
			text += ev.Content
		case Done:
			usage = ev.Usage
			stopReason = ev.StopReason
		case Error:
			t.Fatalf("stream error: %v", ev.Err)
		}
	}
	if text != "Hello world" {
		t.Errorf("text = %q, want 'Hello world'", text)
	}
	if usage.InputTokens != 10 || usage.OutputTokens != 5 {
		t.Errorf("usage = %+v, want {10, 5}", usage)
	}
	if stopReason != "stop" {
		t.Errorf("stopReason = %q, want stop", stopReason)
	}
}

func TestOpenAI_Stream_apiError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
	}))
	defer srv.Close()

	o := NewOpenAI("bad-key", srv.URL, "gpt-4o")
	_, err := o.Stream(context.Background(), Request{
		Model:    "gpt-4o",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
}

func TestOpenAI_Stream_systemPrompt(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	o := NewOpenAI("k", srv.URL, "gpt-4o")
	_, _ = o.Stream(context.Background(), Request{
		Model:    "gpt-4o",
		System:   "You are helpful.",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})

	// OpenAI 把 system 拼到 messages 头
	if !strings.Contains(gotBody, `"role":"system"`) {
		t.Errorf("request body missing system message: %s", gotBody)
	}
	if !strings.Contains(gotBody, "You are helpful.") {
		t.Errorf("request body missing system content: %s", gotBody)
	}
}
```

注意：`TestOpenAI_Stream_systemPrompt` 用到 `io`，import 要加 `io`。

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
go test ./internal/llm/... -run TestOpenAI -v
```
Expected: FAIL，`openai.Stream not implemented`

- [ ] **Step 3: 实现 OpenAI SSE**

Overwrite `internal/llm/openai.go`:
```go
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type OpenAI struct {
	apiKey       string
	baseURL      string
	defaultModel string
}

func NewOpenAI(apiKey, baseURL, defaultModel string) *OpenAI {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	return &OpenAI{apiKey: apiKey, baseURL: baseURL, defaultModel: defaultModel}
}

func (o *OpenAI) Name() string { return "openai" }

type openaiRequest struct {
	Model       string         `json:"model"`
	Messages    []openaiMsg    `json:"messages"`
	MaxTokens   int            `json:"max_tokens,omitempty"`
	Temperature float64        `json:"temperature,omitempty"`
	Stream      bool           `json:"stream"`
	StreamOptions *openaiStreamOpts `json:"stream_options,omitempty"`
}

type openaiMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiStreamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

func (o *OpenAI) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	model := req.Model
	if model == "" {
		model = o.defaultModel
	}

	msgs := make([]openaiMsg, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, openaiMsg{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, openaiMsg{Role: string(m.Role), Content: m.Content})
	}

	bodyReq := openaiRequest{
		Model:       model,
		Messages:    msgs,
		Temperature: req.Temperature,
		Stream:      true,
		StreamOptions: &openaiStreamOpts{IncludeUsage: true},
	}
	if req.MaxTokens > 0 {
		bodyReq.MaxTokens = req.MaxTokens
	}

	body, err := json.Marshal(bodyReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai api error (status %d): %s", resp.StatusCode, errBody)
	}

	ch := make(chan StreamEvent, 16)
	go o.pumpSSE(ctx, resp.Body, ch)
	return ch, nil
}

func (o *OpenAI) pumpSSE(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
	defer body.Close()
	defer close(ch)

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var inputTokens, outputTokens int
	var stopReason string

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- Error{Err: ctx.Err()}
			return
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			select {
			case ch <- Done{Usage: Usage{InputTokens: inputTokens, OutputTokens: outputTokens}, StopReason: stopReason}:
			case <-ctx.Done():
			}
			return
		}

		var evt struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			continue
		}

		if evt.Usage != nil {
			inputTokens = evt.Usage.PromptTokens
			outputTokens = evt.Usage.CompletionTokens
		}

		for _, choice := range evt.Choices {
			if choice.Delta.Content != "" {
				select {
				case ch <- TextDelta{Content: choice.Delta.Content}:
				case <-ctx.Done():
					return
				}
			}
			if choice.FinishReason != "" {
				stopReason = choice.FinishReason
			}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		select {
		case ch <- Error{Err: fmt.Errorf("read sse: %w", err)}:
		case <-ctx.Done():
		}
		return
	}

	// 流结束但没收到 [DONE]，也发 Done
	select {
	case ch <- Done{Usage: Usage{InputTokens: inputTokens, OutputTokens: outputTokens}, StopReason: stopReason}:
	case <-ctx.Done():
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:
```bash
go test ./internal/llm/... -run TestOpenAI -v
```
Expected: 3 个测试 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/llm/openai.go internal/llm/openai_test.go
git commit -m "feat(m1): implement openai provider with SSE streaming"
```

---

### Task 4: Retry 包装器

**Files:**
- Create: `internal/llm/retry.go`
- Create: `internal/llm/retry_test.go`

- [ ] **Step 1: 写失败测试**

Create `internal/llm/retry_test.go`:
```go
package llm

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetry_successOnSecondAttempt(t *testing.T) {
	var calls int32
	p := &retryProvider{
		inner: &fakeFailingProvider{
			failCount: 1,
			calls:     &calls,
			events:    []StreamEvent{TextDelta{Content: "ok"}, Done{}},
		},
		maxRetries: 3,
		baseDelay:  time.Millisecond,
	}
	ch, err := p.Stream(context.Background(), Request{})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	var got []StreamEvent
	for e := range ch {
		got = append(got, e)
	}
	if len(got) != 2 {
		t.Errorf("got %d events, want 2", len(got))
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestRetry_exhausted(t *testing.T) {
	var calls int32
	p := &retryProvider{
		inner: &fakeFailingProvider{
			failCount: 99, // 永远失败
			calls:     &calls,
			events:    []StreamEvent{Done{}},
		},
		maxRetries: 3,
		baseDelay:  time.Millisecond,
	}
	_, err := p.Stream(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestRetry_nonRetryableError(t *testing.T) {
	var calls int32
	p := &retryProvider{
		inner: &fakeFailingProvider{
			failCount:  99,
			calls:      &calls,
			events:     []StreamEvent{Done{}},
			errToReturn: errors.New("400 Bad Request"),
		},
		maxRetries: 3,
		baseDelay:  time.Millisecond,
	}
	_, err := p.Stream(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected error")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("calls = %d, want 1 (non-retryable)", calls)
	}
}

// fakeFailingProvider 前 failCount 次返回可重试错误，之后返回成功流。
type fakeFailingProvider struct {
	failCount   int32
	calls       *int32
	events      []StreamEvent
	errToReturn error // 默认 nil 表示可重试错误
}

func (f *fakeFailingProvider) Name() string { return "fake-failing" }
func (f *fakeFailingProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	n := atomic.AddInt32(f.calls, 1)
	if int32(n) <= f.failCount {
		if f.errToReturn != nil {
			return nil, f.errToReturn
		}
		return nil, errors.New("503 Service Unavailable")
	}
	ch := make(chan StreamEvent, len(f.events))
	go func() {
		defer close(ch)
		for _, e := range f.events {
			ch <- e
		}
	}()
	return ch, nil
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
go test ./internal/llm/... -run TestRetry -v
```
Expected: FAIL，`undefined: retryProvider`

- [ ] **Step 3: 实现 retry**

Create `internal/llm/retry.go`:
```go
package llm

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

// RetryableProvider 包装一个 Provider，对可重试错误做指数退避重试。
type RetryableProvider struct {
	inner      Provider
	maxRetries int
	baseDelay  time.Duration
}

// NewRetryable 构造重试包装器。
func NewRetryable(inner Provider, maxRetries int, baseDelay time.Duration) *RetryableProvider {
	if maxRetries < 1 {
		maxRetries = 1
	}
	if baseDelay <= 0 {
		baseDelay = 500 * time.Millisecond
	}
	return &RetryableProvider{inner: inner, maxRetries: maxRetries, baseDelay: baseDelay}
}

func (r *RetryableProvider) Name() string { return r.inner.Name() }

func (r *RetryableProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	var lastErr error
	for attempt := 0; attempt < r.maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(math.Pow(2, float64(attempt-1))) * r.baseDelay
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		ch, err := r.inner.Stream(ctx, req)
		if err == nil {
			return ch, nil
		}
		lastErr = err
		if !isRetryable(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("after %d retries: %w", r.maxRetries, lastErr)
}

// isRetryable 判断错误是否可重试：5xx、429、网络错误。
func isRetryable(err error) bool {
	msg := err.Error()
	if strings.Contains(msg, "status 5") || strings.Contains(msg, "503") ||
		strings.Contains(msg, "502") || strings.Contains(msg, "500") ||
		strings.Contains(msg, "429") || strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection") || strings.Contains(msg, "EOF") {
		return true
	}
	return false
}
```

注意：测试里用的是 `retryProvider`，实现里是 `RetryableProvider`。统一用 `RetryableProvider`，测试里改用 `NewRetryable`。

修正测试 `retry_test.go`，把 `&retryProvider{...}` 改为 `NewRetryable(inner, 3, time.Millisecond)`：
```go
func TestRetry_successOnSecondAttempt(t *testing.T) {
	var calls int32
	inner := &fakeFailingProvider{
		failCount: 1,
		calls:     &calls,
		events:    []StreamEvent{TextDelta{Content: "ok"}, Done{}},
	}
	p := NewRetryable(inner, 3, time.Millisecond)
	ch, err := p.Stream(context.Background(), Request{})
	// ... 其余不变
```

同样修正另外两个测试。

- [ ] **Step 4: 运行测试确认通过**

Run:
```bash
go test ./internal/llm/... -run TestRetry -v
```
Expected: 3 个测试 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/llm/retry.go internal/llm/retry_test.go
git commit -m "feat(m1): add retry wrapper with exponential backoff"
```

---

### Task 5: Headless Runner

**Files:**
- Create: `internal/headless/runner.go`
- Create: `internal/headless/runner_test.go`

- [ ] **Step 1: 写失败测试**

Create `internal/headless/runner_test.go`:
```go
package headless

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/bytedance/trae-agent/internal/llm"
)

func TestRun_streamsTextToWriter(t *testing.T) {
	provider := &fakeProvider{
		events: []llm.StreamEvent{
			llm.TextDelta{Content: "Hello"},
			llm.TextDelta{Content: " world"},
			llm.Done{Usage: llm.Usage{InputTokens: 10, OutputTokens: 5}},
		},
	}
	var buf bytes.Buffer
	usage, err := Run(context.Background(), provider, llm.Request{
		Model:    "test",
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}},
	}, &buf)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if buf.String() != "Hello world" {
		t.Errorf("output = %q, want 'Hello world'", buf.String())
	}
	if usage.InputTokens != 10 || usage.OutputTokens != 5 {
		t.Errorf("usage = %+v, want {10, 5}", usage)
	}
}

func TestRun_streamError(t *testing.T) {
	provider := &fakeProvider{
		events: []llm.StreamEvent{
			llm.TextDelta{Content: "partial"},
			llm.Error{Err: errors.New("stream broke")},
		},
	}
	var buf bytes.Buffer
	_, err := Run(context.Background(), provider, llm.Request{}, &buf)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if buf.String() != "partial" {
		t.Errorf("partial output = %q, want 'partial'", buf.String())
	}
}

func TestRun_providerError(t *testing.T) {
	provider := &fakeProvider{streamErr: errors.New("provider init failed")}
	var buf bytes.Buffer
	_, err := Run(context.Background(), provider, llm.Request{}, &buf)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

type fakeProvider struct {
	events    []llm.StreamEvent
	streamErr error
}

func (f *fakeProvider) Name() string { return "fake" }
func (f *fakeProvider) Stream(ctx context.Context, req llm.Request) (<-chan llm.StreamEvent, error) {
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	ch := make(chan llm.StreamEvent, len(f.events))
	go func() {
		defer close(ch)
		for _, e := range f.events {
			select {
			case <-ctx.Done():
				return
			case ch <- e:
			}
		}
	}()
	return ch, nil
}

var _ llm.Provider = (*fakeProvider)(nil)
var _ io.Writer = (*bytes.Buffer)(nil)
```

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
go test ./internal/headless/... -v
```
Expected: FAIL，`undefined: Run`

- [ ] **Step 3: 实现 runner**

Create `internal/headless/runner.go`:
```go
package headless

import (
	"context"
	"fmt"
	"io"

	"github.com/bytedance/trae-agent/internal/llm"
)

// Run 执行单轮 LLM 调用，流式写入 out，返回 usage 统计。
func Run(ctx context.Context, provider llm.Provider, req llm.Request, out io.Writer) (llm.Usage, error) {
	ch, err := provider.Stream(ctx, req)
	if err != nil {
		return llm.Usage{}, fmt.Errorf("provider stream: %w", err)
	}

	var usage llm.Usage
	for event := range ch {
		switch e := event.(type) {
		case llm.TextDelta:
			if _, err := io.WriteString(out, e.Content); err != nil {
				return usage, fmt.Errorf("write output: %w", err)
			}
		case llm.Done:
			usage = e.Usage
		case llm.Error:
			return usage, e.Err
		}
	}
	return usage, nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:
```bash
go test ./internal/headless/... -v
```
Expected: 3 个测试 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/headless/
git commit -m "feat(m1): add headless runner for single-turn streaming"
```

---

### Task 6: `trae run` 子命令 + 接入 logger/config

**Files:**
- Create: `internal/cli/run.go`
- Create: `internal/cli/run_test.go`
- Modify: `internal/cli/root.go` — 注册 run、PersistentPreRunE 初始化 logger
- Modify: `internal/config/config.go` — 补充 SystemPrompt 字段

- [ ] **Step 1: 扩展 config**

Modify `internal/config/config.go`，在 Config struct 加字段：
```go
type Config struct {
	DefaultProvider string                    `yaml:"default_provider"`
	Providers       map[string]ProviderConfig `yaml:"model_providers"`
	LogLevel        string                    `yaml:"log_level"`
	SystemPrompt    string                    `yaml:"system_prompt"`
}
```

- [ ] **Step 2: 写 run 命令测试**

Create `internal/cli/run_test.go`:
```go
package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bytedance/trae-agent/internal/llm"
)

func TestRunCommand_streamsToStdout(t *testing.T) {
	// mock anthropic SSE
	sseBody := strings.Join([]string{
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hi"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
		``,
		`event: message_start`,
		`data: {"type":"message_start","message":{"usage":{"input_tokens":5}}}`,
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

	// 写用户配置指向 mock server
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	cfgPath := filepath.Join(tmp, ".trae", "config.yaml")
	os.MkdirAll(filepath.Dir(cfgPath), 0o755)
	os.WriteFile(cfgPath, []byte(`
default_provider: anthropic
model_providers:
  anthropic:
    api_key: test-key
    base_url: `+srv.URL+`
    default_model: claude-3-5-sonnet-20241022
`), 0o644)

	var buf bytes.Buffer
	cmd := NewRunCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"hello"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Hi") {
		t.Errorf("output missing 'Hi', got: %s", buf.String())
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

Run:
```bash
go test ./internal/cli/... -run TestRunCommand -v
```
Expected: FAIL，`undefined: NewRunCmd`

- [ ] **Step 4: 实现 run 命令**

Create `internal/cli/run.go`:
```go
package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/bytedance/trae-agent/internal/config"
	"github.com/bytedance/trae-agent/internal/headless"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/spf13/cobra"
)

func NewRunCmd() *cobra.Command {
	var providerFlag string
	var modelFlag string
	cmd := &cobra.Command{
		Use:   "run [prompt]",
		Short: "Run a single-turn prompt and stream the response",
		Args:  cobra.MinimumNArgs(1),
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

			provider, err := llm.NewProvider(provCfg.Provider, provCfg.APIKey, provCfg.BaseURL, provCfg.DefaultModel)
			if err != nil {
				return err
			}
			provider = llm.NewRetryable(provider, 3, 500*1024*1024*1024) // baseDelay 用合理值
			// 修正：baseDelay 应该是 time.Duration
			// 改为：llm.NewRetryable(provider, 3, 500*time.Millisecond)

			model := modelFlag
			if model == "" {
				model = provCfg.DefaultModel
			}

			req := llm.Request{
				Model:    model,
				System:   cfg.SystemPrompt,
				Messages: []llm.Message{{Role: llm.RoleUser, Content: args[0]}},
			}
			if provCfg.MaxTokens > 0 {
				req.MaxTokens = provCfg.MaxTokens
			}
			req.Temperature = provCfg.Temperature

			usage, err := headless.Run(cmd.Context(), provider, req, os.Stdout)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "\n[tokens: in=%d out=%d]\n", usage.InputTokens, usage.OutputTokens)
			return nil
		},
	}
	cmd.Flags().StringVar(&providerFlag, "provider", "", "LLM provider (anthropic/openai)")
	cmd.Flags().StringVar(&modelFlag, "model", "", "model name override")
	return cmd
}
```

注意上面的 baseDelay 写错了，修正版：
```go
import "time"
// ...
provider = llm.NewRetryable(provider, 3, 500*time.Millisecond)
```

- [ ] **Step 5: 注册 run 命令 + 接入 logger**

Modify `internal/cli/root.go`:
```go
package cli

import (
	"github.com/bytedance/trae-agent/internal/config"
	"github.com/bytedance/trae-agent/internal/logger"
	"github.com/spf13/cobra"
)

var (
	Version = "dev"
	Commit  = "none"
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "trae",
		Short:         "Trae Agent — CLI coding agent",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.LoadOptions{})
			if err != nil {
				return err
			}
			l, err := logger.Init(cfg.LogLevel)
			if err != nil {
				return err
			}
			if closer, ok := l.(interface{ Close() error }); ok {
				cmd.SetContext(context.WithValue(cmd.Context(), loggerCloserKey{}, closer))
			}
			return nil
		},
	}
	root.AddCommand(NewVersionCmd())
	root.AddCommand(NewShowConfigCmd())
	root.AddCommand(NewRunCmd())
	return root
}

type loggerCloserKey struct{}
```

需要加 `context` import。

- [ ] **Step 6: 运行测试**

Run:
```bash
go test ./internal/cli/... -v
```
Expected: 全部 PASS（version + show-config + run）

- [ ] **Step 7: 验证 CLI**

Run:
```bash
make build
# 不带 provider 应报错
./bin/trae run "hi" 2>&1 | head -3
# 带 mock 配置测试（手动）
```

- [ ] **Step 8: Commit**

```bash
git add internal/cli/run.go internal/cli/run_test.go internal/cli/root.go internal/config/config.go
git commit -m "feat(m1): add run subcommand with logger integration"
```

---

### Task 7: 端到端验收

**Files:** 无新增，验证整体

- [ ] **Step 1: 全量测试**

Run:
```bash
make test
```
Expected: 全部 PASS

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
Expected: 正常输出版本

- [ ] **Step 4: run 命令无 provider 报错**

Run:
```bash
HOME=/tmp/trae-m1-empty ./bin/trae run "hi" 2>&1
```
Expected: 报错 "no provider specified"

- [ ] **Step 5: run 命令带配置**

Run:
```bash
mkdir -p /tmp/trae-m1-test/.trae
cat > /tmp/trae-m1-test/.trae/config.yaml <<'EOF'
default_provider: anthropic
model_providers:
  anthropic:
    api_key: test-key
    base_url: http://localhost:9999
    default_model: claude-3-5-sonnet-20241022
EOF
HOME=/tmp/trae-m1-test ./bin/trae run "hi" 2>&1 | head -5
```
Expected: 尝试连接 localhost:9999 失败，重试 3 次后报错（因为 mock server 没启动）。验证重试逻辑工作。

- [ ] **Step 6: 验证 logger 写文件**

Run:
```bash
HOME=/tmp/trae-m1-test ./bin/trae run "hi" 2>&1
ls /tmp/trae-m1-test/.trae/logs/
```
Expected: `trae.log` 文件存在

- [ ] **Step 7: 清理**

Run:
```bash
rm -rf /tmp/trae-m1-test /tmp/trae-m1-empty
```

- [ ] **Step 8: Commit + push**

```bash
git push origin solo-go
```

---

## M1 完成标准

- [ ] `trae run "prompt" --provider anthropic` 流式输出（需真实 API key）
- [ ] `--provider openai` 同样可用
- [ ] 中途断网自动重试 3 次（Task 7 Step 5 验证）
- [ ] token usage 统计正确（测试覆盖）
- [ ] `make test` 全部 PASS
- [ ] `make vet` 无错误
- [ ] logger 接入 CLI 执行路径，写 `~/.trae/logs/trae.log`

## 留到 M2 的项

- ToolCallDelta 事件类型（接口已预留，未实现）
- 多轮对话（M1 只单轮）
- 交互式 REPL（M3）
- compact（M4）
