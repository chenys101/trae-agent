package cli

import (
	"fmt"

	"github.com/bytedance/trae-agent/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func NewShowConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show-config",
		Short: "Print merged configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.LoadOptions{})
			if err != nil {
				return err
			}
			// 输出脱敏配置，避免泄漏 API key
			data, err := yaml.Marshal(cfg.Redacted())
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
}
