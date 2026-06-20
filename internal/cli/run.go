package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/bytedance/trae-agent/internal/config"
	"github.com/bytedance/trae-agent/internal/headless"
	"github.com/bytedance/trae-agent/internal/llm"
	"github.com/spf13/cobra"
)

func NewRunCmd() *cobra.Command {
	var providerFlag string
	var modelFlag string
	cmd := &cobra.Command{
		Use:   "run [prompt]",
		Short: "Run a single-turn prompt and stream the response",
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
			provider, err := llm.NewProvider(providerType, provCfg.APIKey, provCfg.BaseURL, provCfg.DefaultModel)
			if err != nil {
				return err
			}
			provider = llm.NewRetryable(provider, 3, 500*time.Millisecond)

			model := modelFlag
			if model == "" {
				model = provCfg.DefaultModel
			}

			req := llm.Request{
				Model:    model,
				System:   cfg.SystemPrompt,
				Messages: []llm.Message{{Role: llm.RoleUser, Content: args[0]}},
			}
			if provCfg.MaxTokens > 0 {
				req.MaxTokens = provCfg.MaxTokens
			}
			req.Temperature = provCfg.Temperature

			usage, err := headless.Run(cmd.Context(), provider, req, cmd.OutOrStdout())
			if err != nil {
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
