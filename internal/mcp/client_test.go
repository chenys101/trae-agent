package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"
)

// startMockServer 启动模拟 MCP server。
// 返回客户端应使用的 stdin（读端）和 stdout（写端），以及 cleanup 函数。
// handler 处理每个请求，返回结果或错误。通知（ID==0）不调用 handler。
//
// 管道方向说明：
//   - serverStdin, clientStdout := os.Pipe() —— server 从 serverStdin 读，client 写 clientStdout
//   - clientStdin, serverStdout := os.Pipe() —— client 从 clientStdin 读，server 写 serverStdout
func startMockServer(t *testing.T, handler func(req Request) (json.RawMessage, *RPCError)) (clientStdin io.ReadCloser, clientStdout io.WriteCloser, cleanup func()) {
	t.Helper()
	// pipe 1: client 写 -> server 读
	serverStdin, clientStdout, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	// pipe 2: server 写 -> client 读
	clientStdin, serverStdout, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer serverStdin.Close()
		defer serverStdout.Close()
		scanner := bufio.NewScanner(serverStdin)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			var req Request
			if err := json.Unmarshal(line, &req); err != nil {
				continue // 跳过无法解析的行
			}
			// 通知（ID==0）不需要响应
			if req.ID == 0 {
				continue
			}
			result, rpcErr := handler(req)
			resp := Response{JSONRPC: "2.0", ID: req.ID}
			if rpcErr != nil {
				resp.Error = rpcErr
			} else {
				resp.Result = result
			}
			data, _ := json.Marshal(resp)
			data = append(data, '\n')
			if _, err := serverStdout.Write(data); err != nil {
				return
			}
		}
	}()

	cleanup = func() {
		clientStdout.Close()
		clientStdin.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
	return clientStdin, clientStdout, cleanup
}

// newTestClient 创建测试客户端，使用 pipe 代替子进程。
// 调用方负责在测试结束后调用返回的 cleanup 函数。
func newTestClient(t *testing.T, handler func(req Request) (json.RawMessage, *RPCError)) (*Client, func()) {
	t.Helper()
	clientStdin, clientStdout, cleanup := startMockServer(t, handler)
	c := NewClient("test")
	c.stdin = clientStdout
	c.stdout = clientStdin
	c.scanner = bufio.NewScanner(clientStdin)
	c.scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	go c.readLoop()
	return c, cleanup
}

// TestClient_Initialize 测试 initialize 握手。
func TestClient_Initialize(t *testing.T) {
	handler := func(req Request) (json.RawMessage, *RPCError) {
		if req.Method != "initialize" {
			return nil, &RPCError{Code: -32601, Message: "method not found"}
		}
		result, _ := json.Marshal(InitializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities:    ServerCapabilities{Tools: &struct{}{}},
			ServerInfo:      ServerInfo{Name: "test-server", Version: "1.0.0"},
		})
		return result, nil
	}
	c, cleanup := newTestClient(t, handler)
	defer cleanup()

	ctx := context.Background()
	if err := c.initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	si := c.ServerInfo()
	if si.Name != "test-server" {
		t.Errorf("ServerInfo.Name = %q, want test-server", si.Name)
	}
	if si.Version != "1.0.0" {
		t.Errorf("ServerInfo.Version = %q, want 1.0.0", si.Version)
	}
	if c.Name() != "test" {
		t.Errorf("Name() = %q, want test", c.Name())
	}
}

// TestClient_ListTools 测试列出工具。
func TestClient_ListTools(t *testing.T) {
	handler := func(req Request) (json.RawMessage, *RPCError) {
		if req.Method != "tools/list" {
			return nil, &RPCError{Code: -32601, Message: "method not found"}
		}
		result, _ := json.Marshal(ListToolsResult{
			Tools: []Tool{
				{Name: "echo", Description: "echoes input"},
				{Name: "add", Description: "adds two numbers"},
			},
		})
		return result, nil
	}
	c, cleanup := newTestClient(t, handler)
	defer cleanup()

	ctx := context.Background()
	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("len(tools) = %d, want 2", len(tools))
	}
	if tools[0].Name != "echo" {
		t.Errorf("tools[0].Name = %q, want echo", tools[0].Name)
	}
	if tools[1].Name != "add" {
		t.Errorf("tools[1].Name = %q, want add", tools[1].Name)
	}
}

// TestClient_CallTool_Echo 测试 echo 工具调用。
func TestClient_CallTool_Echo(t *testing.T) {
	handler := func(req Request) (json.RawMessage, *RPCError) {
		if req.Method != "tools/call" {
			return nil, &RPCError{Code: -32601, Message: "method not found"}
		}
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		_ = json.Unmarshal(req.Params, &params)
		if params.Name != "echo" {
			return nil, &RPCError{Code: -32602, Message: "unknown tool: " + params.Name}
		}
		var args struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		result, _ := json.Marshal(ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: args.Text}},
		})
		return result, nil
	}
	c, cleanup := newTestClient(t, handler)
	defer cleanup()

	ctx := context.Background()
	args, _ := json.Marshal(map[string]string{"text": "hello"})
	result, err := c.CallTool(ctx, "echo", args)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("len(Content) = %d, want 1", len(result.Content))
	}
	if result.Content[0].Text != "hello" {
		t.Errorf("Content[0].Text = %q, want hello", result.Content[0].Text)
	}
	if result.IsError {
		t.Errorf("IsError = true, want false")
	}
}

