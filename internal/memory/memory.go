// Package memory 负责加载和管理项目记忆文件（AGENTS.md）。
// 记忆文件用于让 agent 了解项目约定、结构、命令等，类似 Claude Code 的 CLAUDE.md。
package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// 记忆文件名，统一使用 AGENTS.md
const MemoryFile = "AGENTS.md"

// Load 加载项目级和用户级记忆，拼接后返回。
// 优先级：项目级 .trae/AGENTS.md > 用户级 ~/.trae/AGENTS.md
// 项目级内容在前，用户级内容在后，用分隔线区分。
func Load(workDir string) string {
	var parts []string

	// 项目级：.trae/AGENTS.md
	if projectMem := loadFile(filepath.Join(workDir, ".trae", MemoryFile)); projectMem != "" {
		parts = append(parts, "# Project Memory\n\n"+projectMem)
	}

	// 用户级：~/.trae/AGENTS.md
	home, err := os.UserHomeDir()
	if err == nil {
		if userMem := loadFile(filepath.Join(home, ".trae", MemoryFile)); userMem != "" {
			parts = append(parts, "# User Memory\n\n"+userMem)
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// loadFile 读取文件内容，文件不存在时返回空字符串。
func loadFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// SaveProject 将内容写入项目级 .trae/AGENTS.md。
func SaveProject(workDir, content string) error {
	dir := filepath.Join(workDir, ".trae")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create .trae dir: %w", err)
	}
	path := filepath.Join(dir, MemoryFile)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write memory file: %w", err)
	}
	return nil
}

// ProjectPath 返回项目级记忆文件路径。
func ProjectPath(workDir string) string {
	return filepath.Join(workDir, ".trae", MemoryFile)
}

// Exists 检查项目级记忆文件是否存在。
func Exists(workDir string) bool {
	_, err := os.Stat(ProjectPath(workDir))
	return err == nil
}
