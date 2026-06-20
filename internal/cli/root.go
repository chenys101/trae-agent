package cli

import (
	"context"

	"github.com/bytedance/trae-agent/internal/config"
	"github.com/bytedance/trae-agent/internal/logger"
	"github.com/spf13/cobra"
)

var (
	Version = "dev"
	Commit  = "none"
)

type loggerCloserKey struct{}

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "trae",
		Short:         "Trae Agent — CLI coding agent",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.LoadOptions{})
			if err != nil {
				return err
			}
			l, err := logger.Init(cfg.LogLevel)
			if err != nil {
				return err
			}
			if closer, ok := l.(interface{ Close() error }); ok {
				cmd.SetContext(context.WithValue(cmd.Context(), loggerCloserKey{}, closer))
			}
			return nil
		},
	}
	root.AddCommand(NewVersionCmd())
	root.AddCommand(NewShowConfigCmd())
	root.AddCommand(NewRunCmd())
	return root
}
