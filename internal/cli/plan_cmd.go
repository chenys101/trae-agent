package cli

// NewPlanCmd 创建 /plan 斜杠命令：切换计划模式。
// 开启后下一次 agent 调用只做规划分析，不执行修改操作。
// 再次输入 /plan 关闭计划模式。
func NewPlanCmd() *Command {
	return &Command{
		Name:        "/plan",
		Description: "Toggle plan mode (analyze only, no execution)",
		Usage:       "/plan",
		Handler: func(repl *REPL, args []string) CommandResult {
			repl.mu.Lock()
			repl.planMode = !repl.planMode
			on := repl.planMode
			repl.mu.Unlock()

			if on {
				return CommandResult{Message: "[plan mode ON — agent will plan without executing. Use /plan again to turn off]"}
			}
			return CommandResult{Message: "[plan mode OFF — normal execution resumed]"}
		},
	}
}
