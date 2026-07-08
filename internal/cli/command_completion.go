package cli

import (
	"fmt"
	"sort"
	"strings"
)

// suggestCommands 根据用户输入提供命令建议。
// 场景：
//   - 输入 "/" 或空：返回所有命令
//   - 输入 "/partial"：返回前缀匹配的命令
//   - 输入 "/typo"（无前缀匹配）：返回编辑距离最近的命令
//
// 返回的字符串已格式化为可直接打印的多行文本，无匹配时返回空字符串。
func (r *REPL) suggestCommands(input string) string {
	input = strings.TrimSpace(input)
	all := r.commands.List()

	// 输入为空或仅 "/"：列出所有命令
	if input == "" || input == "/" {
		return r.formatCommandList(all)
	}

	// 前缀匹配
	var prefixMatches []*Command
	for _, cmd := range all {
		if strings.HasPrefix(cmd.Name, input) {
			prefixMatches = append(prefixMatches, cmd)
		}
	}
	if len(prefixMatches) > 0 {
		return r.formatCommandList(prefixMatches)
	}

	// 无前缀匹配：用编辑距离找最接近的命令
	closest := r.closestCommands(input, all, 3)
	if len(closest) > 0 {
		var b strings.Builder
		b.WriteString(fmt.Sprintf("No command matching %q. Did you mean:\n", input))
		b.WriteString(r.formatCommandList(closest))
		return b.String()
	}

	return ""
}

// formatCommandList 格式化命令列表为可打印的多行文本。
func (r *REPL) formatCommandList(cmds []*Command) string {
	if len(cmds) == 0 {
		return ""
	}
	// 按名称排序，保证输出稳定
	sorted := make([]*Command, len(cmds))
	copy(sorted, cmds)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	var b strings.Builder
	// 计算最长命令名用于对齐
	maxLen := 0
	for _, cmd := range sorted {
		if len(cmd.Name) > maxLen {
			maxLen = len(cmd.Name)
		}
	}
	for _, cmd := range sorted {
		fmt.Fprintf(&b, "  %-*s  %s\n", maxLen, cmd.Name, cmd.Description)
	}
	return b.String()
}

// closestCommands 用编辑距离（Levenshtein）找最接近的 N 个命令。
// threshold 为最大允许距离，超过则不视为候选。
// 阈值取命令长度的 1/2（向下取整），最小 2、最大 3，
// 兼顾短命令（如 /cat → /car 距离 1）和长命令（如 /permissions 拼错）。
func (r *REPL) closestCommands(input string, cmds []*Command, maxResults int) []*Command {
	type scored struct {
		cmd  *Command
		dist int
	}
	var scoredCmds []scored
	for _, cmd := range cmds {
		d := levenshtein(strings.ToLower(input), strings.ToLower(cmd.Name))
		// 阈值 = 命令长度 / 2，限制在 [2, 3]
		threshold := len(cmd.Name) / 2
		if threshold < 2 {
			threshold = 2
		}
		if threshold > 3 {
			threshold = 3
		}
		if d <= threshold {
			scoredCmds = append(scoredCmds, scored{cmd, d})
		}
	}
	// 按距离排序，取前 N 个
	sort.Slice(scoredCmds, func(i, j int) bool {
		return scoredCmds[i].dist < scoredCmds[j].dist
	})
	if len(scoredCmds) > maxResults {
		scoredCmds = scoredCmds[:maxResults]
	}
	result := make([]*Command, len(scoredCmds))
	for i, s := range scoredCmds {
		result[i] = s.cmd
	}
	return result
}

// levenshtein 计算两个字符串的编辑距离。
// 用于命令拼写纠错，如 "/hlep" → "/help"（距离=2）。
func levenshtein(a, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	la := len(ra)
	lb := len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// 滚动数组优化空间，仅需前一行
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			// 三种操作：删除、插入、替换
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			curr[j] = min3(del, ins, sub)
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

// min3 返回三个整数的最小值。
func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
