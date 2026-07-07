package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/bytedance/trae-agent/internal/paths"
)

// Logger 是日志接口，包装 slog.Logger 的常用方法。
// Close 释放底层资源（如文件）；With 返回带固定键值对的子 logger。
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	Close() error
	With(args ...any) Logger
}

// fileLogger 写日志到文件，并支持关闭底层文件。
type fileLogger struct {
	*slog.Logger
	file *os.File
}

// Close 关闭日志文件。未绑定文件时（如 InitWithWriter 构造）为 no-op。
func (l *fileLogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// With 返回带固定键值对的子 logger。
// 子 logger 不持有底层文件，关闭责任由原始 logger 承担，避免重复 Close。
func (l *fileLogger) With(args ...any) Logger {
	return &fileLogger{Logger: l.Logger.With(args...)}
}

// newLogger 构造 fileLogger，可选绑定底层文件（Close 时关闭）。
// debug 级别开启 AddSource，便于定位日志来源。
func newLogger(w io.Writer, level string, file *os.File) (*fileLogger, error) {
	lvl, err := parseLevel(level)
	if err != nil {
		return nil, err
	}
	opts := &slog.HandlerOptions{Level: lvl}
	if lvl == slog.LevelDebug {
		opts.AddSource = true
	}
	handler := slog.NewTextHandler(w, opts)
	return &fileLogger{Logger: slog.New(handler), file: file}, nil
}

// InitWithWriter 用指定 writer 初始化 logger，不绑定文件（Close 为 no-op）。
// 适用于测试或输出到 stderr 等场景。
func InitWithWriter(w io.Writer, level string) (Logger, error) {
	return newLogger(w, level, nil)
}

// Init 初始化 logger，写文件到用户主目录下 .trae/logs/trae.log。
// level: debug / info / warn / error（大小写不敏感）
func Init(level string) (Logger, error) {
	// 用 paths 包统一管理 .trae 目录路径
	logPath, err := paths.UnderTrae("logs", "trae.log")
	if err != nil {
		return nil, fmt.Errorf("resolve log path: %w", err)
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	l, err := newLogger(f, level, f)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	// 全局副作用：设置 slog.Default，使未持有 logger 引用的代码（如第三方库）
	// 也能通过 slog.Info 等输出到同一文件。调用方依赖此行为，故保留。
	slog.SetDefault(l.Logger)
	return l, nil
}

// parseLevel 解析日志级别字符串，大小写不敏感。
func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid log level %q: valid values are debug, info, warn, error", s)
	}
}

var _ io.Closer = (*fileLogger)(nil)
