// Package paths 统一管理 trae 相关目录路径，消除跨包重复的
// os.UserHomeDir + filepath.Join(home, ".trae", ...) 逻辑。
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// TraeDir 返回用户主目录下的 .trae 目录路径，确保目录存在。
func TraeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	dir := filepath.Join(home, ".trae")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create .trae dir: %w", err)
	}
	return dir, nil
}

// UnderTrae 返回 .trae 目录下子路径，自动创建父目录。
// 例：UnderTrae("sessions") → ~/.trae/sessions/
func UnderTrae(parts ...string) (string, error) {
	dir, err := TraeDir()
	if err != nil {
		return "", err
	}
	full := filepath.Join(append([]string{dir}, parts...)...)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", fmt.Errorf("create dir %s: %w", filepath.Dir(full), err)
	}
	return full, nil
}

// UserConfigPath 返回用户级配置文件路径 ~/.trae/config.yaml。
func UserConfigPath() (string, error) {
	dir, err := TraeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// ProjectConfigPath 返回项目级配置文件路径 projectDir/.trae/config.yaml。
func ProjectConfigPath(projectDir string) string {
	return filepath.Join(projectDir, ".trae", "config.yaml")
}
