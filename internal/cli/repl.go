package cli

import (
	"github.com/bytedance/trae-agent/internal/agent"
	"github.com/bytedance/trae-agent/internal/llm"
)

// REPL 交互式会话。Task 1 填充 Run 等方法。
type REPL struct {
	agent      *agent.Agent
	renderer   *Renderer
	commands   *CommandRegistry
	messages   []llm.Message
	totalUsage llm.Usage
}

// NewREPL 构造 REPL 实例。
func NewREPL(a *agent.Agent) *REPL {
	return &REPL{
		agent:    a,
		commands: NewCommandRegistry(),
		renderer: NewRenderer(),
	}
}
