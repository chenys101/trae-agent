package permission

import (
	"encoding/json"
	"strings"
)

// Action 权限决策动作。
type Action int

const (
	ActionAllow Action = iota
	ActionAsk
	ActionDeny
	ActionAllowAlways
	ActionDenyAlways
)

// Decision 权限检查结果。
type Decision struct {
	Action  Action
	Reason  string
	Matched string // 匹配的规则描述，供调试
}

// Rule 单条权限规则。
type Rule struct {
	Tool   string   `json:"tool"`   // 工具名，"*" 匹配所有
	Args   []string `json:"args"`   // 关键参数子串匹配（AND 关系），空则不检查 args
	Action Action   `json:"action"`
	Desc   string   `json:"desc"`   // 规则描述
}

// Policy 权限策略接口。
type Policy interface {
	Check(tool string, args json.RawMessage) Decision
}

// DefaultPolicy 默认策略，内置规则 + 用户规则。
type DefaultPolicy struct {
	rules        []Rule
	builtinCount int // 内置规则数量，用于区分用户规则
}

// NewPolicy 创建带内置规则的策略。
func NewPolicy() *DefaultPolicy {
	builtins := builtinRules()
	return &DefaultPolicy{
		rules:        builtins,
		builtinCount: len(builtins),
	}
}

// Check 检查工具调用是否被允许。
// 匹配顺序：用户规则从后往前（后加的优先），内置规则从前往后（具体的优先）。
func (p *DefaultPolicy) Check(tool string, args json.RawMessage) Decision {
	// 用户规则从后往前匹配（后加的优先级高）
	for i := len(p.rules) - 1; i >= p.builtinCount; i-- {
		if matchRule(p.rules[i], tool, args) {
			return Decision{Action: p.rules[i].Action, Reason: p.rules[i].Desc, Matched: p.rules[i].Desc}
		}
	}
	// 内置规则从前往后匹配（具体的优先，避免 rm -rf / 被 rm -rf 遮蔽）
	for i := 0; i < p.builtinCount; i++ {
		if matchRule(p.rules[i], tool, args) {
			return Decision{Action: p.rules[i].Action, Reason: p.rules[i].Desc, Matched: p.rules[i].Desc}
		}
	}
	return Decision{Action: ActionAllow, Reason: "no rule matched, default allow"}
}

// AddUserRule 添加用户规则（在内置规则之后）。
func (p *DefaultPolicy) AddUserRule(r Rule) {
	p.rules = append(p.rules, r)
}

// ClearUserRules 清除所有用户规则（原地修改，不重新分配 slice）。
// 必须原地修改，因为 dispatcher 持有同一个 *DefaultPolicy 指针。
func (p *DefaultPolicy) ClearUserRules() {
	p.rules = p.rules[:p.builtinCount]
}

// UserRules 返回用户规则的副本。
func (p *DefaultPolicy) UserRules() []Rule {
	out := make([]Rule, len(p.rules)-p.builtinCount)
	copy(out, p.rules[p.builtinCount:])
	return out
}

// Rules 返回所有规则的副本（供查看）。
func (p *DefaultPolicy) Rules() []Rule {
	out := make([]Rule, len(p.rules))
	copy(out, p.rules)
	return out
}

// matchRule 检查规则是否匹配。
// tool 匹配（精确或 "*"），args 子串全部匹配（AND）。
func matchRule(r Rule, tool string, args json.RawMessage) bool {
	if r.Tool != "*" && r.Tool != tool {
		return false
	}
	if len(r.Args) == 0 {
		return true
	}
	// 将 args 序列化为字符串进行子串匹配
	argsStr := string(args)
	for _, a := range r.Args {
		if !strings.Contains(argsStr, a) {
			return false
		}
	}
	return true
}

// ExtractKeyArg 从工具参数 JSON 中提取关键参数字符串，供构建规则。
// 对于 bash 工具，提取 command 字段；其他工具返回原始 JSON 字符串。
func ExtractKeyArg(tool string, args json.RawMessage) string {
	if tool == "bash" {
		var v struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(args, &v) == nil {
			return v.Command
		}
	}
	return string(args)
}

// builtinRules 返回内置危险命令规则。
// 顺序很重要：具体的规则在前（如 rm -rf /），宽泛的在后（如 rm -rf），
// 因为内置规则从前往后匹配，先匹配到的优先。
func builtinRules() []Rule {
	return []Rule{
		// Deny: 不可恢复的破坏性操作
		{Tool: "bash", Args: []string{"rm -rf /"}, Action: ActionDeny, Desc: "rm -rf / 禁止执行"},
		{Tool: "bash", Args: []string{"mkfs"}, Action: ActionDeny, Desc: "mkfs 格式化磁盘禁止执行"},
		{Tool: "bash", Args: []string{"dd if="}, Action: ActionDeny, Desc: "dd 写入磁盘禁止执行"},
		{Tool: "bash", Args: []string{":(){ :|:& };:"}, Action: ActionDeny, Desc: "fork bomb 禁止执行"},
		// Ask: 需要确认的操作
		{Tool: "bash", Args: []string{"rm -rf"}, Action: ActionAsk, Desc: "rm -rf 需要确认"},
		{Tool: "bash", Args: []string{"git push"}, Action: ActionAsk, Desc: "git push 需要确认"},
		{Tool: "bash", Args: []string{"git push --force"}, Action: ActionAsk, Desc: "git push --force 需要确认"},
	}
}
