//go:build !windows

package cli

// isMintty 在 Unix 上始终返回 false。
// Unix 终端原生支持 PTY，chzyer/readline 直接可用。
func isMintty() bool {
	return false
}
