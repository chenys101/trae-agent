package cli

import (
	"fmt"
	"io"

	"github.com/bytedance/trae-agent/internal/agent"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/charmbracelet/lipgloss"
)

// Renderer 流式渲染 agent 事件到终端。
type Renderer struct {
	toolCallStyle lipgloss.Style
	resultStyle   lipgloss.Style
	errorStyle    lipgloss.Style
	infoStyle     lipgloss.Style
}

func NewRenderer() *Renderer {
	return &Renderer{
		toolCallStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("63")).Bold(true),
		resultStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		errorStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
		infoStyle:     lipgloss.NewStyle().Foreground(lipgloss.Color("36")),
	}
}

// Welcome 返回欢迎信息。
func (r *Renderer) Welcome() string {
	return r.infoStyle.Render("trae interactive — type /help for commands, Ctrl+C to interrupt, Ctrl+D to exit")
}

// Render 消费 agent 事件流，渲染到 out。返回 token 用量。
func (r *Renderer) Render(events <-chan agent.Event, out io.Writer) (llm.Usage, error) {
	var usage llm.Usage
	for ev := range events {
		switch e := ev.(type) {
		case agent.TextEvent:
			fmt.Fprint(out, e.Content)
		case agent.ToolCallEvent:
			fmt.Fprintln(out)
			fmt.Fprintln(out, r.toolCallStyle.Render(fmt.Sprintf("▶ %s %s", e.Name, e.Args)))
		case agent.ToolResultEvent:
			content := e.Result.Content
			if len(content) > 500 {
				content = content[:500] + "... (truncated)"
			}
			if e.Result.IsError {
				fmt.Fprintln(out, r.errorStyle.Render(fmt.Sprintf("✗ %s", content)))
			} else {
				fmt.Fprintln(out, r.resultStyle.Render(fmt.Sprintf("✓ %s", content)))
			}
		case agent.DoneEvent:
			usage = e.Usage
		}
	}
	fmt.Fprintln(out)
	return usage, nil
}
