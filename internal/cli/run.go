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

func NewRunCmd() *cobra.Command {
	var providerFlag string
	var modelFlag string
	cmd := &cobra.Command{
		Use:   "run [prompt]",
		Short: "Run a prompt through the agent loop with tool access",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.LoadOptions{})
			if err != nil {
				return err
			}

			providerName := providerFlag
			if providerName == "" {
				providerName = cfg.DefaultProvider
			}
			if providerName == "" {
				return fmt.Errorf("no provider specified: set default_provider in config or use --provider")
			}

			provCfg, ok := cfg.Providers[providerName]
			if !ok {
				return fmt.Errorf("provider %q not found in config", providerName)
			}

			providerType := provCfg.Provider
			if providerType == "" {
				providerType = providerName
			}
			llmProvider, err := llm.NewProvider(providerType, provCfg.APIKey, provCfg.BaseURL, provCfg.DefaultModel)
			if err != nil {
				return err
			}
			llmProvider = llm.NewRetryable(llmProvider, 3, 500*time.Millisecond)

			// 构造工具
			registry := tool.NewRegistry(
				tool.NewRead(),
				tool.NewWrite(),
				tool.NewEdit(),
				tool.NewGlob(),
				tool.NewGrep(),
				tool.NewBash(120*time.Second),
			)

			maxSteps := cfg.MaxSteps
			if maxSteps == 0 {
				maxSteps = 20
			}
			a := agent.New(llmProvider, registry, agent.WithMaxSteps(maxSteps))

			events := make(chan agent.Event, 64)
			errCh := make(chan error, 1)
			go func() {
				errCh <- a.Run(cmd.Context(), args[0], events)
			}()

			usage, renderErr := agent.RenderEvents(cmd.Context(), events, cmd.OutOrStdout())
			if renderErr != nil {
				return renderErr
			}
			if err := <-errCh; err != nil {
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
