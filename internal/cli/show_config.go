package cli

import (
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func NewShowConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show-config",
		Short: "Print merged configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := configFromCmd(cmd)
			if err != nil {
				return err
			}
			// 输出脱敏配置，避免泄漏 API key
			data, err := yaml.Marshal(cfg.Redacted())
			if err != nil {
				return err
			}
			// 直接写字节，避免不必要的 string 转换
			_, err = cmd.OutOrStdout().Write(data)
			return err
		},
	}
}
