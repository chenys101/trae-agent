package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Logger 是日志接口，包装 slog.Logger 的常用方法。
// 具体实现的动态类型还满足 io.Closer，可通过类型断言关闭底层文件。
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

// fileLogger 写日志到文件，并支持关闭底层文件。
type fileLogger struct {
	*slog.Logger
	file *os.File
}

// Close 关闭日志文件。
func (l *fileLogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Init 初始化 logger，写文件到 ~/.trae/logs/trae.log。
// level: debug / info / warn / error
func Init(level string) (Logger, error) {
	lvl, err := parseLevel(level)
	if err != nil {
		return nil, err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}
	logDir := filepath.Join(home, ".trae", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	logPath := filepath.Join(logDir, "trae.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	handler := slog.NewTextHandler(f, &slog.HandlerOptions{Level: lvl})
	l := &fileLogger{
		Logger: slog.New(handler),
		file:   f,
	}
	slog.SetDefault(l.Logger)
	return l, nil
}

func parseLevel(s string) (slog.Level, error) {
	switch s {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid log level: %s", s)
	}
}

var _ io.Closer = (*fileLogger)(nil)
