package cli

import (
	"fmt"
	"strings"

	"github.com/bytedance/trae-agent/internal/permission"
)

// actionName 将 Action 转为可读字符串。
func actionName(a permission.Action) string {
	switch a {
	case permission.ActionAllow:
		return "allow"
	case permission.ActionAsk:
		return "ask"
	case permission.ActionDeny:
		return "deny"
	case permission.ActionAllowAlways:
		return "allow-always"
	case permission.ActionDenyAlways:
		return "deny-always"
	default:
		return fmt.Sprintf("action(%d)", int(a))
	}
}

// formatRule 格式化单条规则为可读字符串。
func formatRule(r permission.Rule, builtin bool) string {
	tag := "user"
	if builtin {
		tag = "builtin"
	}
	argsStr := "(any args)"
	if len(r.Args) > 0 {
		argsStr = fmt.Sprintf("args~=%q", strings.Join(r.Args, ", "))
	}
	return fmt.Sprintf("  [%s] %-6s tool=%-8s %s desc=%q",
		tag, actionName(r.Action), r.Tool, argsStr, r.Desc)
}

// handlePermissionsCommand 处理 /permissions 命令。
// 无参数：列出所有规则（内置 + 用户）。
// clear：清除用户规则并持久化。
func handlePermissionsCommand(r *REPL, args []string) CommandResult {
	if r.policy == nil {
		return CommandResult{Message: "permission policy not configured"}
	}

	// /permissions clear：清除用户规则
	if len(args) > 0 && args[0] == "clear" {
		r.policy.ClearUserRules()
		if r.permStore != nil {
			if err := r.permStore.Save(r.policy); err != nil {
				return CommandResult{Message: fmt.Sprintf("clear user rules: save failed: %v", err)}
			}
		}
		return CommandResult{Message: "user permission rules cleared (builtins retained)"}
	}

	// /permissions：列出所有规则
	rules := r.policy.Rules()
	if len(rules) == 0 {
		return CommandResult{Message: "no permission rules"}
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Permission rules (%d total):\n", len(rules)))
	// 内置规则在前，用户规则在后
	builtinCount := 0
	for _, rule := range rules {
		if isBuiltinRule(rule) && builtinCount < countBuiltins(r) {
			b.WriteString(formatRule(rule, true))
			b.WriteString("\n")
			builtinCount++
		}
	}
	for _, rule := range r.policy.UserRules() {
		b.WriteString(formatRule(rule, false))
		b.WriteString("\n")
	}
	return CommandResult{Message: b.String()}
}

// isBuiltinRule 粗略判断是否为内置规则（通过描述匹配）。
// 这是一个辅助函数，实际区分由 DefaultPolicy 内部 builtinCount 决定。
func isBuiltinRule(r permission.Rule) bool {
	for _, b := range permission.NewPolicy().Rules() {
		if b.Desc == r.Desc && b.Tool == r.Tool && b.Action == r.Action {
			return true
		}
	}
	return false
}

// countBuiltins 返回内置规则数量。
func countBuiltins(r *REPL) int {
	if r.policy == nil {
		return 0
	}
	// 总规则数减去用户规则数即为内置规则数
	total := len(r.policy.Rules())
	user := len(r.policy.UserRules())
	return total - user
}

// NewPermissionsCmd 构造 /permissions 命令。
func NewPermissionsCmd() *Command {
	return &Command{
		Name:        "/permissions",
		Description: "Show or clear permission rules (usage: /permissions [clear])",
		Usage:       "/permissions [clear]",
		Handler:     handlePermissionsCommand,
	}
}
