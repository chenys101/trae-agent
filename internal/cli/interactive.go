package cli

import (
	"fmt"
	"time"

	"github.com/bytedance/trae-agent/internal/agent"
	"github.com/bytedance/trae-agent/internal/config"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/bytedance/trae-agent/internal/tool"
	"github.com/spf13/cobra"
)

func NewInteractiveCmd() *cobra.Command {
	var providerFlag string
	var modelFlag string
	cmd := &cobra.Command{
		Use:   "interactive",
		Short: "Start interactive REPL session",
		Args:  cobra.NoArgs,
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
			a := agent.New(llmProvider, registry, agent.WithMaxSteps(maxSteps), agent.WithModel(modelFlag))

		repl, err := NewREPL(a)
		if err != nil {
			return err
		}
		return repl.Run(cmd.Context())
		},
	}
	cmd.Flags().StringVar(&providerFlag, "provider", "", "LLM provider (anthropic/openai)")
	cmd.Flags().StringVar(&modelFlag, "model", "", "model name override")
	return cmd
}
