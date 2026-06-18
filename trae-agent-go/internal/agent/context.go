// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GetProjectContext 扫描项目根目录，返回格式化的项目上下文摘要。
func GetProjectContext(projectPath string) string {
	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return fmt.Sprintf("[Unable to read project directory: %v]", err)
	}

	var dirs, files []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if entry.IsDir() {
			dirs = append(dirs, name+"/")
		} else {
			files = append(files, name)
		}
	}

	sort.Strings(dirs)
	sort.Strings(files)

	projectType := detectProjectType(projectPath)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Project path: %s\n", projectPath))
	if projectType != "" {
		sb.WriteString(fmt.Sprintf("Project type: %s\n", projectType))
	}
	if len(dirs) > 0 {
		sb.WriteString(fmt.Sprintf("Directories: %s\n", strings.Join(dirs, ", ")))
	}
	if len(files) > 0 {
		sb.WriteString(fmt.Sprintf("Files: %s\n", strings.Join(files, ", ")))
	}

	return sb.String()
}

// detectProjectType 根据项目根目录的标志性文件检测项目类型。
func detectProjectType(projectPath string) string {
	indicators := map[string]string{
		"pyproject.toml": "Python",
		"package.json":   "Node.js",
		"go.mod":         "Go",
		"Cargo.toml":     "Rust",
	}
	for filename, projectType := range indicators {
		if _, err := os.Stat(filepath.Join(projectPath, filename)); err == nil {
			return projectType
		}
	}
	return ""
}

// InjectProjectContext 将项目上下文注入到用户消息中。
func InjectProjectContext(userMessage string, projectPath string) string {
	ctx := GetProjectContext(projectPath)
	return fmt.Sprintf("%s\n\n--- Project Context ---\n%s", userMessage, ctx)
}
