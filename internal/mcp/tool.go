package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bytedance/trae-agent/internal/tool"
)

// MCPTool 将 MCP 工具适配为 tool.Tool 接口。
type MCPTool struct {
	client   *Client
	toolName string
	fullName string // mcp__<server>__<tool>
	desc     string
	schema   json.RawMessage
}

// NewMCPTool 创建 MCP 工具适配器。
// fullName 格式为 mcp__<server>__<tool>，避免与内置工具和其他 server 工具冲突。
func NewMCPTool(client *Client, serverName, toolName, desc string, schema json.RawMessage) *MCPTool {
	return &MCPTool{
		client:   client,
		toolName: toolName,
		fullName: fmt.Sprintf("mcp__%s__%s", serverName, toolName),
		desc:     desc,
		schema:   schema,
	}
}

var _ tool.Tool = (*MCPTool)(nil)

func (m *MCPTool) Name() string            { return m.fullName }
func (m *MCPTool) Description() string     { return m.desc }
func (m *MCPTool) Schema() json.RawMessage { return m.schema }

// Run 调用 MCP server 执行工具，拼接文本内容块返回。
func (m *MCPTool) Run(ctx context.Context, args json.RawMessage) tool.Result {
	result, err := m.client.CallTool(ctx, m.toolName, args)
	if err != nil {
		return tool.ErrorResult("mcp call %s: %v", m.toolName, err)
	}
	// 拼接所有文本内容块
	var text string
	for _, block := range result.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	if result.IsError {
		return tool.Result{Content: text, IsError: true}
	}
	return tool.Result{Content: text}
}
