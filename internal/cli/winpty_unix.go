//go:build !windows

package cli

// shouldUseWinpty 在 Unix 上始终返回 false。
// Unix 终端原生支持 PTY，chzyer/readline 直接可用，无需 winpty 桥接。
func shouldUseWinpty() bool {
	return false
}

// execViaWinpty 在 Unix 上不会被调用，仅为编译占位。
func execViaWinpty() (int, error) {
	return 0, nil
}
