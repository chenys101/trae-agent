package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func NewInteractiveCmd() *cobra.Command {
	var providerFlag, modelFlag string
	cmd := &cobra.Command{
		Use:   "interactive",
		Short: "Start interactive REPL session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// mintty（Git Bash）兼容：检测到 mintty 时自动用 winpty 包装，
			// 让 chzyer/readline 通过 PTY→Console 桥接正常工作。
			// 已在 winpty 下（TRAE_WINPTY=1）时跳过，避免递归。
			if shouldUseWinpty() {
				code, err := execViaWinpty()
				if err != nil {
					// winpty 自身失败，回退到 scanner 模式并打印提示
					fmt.Fprintf(os.Stderr, "[warn] %v\n", err)
					fmt.Fprintln(os.Stderr, "[warn] winpty 不可用，将使用简化输入模式（无行编辑/历史/补全）")
				} else {
					// winpty 包装成功，子进程已执行完毕，直接退出
					os.Exit(code)
				}
			}

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
