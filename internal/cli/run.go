package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/bytedance/trae-agent/internal/agent"
	"github.com/bytedance/trae-agent/internal/config"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/tool"
	"github.com/spf13/cobra"
)

// defaultMaxRetries 是 LLM provider 重试包装器的最大尝试次数（含首次调用）。
// 保持硬编码：不同 provider 的重试策略差异不大，且 config 暂未提供该字段。
const defaultMaxRetries = 3

// defaultRetryBaseDelay 是重试退避的初始延迟。
const defaultRetryBaseDelay = 500 * time.Millisecond

// defaultBashTimeout 是内置 bash 工具的单次执行超时。
const defaultBashTimeout = 120 * time.Second

// defaultMaxStepsFallback 是 config 未设置 MaxSteps 时的兜底值。
// 正常情况下 config.Default() 已设为 20，此兜底仅防御性使用。
const defaultMaxStepsFallback = 20

// buildAgent 根据 config 与 flag 构造 agent 实例，供 run / interactive 共享。
// providerFlag / modelFlag 为空时使用 config 默认值。
func buildAgent(cfg config.Config, providerFlag, modelFlag string) (*agent.Agent, error) {
	providerName := providerFlag
	if providerName == "" {
		providerName = cfg.DefaultProvider
	}
	if providerName == "" {
		return nil, fmt.Errorf("no provider specified: set default_provider in config or use --provider")
	}

	provCfg, ok := cfg.Providers[providerName]
	if !ok {
		return nil, fmt.Errorf("provider %q not found in config", providerName)
	}

	providerType := provCfg.Provider
	if providerType == "" {
		providerType = providerName
	}
	llmProvider, err := llm.NewProvider(providerType, provCfg.APIKey, provCfg.BaseURL, provCfg.DefaultModel)
	if err != nil {
		return nil, err
	}
	llmProvider = llm.NewRetryable(llmProvider, defaultMaxRetries, defaultRetryBaseDelay)

	// 构造基础工具（不含 Task）。
	// SubagentRunner 持有 baseRegistry，使子 agent 不能派发子 agent，避免无限递归。
	baseTools := []tool.Tool{
		tool.NewRead(),
		tool.NewWrite(),
		tool.NewEdit(),
		tool.NewGlob(),
		tool.NewGrep(),
		tool.NewBash(defaultBashTimeout),
	}
	baseRegistry := tool.NewRegistry(baseTools...)

	// 子 agent runner 复用主 agent 的 provider 和 baseRegistry。
	// model 用 provCfg.DefaultModel 兜底，避免 modelFlag 为空时子 agent API 调用失败。
	subagentModel := modelFlag
	if subagentModel == "" {
		subagentModel = provCfg.DefaultModel
	}
	subagentRunner := agent.NewSubagentRunner(llmProvider, baseRegistry, subagentModel)

	// 最终 registry 在基础工具之上加入 Task 工具
	fullRegistry := tool.NewRegistry(append(baseTools, tool.NewTask(subagentRunner))...)

	maxSteps := cfg.MaxSteps
	if maxSteps == 0 {
		maxSteps = defaultMaxStepsFallback
	}
	return agent.New(llmProvider, fullRegistry, agent.WithMaxSteps(maxSteps), agent.WithModel(modelFlag)), nil
}

func NewRunCmd() *cobra.Command {
	var providerFlag string
	var modelFlag string
	cmd := &cobra.Command{
		Use:   "run [prompt]",
		Short: "Run a prompt through the agent loop with tool access",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configFromCmd(cmd)
			if err != nil {
				return err
			}

			a, err := buildAgent(cfg, providerFlag, modelFlag)
			if err != nil {
				return err
			}

			events := make(chan agent.Event, 64)
			errCh := make(chan error, 1)
			go func() {
				errCh <- a.Run(cmd.Context(), args[0], events)
			}()

			renderer := NewRenderer()
			usage, renderErr := renderer.Render(events, cmd.OutOrStdout())
			if renderErr != nil {
				return renderErr
			}
			if err := <-errCh; err != nil {
				// 区分 context 取消与其他错误，给出友好提示
				if cmd.Context().Err() != nil {
					fmt.Fprintln(os.Stderr, "\n[interrupted]")
					return nil
				}
				return err
			}
			fmt.Fprintf(os.Stderr, "\n[tokens: in=%d out=%d]\n", usage.InputTokens, usage.OutputTokens)
			return nil
		},
	}
	cmd.Flags().StringVar(&providerFlag, "provider", "", "LLM provider (anthropic/openai)")
	cmd.Flags().StringVar(&modelFlag, "model", "", "model name override")
	return cmd
}
