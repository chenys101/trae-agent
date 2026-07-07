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

// configKey 用于在 context 中存取已加载的 config，避免子命令重复加载。
type configKey struct{}

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "trae",
		Short:         "Trae Agent — CLI coding agent",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// config 在此加载一次并存入 context，子命令通过 configFromCmd 取出复用，
			// 避免 run / interactive / show-config 各自重复加载。
			cfg, err := config.Load(config.LoadOptions{})
			if err != nil {
				return err
			}
			ctx := context.WithValue(cmd.Context(), configKey{}, cfg)
			// logger 同时输出到控制台(stderr)和文件，文件路径由 cfg.LogFile 配置
			l, err := logger.Init(cfg.LogLevel, cfg.LogFile)
			if err != nil {
				return err
			}
			if closer, ok := l.(interface{ Close() error }); ok {
				ctx = context.WithValue(ctx, loggerCloserKey{}, closer)
			}
			cmd.SetContext(ctx)
			return nil
		},
		PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
			if v := cmd.Context().Value(loggerCloserKey{}); v != nil {
				if closer, ok := v.(interface{ Close() error }); ok {
					_ = closer.Close()
				}
			}
			return nil
		},
	}
	root.AddCommand(NewVersionCmd())
	root.AddCommand(NewShowConfigCmd())
	root.AddCommand(NewRunCmd())
	root.AddCommand(NewInteractiveCmd())
	return root
}

// configFromCmd 从 cmd.Context() 取出 PersistentPreRunE 已加载的 config。
// 若 context 中不存在（如子命令被直接构造执行、未走 root 的 PersistentPreRunE），
// 则降级为现场加载，保证测试与独立调用可用。
func configFromCmd(cmd *cobra.Command) (config.Config, error) {
	if ctx := cmd.Context(); ctx != nil {
		if v, ok := ctx.Value(configKey{}).(config.Config); ok {
			return v, nil
		}
	}
	return config.Load(config.LoadOptions{})
}
