package mcp

import (
	"context"
	"fmt"
	"sync"

	"github.com/bytedance/trae-agent/internal/tool"
)

// ServerConfig 单个 MCP server 配置。
type ServerConfig struct {
	Command string   `json:"command" yaml:"command"`
	Args    []string `json:"args,omitempty" yaml:"args"`
	Env     []string `json:"env,omitempty" yaml:"env"`
}

// ServerStatus server 运行状态。
type ServerStatus struct {
	Name    string
	Running bool
	Tools   int
	Error   string
}

// Manager 管理多个 MCP server 的生命周期。
type Manager struct {
	mu      sync.Mutex
	clients map[string]*Client
	tools   []tool.Tool
}

// NewManager 创建管理器。
func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]*Client),
	}
}

// StartAll 启动所有配置的 server，返回每个 server 的启动错误。
// 部分失败不影响其他 server。
func (m *Manager) StartAll(ctx context.Context, servers map[string]ServerConfig) []error {
	var errs []error
	for name, cfg := range servers {
		if err := m.startOne(ctx, name, cfg); err != nil {
			errs = append(errs, fmt.Errorf("server %s: %w", name, err))
		}
	}
	return errs
}

// startOne 启动单个 server 并发现工具。
func (m *Manager) startOne(ctx context.Context, name string, cfg ServerConfig) error {
	client := NewClient(name)
	if err := client.Start(ctx, cfg.Command, cfg.Args, cfg.Env); err != nil {
		return err
	}

	// 发现工具
	tools, err := client.ListTools(ctx)
	if err != nil {
		client.Close()
		return fmt.Errorf("list tools: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[name] = client
	for _, t := range tools {
		mcpTool := NewMCPTool(client, name, t.Name, t.Description, t.InputSchema)
		m.tools = append(m.tools, mcpTool)
	}
	return nil
}

// Tools 返回所有已连接 server 的工具。
func (m *Manager) Tools() []tool.Tool {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]tool.Tool, len(m.tools))
	copy(out, m.tools)
	return out
}

// Servers 返回所有 server 状态。
func (m *Manager) Servers() []ServerStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	statuses := make([]ServerStatus, 0, len(m.clients))
	for name := range m.clients {
		statuses = append(statuses, ServerStatus{
			Name:    name,
			Running: true,
		})
	}
	return statuses
}

// Close 关闭所有 server。幂等。
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.clients {
		c.Close()
	}
	m.clients = make(map[string]*Client)
	m.tools = nil
	return nil
}
