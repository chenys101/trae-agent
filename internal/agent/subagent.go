package agent

import (
	"github.com/bytedance/trae-agent/internal/tool"
)

// SubagentType 定义子 agent 的类型，决定其可用工具集和行为约束。
type SubagentType struct {
	Name         string   // 类型名，如 "search"、"general-purpose"
	Description  string   // 给主 agent 看的描述，帮助 LLM 选择合适类型
	AllowedTools []string // 允许使用的工具名；nil 表示不限制（用全部工具）
}

// subagentTypes 内置子 agent 类型。
var subagentTypes = map[string]SubagentType{
	"search": {
		Name:         "search",
		Description:  "Fast agent specialized for exploring codebases. Use for high-level concept searches, finding connections between different parts of the codebase, or searching broad/ambiguous keywords. Only has read-only tools (read/glob/grep), cannot modify files.",
		AllowedTools: []string{"read", "glob", "grep"},
	},
	"general-purpose": {
		Name:         "general-purpose",
		Description:  "Perform a general-purpose coding task (a sub-task of the user's overall task). Use for complex multi-step coding tasks, operations that produce a lot of output not needed after the sub-agent completes, or cross-layer changes that have been planned out and can be implemented independently.",
		AllowedTools: nil, // 不限制，使用全部工具
	},
}

// GetSubagentType 按名称查询子 agent 类型。
func GetSubagentType(name string) (SubagentType, bool) {
	st, ok := subagentTypes[name]
	return st, ok
}

// ListSubagentTypes 返回所有内置子 agent 类型。
func ListSubagentTypes() []SubagentType {
	out := make([]SubagentType, 0, len(subagentTypes))
	for _, st := range subagentTypes {
		out = append(out, st)
	}
	return out
}

// RestrictedRegistry 基于全量 registry 构造受限 registry，只保留 AllowedTools 中的工具。
// 若 AllowedTools 为 nil，返回全量 registry 的副本。
func (st SubagentType) RestrictedRegistry(full *tool.Registry) *tool.Registry {
	if st.AllowedTools == nil {
		// 不限制：构造包含全部工具的新 registry
		return tool.NewRegistry(full.List()...)
	}
	allowed := make(map[string]bool, len(st.AllowedTools))
	for _, name := range st.AllowedTools {
		allowed[name] = true
	}
	var kept []tool.Tool
	for _, t := range full.List() {
		if allowed[t.Name()] {
			kept = append(kept, t)
		}
	}
	return tool.NewRegistry(kept...)
}
