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
			// mintty（Git Bash）兼容提示：检测到 mintty 时给出建议，
			// 但不自动包装（自动 winpty 会吞掉子进程错误，导致闪退不可见）。
			// 用户可手动运行 `winpty trae interactive` 获得完整 readline 体验。
			if isMintty() {
				fmt.Fprintln(os.Stderr, "[提示] 检测到 Git Bash/mintty 终端。")
				fmt.Fprintln(os.Stderr, "[提示] readline 在此终端下不兼容，将自动降级到简化输入模式。")
				fmt.Fprintln(os.Stderr, "[提示] 如需完整体验（行编辑/历史/补全），请运行：winpty trae interactive")
				fmt.Fprintln(os.Stderr, "[提示] 或使用 Windows Terminal / cmd 运行。")
				fmt.Fprintln(os.Stderr)
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
