package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bytedance/trae-agent/internal/agent"
	"github.com/bytedance/trae-agent/internal/memory"
)

// initPromptTemplate 是 /init 命令让 agent 分析项目并生成 AGENTS.md 的提示词。
const initPromptTemplate = `Analyze this project and generate a project memory file (AGENTS.md) that will help future AI agent sessions understand this codebase quickly.

Please explore the project structure, read key files, and produce a concise AGENTS.md with the following sections:

## Project Overview
Brief description of what this project does.

## Tech Stack
Languages, frameworks, build tools, and key dependencies.

## Project Structure
Key directories and their purposes.

## Build & Test Commands
How to build, test, lint, and run the project (e.g., make test, go build).

## Coding Conventions
Naming conventions, code style, testing patterns observed in the codebase.

## Key Files
Important files an agent should know about (entry point, config, etc.).

Write the result to .trae/AGENTS.md using the write tool. Keep it concise and factual — only include what you actually observe in the codebase.`

// NewInitCmd 创建 /init 斜杠命令：让 agent 分析项目并生成 .trae/AGENTS.md。
// 通过 AgentInput 字段触发主循环自动执行 agent，复用现有渲染流程。
func NewInitCmd() *Command {
	return &Command{
		Name:        "/init",
		Description: "Analyze project and generate .trae/AGENTS.md memory file",
		Usage:       "/init",
		Handler: func(repl *REPL, args []string) CommandResult {
			workDir, err := os.Getwd()
			if err != nil {
				return CommandResult{Message: "error: cannot get working directory: " + err.Error()}
			}
			if memory.Exists(workDir) {
				return CommandResult{Message: fmt.Sprintf(
					"%s already exists. Delete it first if you want to regenerate.",
					filepath.Join(".trae", memory.MemoryFile),
				)}
			}
			return CommandResult{
				Message:     "[analyzing project and generating .trae/AGENTS.md...]",
				AgentInput:  initPromptTemplate,
			}
		},
	}
}

// RunInit 在 headless 模式下执行 /init 逻辑（供 trae init 子命令使用）。
// 返回生成的记忆文件路径和可能的错误。
func RunInit(ctx context.Context, a *agent.Agent) (string, error) {
	workDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	events := make(chan agent.Event, 64)
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Run(ctx, initPromptTemplate, events)
	}()

	// 消费事件，等待完成
	for range events {
	}
	if err := <-errCh; err != nil {
		return "", fmt.Errorf("agent init failed: %w", err)
	}

	path := memory.ProjectPath(workDir)
	if !memory.Exists(workDir) {
		return "", fmt.Errorf("agent did not generate %s", path)
	}
	return path, nil
}