// TestClient_CallTool_Add 测试 add 工具调用。
func TestClient_CallTool_Add(t *testing.T) {
	handler := func(req Request) (json.RawMessage, *RPCError) {
		if req.Method != "tools/call" {
			return nil, &RPCError{Code: -32601, Message: "method not found"}
		}
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		_ = json.Unmarshal(req.Params, &params)
		if params.Name != "add" {
			return nil, &RPCError{Code: -32602, Message: "unknown tool: " + params.Name}
		}
		var args map[string]int
		_ = json.Unmarshal(params.Arguments, &args)
		sum := args["a"] + args["b"]
		result, _ := json.Marshal(ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("%d", sum)}},
		})
		return result, nil
	}
	c, cleanup := newTestClient(t, handler)
	defer cleanup()

	ctx := context.Background()
	args, _ := json.Marshal(map[string]int{"a": 3, "b": 4})
	result, err := c.CallTool(ctx, "add", args)
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("len(Content) = %d, want 1", len(result.Content))
	}
	if result.Content[0].Text != "7" {
		t.Errorf("Content[0].Text = %q, want 7", result.Content[0].Text)
	}
}

// TestMCPTool_Adapter 测试 MCPTool 适配器的 Name/Description/Schema/Run。
func TestMCPTool_Adapter(t *testing.T) {
	handler := func(req Request) (json.RawMessage, *RPCError) {
		if req.Method != "tools/call" {
			return nil, &RPCError{Code: -32601, Message: "method not found"}
		}
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		_ = json.Unmarshal(req.Params, &params)
		var args struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		result, _ := json.Marshal(ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: "echo: " + args.Text}},
		})
		return result, nil
	}
	c, cleanup := newTestClient(t, handler)
	defer cleanup()

	schema := json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`)
	mcpTool := NewMCPTool(c, "test-server", "echo", "echoes input", schema)

	// 检查 Name（应为 mcp__<server>__<tool> 格式）
	if got := mcpTool.Name(); got != "mcp__test-server__echo" {
		t.Errorf("Name() = %q, want mcp__test-server__echo", got)
	}
	// 检查 Description
	if got := mcpTool.Description(); got != "echoes input" {
		t.Errorf("Description() = %q, want echoes input", got)
	}
	// 检查 Schema
	if got := string(mcpTool.Schema()); got != string(schema) {
		t.Errorf("Schema() = %s, want %s", got, schema)
	}
	// 检查 Run
	ctx := context.Background()
	args, _ := json.Marshal(map[string]string{"text": "hello"})
	result := mcpTool.Run(ctx, args)
	if result.IsError {
		t.Errorf("IsError = true, want false")
	}
	if result.Content != "echo: hello" {
		t.Errorf("Content = %q, want 'echo: hello'", result.Content)
	}
}

// TestMCPTool_AdapterError 测试 MCPTool 在 CallTool 失败时返回错误结果。
func TestMCPTool_AdapterError(t *testing.T) {
	handler := func(req Request) (json.RawMessage, *RPCError) {
		return nil, &RPCError{Code: -32603, Message: "boom"}
	}
	c, cleanup := newTestClient(t, handler)
	defer cleanup()

	mcpTool := NewMCPTool(c, "test-server", "echo", "echoes input", nil)
	ctx := context.Background()
	result := mcpTool.Run(ctx, json.RawMessage(`{}`))
	if !result.IsError {
		t.Errorf("IsError = false, want true")
	}
	if result.Content == "" {
		t.Errorf("Content empty, want error message")
	}
}

// TestClient_RPCError 测试 RPC 错误响应。
func TestClient_RPCError(t *testing.T) {
	handler := func(req Request) (json.RawMessage, *RPCError) {
		return nil, &RPCError{Code: -32600, Message: "invalid request"}
	}
	c, cleanup := newTestClient(t, handler)
	defer cleanup()

	ctx := context.Background()
	_, err := c.ListTools(ctx)
	if err == nil {
		t.Fatal("ListTools should fail with RPC error")
	}
}

// TestClient_Concurrent 测试并发请求（10 个并行调用）。
func TestClient_Concurrent(t *testing.T) {
	handler := func(req Request) (json.RawMessage, *RPCError) {
		if req.Method != "tools/call" {
			return nil, &RPCError{Code: -32601, Message: "method not found"}
		}
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		_ = json.Unmarshal(req.Params, &params)
		var args map[string]int
		_ = json.Unmarshal(params.Arguments, &args)
		sum := args["a"] + args["b"]
		result, _ := json.Marshal(ToolCallResult{
			Content: []ContentBlock{{Type: "text", Text: fmt.Sprintf("%d", sum)}},
		})
		return result, nil
	}
	c, cleanup := newTestClient(t, handler)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const n = 10
	var wg sync.WaitGroup
	errs := make([]error, n)
	results := make([]ToolCallResult, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args, _ := json.Marshal(map[string]int{"a": i, "b": i})
			result, err := c.CallTool(ctx, "add", args)
			errs[i] = err
			results[i] = result
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Errorf("goroutine %d: CallTool error: %v", i, errs[i])
			continue
		}
		want := fmt.Sprintf("%d", i+i)
		if len(results[i].Content) == 0 || results[i].Content[0].Text != want {
			t.Errorf("goroutine %d: Content = %v, want %s", i, results[i].Content, want)
		}
	}
}

// TestClient_CloseIdempotent 测试 Close 幂等。
func TestClient_CloseIdempotent(t *testing.T) {
	handler := func(req Request) (json.RawMessage, *RPCError) {
		return nil, &RPCError{Code: -32601, Message: "method not found"}
	}
	c, cleanup := newTestClient(t, handler)
	defer cleanup()

	if err := c.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("third Close: %v", err)
	}
}
