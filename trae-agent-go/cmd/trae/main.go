// Copyright (c) 2025 ByteDance Ltd. and/or its affiliates
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/bytedance/trae-agent-go/internal/agent"
	"github.com/bytedance/trae-agent-go/internal/config"
	"github.com/bytedance/trae-agent-go/internal/tool"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var version = "dev"

const defaultAgentName = "trae_agent"

func main() {
	rootCmd := newRootCmd()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:              "trae",
		Short:            "Trae Agent - AI 编码助手",
		Version:          version,
	}

	cmd.AddCommand(newRunCmd())
	cmd.AddCommand(newInteractiveCmd())
	cmd.AddCommand(newShowConfigCmd())
	cmd.AddCommand(newToolsCmd())

	return cmd
}

func newRunCmd() *cobra.Command {
	var (
		provider   string
		model      string
		maxSteps   int
		workingDir string
		configPath string
		filePath   string
	)

	cmd := &cobra.Command{
		Use:   "run [task]",
		Short: "执行单个任务",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			task, err := resolveTask(args, filePath)
			if err != nil {
				return err
			}

			cfg, err := loadConfigOrDefault(configPath)
			if err != nil {
				return fmt.Errorf("加载配置失败: %w", err)
			}

			agentCfg, err := buildAgentConfig(cfg, provider, model, maxSteps, workingDir)
			if err != nil {
				return fmt.Errorf("构建 Agent 配置失败: %w", err)
			}

			a, err := agent.NewAgent(agentCfg)
			if err != nil {
				return fmt.Errorf("创建 Agent 失败: %w", err)
			}

			a.NewTask(task, workingDir)

			execution, err := a.ExecuteTask(cmd.Context())
			if err != nil {
				return fmt.Errorf("执行任务失败: %w", err)
			}

			printExecution(cmd.OutOrStdout(), execution)
			return nil
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "", "覆盖 LLM 提供者")
	cmd.Flags().StringVar(&model, "model", "", "覆盖模型名称")
	cmd.Flags().IntVar(&maxSteps, "max-steps", 0, "覆盖最大步数")
	cmd.Flags().StringVar(&workingDir, "working-dir", "", "工作目录")
	cmd.Flags().StringVar(&configPath, "config", "", "配置文件路径")
	cmd.Flags().StringVarP(&filePath, "file", "f", "", "从文件读取任务描述")

	return cmd
}

func newInteractiveCmd() *cobra.Command {
	var (
		provider   string
		model      string
		maxSteps   int
		workingDir string
		configPath string
	)

	cmd := &cobra.Command{
		Use:   "interactive",
		Short: "启动交互式会话",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfigOrDefault(configPath)
			if err != nil {
				return fmt.Errorf("加载配置失败: %w", err)
			}

			agentCfg, err := buildAgentConfig(cfg, provider, model, maxSteps, workingDir)
			if err != nil {
				return fmt.Errorf("构建 Agent 配置失败: %w", err)
			}

			a, err := agent.NewAgent(agentCfg)
			if err != nil {
				return fmt.Errorf("创建 Agent 失败: %w", err)
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			out := cmd.OutOrStdout()

			scanner := bufio.NewScanner(os.Stdin)
			fmt.Fprintln(out, "Trae Agent 交互模式 (输入 exit 或 quit 退出)")

			for {
				fmt.Fprint(out, "> ")
				if !scanner.Scan() {
					break
				}
				line := scanner.Text()
				if line == "exit" || line == "quit" {
					fmt.Fprintln(out, "再见！")
					break
				}
				if line == "" {
					continue
				}

				a.NewTask(line, workingDir)

				execution, err := a.ExecuteTask(ctx)
				if err != nil {
					fmt.Fprintf(os.Stderr, "执行任务失败: %v\n", err)
					continue
				}

				printExecution(out, execution)
			}

			if err := scanner.Err(); err != nil {
				return fmt.Errorf("读取输入失败: %w", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "", "覆盖 LLM 提供者")
	cmd.Flags().StringVar(&model, "model", "", "覆盖模型名称")
	cmd.Flags().IntVar(&maxSteps, "max-steps", 0, "覆盖最大步数")
	cmd.Flags().StringVar(&workingDir, "working-dir", "", "工作目录")
	cmd.Flags().StringVar(&configPath, "config", "", "配置文件路径")

	return cmd
}

func newShowConfigCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "show-config",
		Short: "显示当前配置",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfigOrDefault(configPath)
			if err != nil {
				return fmt.Errorf("加载配置失败: %w", err)
			}

			data, err := yaml.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("序列化配置失败: %w", err)
			}

			fmt.Fprint(cmd.OutOrStdout(), string(data))
			return nil
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "", "配置文件路径")

	return cmd
}

func newToolsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tools",
		Short: "列出所有可用工具",
		RunE: func(cmd *cobra.Command, args []string) error {
			registry := tool.NewDefaultRegistry()
			names := registry.List()

			out := cmd.OutOrStdout()
			for _, name := range names {
				t, err := registry.Get(name)
				if err != nil {
					continue
				}
				fmt.Fprintf(out, "%-25s %s\n", t.GetName(), t.GetDescription())
			}
			return nil
		},
	}
}

// loadConfigOrDefault 加载配置文件，如果找不到则使用默认配置。
func loadConfigOrDefault(configPath string) (*config.Config, error) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		// 找不到配置文件时使用默认配置
		return config.DefaultConfig(), nil
	}
	return cfg, nil
}

// resolveTask 从位置参数或文件解析任务描述。
func resolveTask(args []string, filePath string) (string, error) {
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("读取任务文件失败: %w", err)
		}
		return string(data), nil
	}

	if len(args) > 0 {
		return args[0], nil
	}

	return "", fmt.Errorf("请提供任务描述作为参数，或使用 --file 指定任务文件")
}

// buildAgentConfig 从应用配置和 CLI 覆盖构建 agent.AgentConfig。
func buildAgentConfig(cfg *config.Config, providerOverride, modelOverride string, maxStepsOverride int, workingDir string) (agent.AgentConfig, error) {
	// 获取 agent 配置，优先使用 trae_agent
	agentConf, ok := cfg.Agents[defaultAgentName]
	if !ok {
		// 如果没有 trae_agent 配置，构造一个默认的
		agentConf = config.AgentConfig{
			Provider: cfg.DefaultProvider,
			MaxSteps: 30,
		}
	}

	// 应用 CLI 覆盖
	providerName := agentConf.Provider
	if providerOverride != "" {
		providerName = providerOverride
	}

	modelName := agentConf.Model
	if modelOverride != "" {
		modelName = modelOverride
	}

	maxSteps := agentConf.MaxSteps
	if maxStepsOverride > 0 {
		maxSteps = maxStepsOverride
	}

	// 解析模型配置
	modelCfg, err := cfg.ResolveModelConfig(providerName, modelName)
	if err != nil {
		return agent.AgentConfig{}, err
	}

	return agent.AgentConfig{
		ModelConfig: modelCfg,
		MaxSteps:    maxSteps,
		ToolNames:   agentConf.Tools,
		WorkingDir:  workingDir,
	}, nil
}

// printExecution 输出执行结果。
func printExecution(out io.Writer, execution *agent.AgentExecution) {
	fmt.Fprintf(out, "\n=== 任务执行结果 ===\n")
	fmt.Fprintf(out, "任务: %s\n", execution.Task)
	fmt.Fprintf(out, "状态: %s\n", execution.AgentState)
	fmt.Fprintf(out, "成功: %s\n", strconv.FormatBool(execution.Success))
	fmt.Fprintf(out, "步数: %d\n", len(execution.Steps))
	if execution.TotalTokens != nil {
		fmt.Fprintf(out, "Token 使用: 输入=%d, 输出=%d\n", execution.TotalTokens.InputTokens, execution.TotalTokens.OutputTokens)
	}
	fmt.Fprintf(out, "执行时间: %.2f 秒\n", execution.ExecutionTime)

	if len(execution.Steps) > 0 {
		fmt.Fprintln(out, "\n--- 执行步骤 ---")
		for _, step := range execution.Steps {
			fmt.Fprintf(out, "步骤 %d [%s]: %s\n", step.StepNumber, step.State, truncate(step.Thought, 100))
			for _, tc := range step.ToolCalls {
				fmt.Fprintf(out, "  工具调用: %s\n", tc.Name)
			}
			for _, tr := range step.ToolResults {
				status := "成功"
				if !tr.Success {
					status = "失败"
				}
				fmt.Fprintf(out, "  工具结果: %s (%s)\n", tr.Name, status)
			}
		}
	}
	fmt.Fprintln(out, "===================")
}

// truncate 截断字符串到指定长度。
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
