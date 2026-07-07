package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// CustomCommand 从 markdown 文件加载的自定义斜杠命令。
// 文件格式：可选 YAML frontmatter + markdown body。
// frontmatter 支持 description 和 argument-hint 字段。
// body 作为 prompt 发送给 agent，支持 $ARGUMENTS 和 $1/$2/... 参数替换。
type CustomCommand struct {
	name        string
	description string
	body        string
}

// frontmatterRe 匹配 markdown 文件开头的 YAML frontmatter。
var frontmatterRe = regexp.MustCompile(`(?s)^---\s*\n(.*?)\n---\s*\n?(.*)$`)

// LoadCustomCommands 从 .trae/commands/ 目录加载自定义命令。
// 文件名（去 .md 后缀）即命令名，自动加 / 前缀。
// 返回加载的命令列表，按名称排序。目录不存在时返回空切片。
func LoadCustomCommands(workDir string) []*CustomCommand {
	cmdDir := filepath.Join(workDir, ".trae", "commands")

	entries, err := os.ReadDir(cmdDir)
	if err != nil {
		return nil
	}

	var cmds []*CustomCommand
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		cmdName := "/" + strings.TrimSuffix(name, ".md")
		data, err := os.ReadFile(filepath.Join(cmdDir, name))
		if err != nil {
			continue
		}
		desc, body := parseFrontmatter(string(data))
		if desc == "" {
			desc = "Custom command: " + cmdName
		}
		cmds = append(cmds, &CustomCommand{
			name:        cmdName,
			description: desc,
			body:        body,
		})
	}

	// 按名称排序，保证 /help 输出稳定
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].name < cmds[j].name
	})
	return cmds
}

// parseFrontmatter 解析 YAML frontmatter，返回 description 和 body。
// 无 frontmatter 时 desc 为空，body 为原始内容。
func parseFrontmatter(content string) (desc, body string) {
	matches := frontmatterRe.FindStringSubmatch(content)
	if matches == nil {
		return "", strings.TrimSpace(content)
	}
	fmBody := matches[1]
	body = strings.TrimSpace(matches[2])

	// 简单解析 description 字段（不做完整 YAML 解析）
	for _, line := range strings.Split(fmBody, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "description:") {
			desc = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			// 去掉可能的引号
			desc = strings.Trim(desc, `"'`)
		}
	}
	return desc, body
}

// renderBody 将命令 body 中的参数占位符替换为实际值。
// 支持 $ARGUMENTS（全部参数）和 $1, $2, ... （位置参数）。
func (c *CustomCommand) renderBody(args []string) string {
	result := c.body
	// $ARGUMENTS 替换为所有参数拼接
	result = strings.ReplaceAll(result, "$ARGUMENTS", strings.Join(args, " "))
	// $1, $2, ... 替换为位置参数
	for i, arg := range args {
		result = strings.ReplaceAll(result, fmt.Sprintf("$%d", i+1), arg)
	}
	return result
}

// ToCommand 将自定义命令转换为 CommandRegistry 可注册的 Command。
func (c *CustomCommand) ToCommand() *Command {
	return &Command{
		Name:        c.name,
		Description: c.description,
		Usage:       c.name + " [args...]",
		Handler: func(repl *REPL, args []string) CommandResult {
			prompt := c.renderBody(args)
			if prompt == "" {
				return CommandResult{Message: "error: command body is empty"}
			}
			return CommandResult{
				Message:    fmt.Sprintf("[%s]", c.name),
				AgentInput: prompt,
			}
		},
	}
}
