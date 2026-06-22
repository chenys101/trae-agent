package cli

import (
	"github.com/spf13/cobra"
)

func NewInteractiveCmd() *cobra.Command {
	var providerFlag, modelFlag string
	cmd := &cobra.Command{
		Use:   "interactive",
		Short: "Start interactive REPL session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configFromCmd(cmd)
			if err != nil {
				return err
			}

			a, policy, permStore, mcpMgr, err := buildAgent(cmd.Context(), cfg, providerFlag, modelFlag)
			if err != nil {
				return err
			}
			if mcpMgr != nil {
				defer mcpMgr.Close()
			}

			repl, err := NewREPL(a, policy, permStore, mcpMgr)
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
