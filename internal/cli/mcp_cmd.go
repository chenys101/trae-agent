package cli

import (
	"fmt"
	"strings"
)

// handleMCPCommand 处理 /mcp 命令，显示已连接的 MCP server 和工具。
func handleMCPCommand(r *REPL, args []string) CommandResult {
	if r.mcpMgr == nil {
		return CommandResult{Message: "MCP manager not configured"}
	}

	servers := r.mcpMgr.Servers()
	tools := r.mcpMgr.Tools()

	if len(servers) == 0 {
		return CommandResult{Message: "no MCP servers connected (configure mcp_servers in config.yaml)"}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("MCP servers (%d connected, %d tools):\n", len(servers), len(tools)))
	for _, s := range servers {
		status := "running"
		if !s.Running {
			status = "stopped"
		}
		b.WriteString(fmt.Sprintf("  %s [%s]", s.Name, status))
		if s.Error != "" {
			b.WriteString(fmt.Sprintf(" error=%s", s.Error))
		}
		b.WriteString("\n")
	}

	if len(tools) > 0 {
		b.WriteString("\nTools:\n")
		for _, t := range tools {
			desc := t.Description()
			if len(desc) > 60 {
				desc = desc[:60] + "..."
			}
			b.WriteString(fmt.Sprintf("  %-40s %s\n", t.Name(), desc))
		}
	}
	return CommandResult{Message: b.String()}
}

// NewMCPCmd 构造 /mcp 命令。
func NewMCPCmd() *Command {
	return &Command{
		Name:        "/mcp",
		Description: "Show connected MCP servers and tools",
		Usage:       "/mcp",
		Handler:     handleMCPCommand,
	}
}
