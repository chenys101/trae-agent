package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/bytedance/trae-agent/internal/consts"
)

// Client 管理 MCP server 子进程，通过 stdio 通信。
type Client struct {
	name       string
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	scanner    *bufio.Scanner
	mu         sync.Mutex
	nextID     int64
	pending    map[int64]chan Response
	closed     bool
	serverInfo ServerInfo
}

// NewClient 创建客户端（未启动）。
func NewClient(name string) *Client {
	return &Client{
		name:    name,
		pending: make(map[int64]chan Response),
	}
}

// Start 启动子进程并完成 initialize 握手。
func (c *Client) Start(ctx context.Context, command string, args []string, env []string) error {
	c.cmd = exec.CommandContext(ctx, command, args...)
	c.cmd.Env = env

	stdin, err := c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	c.stdin = stdin
	c.stdout = stdout
	c.scanner = bufio.NewScanner(stdout)
	// 增大 buffer，MCP 消息可能较大
	c.scanner.Buffer(make([]byte, 0, consts.ScannerInitialBuf), consts.ScannerMaxBuf)

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}

	// 启动读取循环
	go c.readLoop()

	// initialize 握手
	if err := c.initialize(ctx); err != nil {
		c.kill()
		return err
	}

	return nil
}

// initialize 完成 MCP initialize 握手。
func (c *Client) initialize(ctx context.Context) error {
	params, _ := json.Marshal(map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "trae-agent",
			"version": "0.1.0",
		},
	})

	resp, err := c.request(ctx, "initialize", params)
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	var initResult InitializeResult
	if err := json.Unmarshal(resp.Result, &initResult); err != nil {
		return fmt.Errorf("parse initialize result: %w", err)
	}
	c.serverInfo = initResult.ServerInfo

	// 发送 initialized 通知
	notif := Notification{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	data, _ := json.Marshal(notif)
	c.mu.Lock()
	_, err = c.stdin.Write(append(data, '\n'))
	c.mu.Unlock()
	if err != nil {
		return fmt.Errorf("send initialized notification: %w", err)
	}

	return nil
}

// readLoop 读取子进程 stdout，按 ID 分发响应。
func (c *Client) readLoop() {
	for c.scanner.Scan() {
		line := c.scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var resp Response
		if err := json.Unmarshal(line, &resp); err != nil {
			continue // 跳过无法解析的行
		}
		// 按 ID 分发到等待的请求
		if resp.ID != 0 {
			c.mu.Lock()
			ch, ok := c.pending[resp.ID]
			if ok {
				delete(c.pending, resp.ID)
			}
			c.mu.Unlock()
			if ok {
				ch <- resp
			}
		}
	}
}

// request 发送请求并等待响应。
func (c *Client) request(ctx context.Context, method string, params json.RawMessage) (Response, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return Response{}, fmt.Errorf("client closed")
	}
	c.nextID++
	id := c.nextID
	ch := make(chan Response, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	req := Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return Response{}, err
	}
	data = append(data, '\n')

	c.mu.Lock()
	_, err = c.stdin.Write(data)
	c.mu.Unlock()
	if err != nil {
		return Response{}, fmt.Errorf("write request: %w", err)
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return resp, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp, nil
	case <-ctx.Done():
		return Response{}, ctx.Err()
	case <-time.After(consts.MCPRPCTimeout):
		return Response{}, fmt.Errorf("request timeout: %s", method)
	}
}

// ListTools 列出 server 提供的工具。
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	resp, err := c.request(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}
	var result ListToolsResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse tools/list result: %w", err)
	}
	return result.Tools, nil
}

// CallTool 调用工具。
func (c *Client) CallTool(ctx context.Context, name string, arguments json.RawMessage) (ToolCallResult, error) {
	params, _ := json.Marshal(map[string]interface{}{
		"name":      name,
		"arguments": json.RawMessage(arguments),
	})
	resp, err := c.request(ctx, "tools/call", params)
	if err != nil {
		return ToolCallResult{}, err
	}
	var result ToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return ToolCallResult{}, fmt.Errorf("parse tools/call result: %w", err)
	}
	return result, nil
}

// ServerInfo 返回 server 信息（initialize 后可用）。
func (c *Client) ServerInfo() ServerInfo {
	return c.serverInfo
}

// Name 返回 server 名称。
func (c *Client) Name() string {
	return c.name
}

// Close 关闭客户端，先尝试优雅退出，2s 后强制 kill。幂等。
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	// 关闭 stdin 通知子进程退出
	if c.stdin != nil {
		c.stdin.Close()
	}
	// 等待子进程退出，最多 2s
	done := make(chan error, 1)
	go func() {
		if c.cmd != nil && c.cmd.Process != nil {
			done <- c.cmd.Wait()
		} else {
			done <- nil
		}
	}()
	select {
	case <-done:
	case <-time.After(consts.MCPGracefulCloseTimeout):
		c.kill()
		<-done
	}
	return nil
}

// kill 强制终止子进程。
func (c *Client) kill() {
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}
}
