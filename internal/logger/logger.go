// Package logger 提供结构化日志，同时输出到控制台(stderr)和文件。
// 控制台用精简格式（无时间戳），避免干扰交互式 REPL；
// 文件用完整格式（含时间戳、来源），便于排查问题。
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
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

// multiLogger 同时写控制台和文件，两者可有不同格式。
// fileLogger 为 nil 时表示仅控制台输出（log_file 配置为 "off"）。
type multiLogger struct {
	console *slog.Logger
	file    *slog.Logger
	closer  io.Closer
}

func (m *multiLogger) Debug(msg string, args ...any) {
	m.console.Debug(msg, args...)
	if m.file != nil {
		m.file.Debug(msg, args...)
	}
}

func (m *multiLogger) Info(msg string, args ...any) {
	m.console.Info(msg, args...)
	if m.file != nil {
		m.file.Info(msg, args...)
	}
}

func (m *multiLogger) Warn(msg string, args ...any) {
	m.console.Warn(msg, args...)
	if m.file != nil {
		m.file.Warn(msg, args...)
	}
}

func (m *multiLogger) Error(msg string, args ...any) {
	m.console.Error(msg, args...)
	if m.file != nil {
		m.file.Error(msg, args...)
	}
}

// Close 关闭文件。控制台无底层资源需释放。
func (m *multiLogger) Close() error {
	if m.closer != nil {
		return m.closer.Close()
	}
	return nil
}

// With 返回带固定键值对的子 logger。
// 子 logger 不持有文件 closer，关闭责任由原始 logger 承担，避免重复 Close。
func (m *multiLogger) With(args ...any) Logger {
	var f *slog.Logger
	if m.file != nil {
		f = m.file.With(args...)
	}
	return &multiLogger{console: m.console.With(args...), file: f}
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

// newConsoleHandler 构造控制台 handler：写 stderr，精简格式（去掉时间戳，减少噪音）。
func newConsoleHandler(level slog.Level) slog.Handler {
	return newConsoleHandlerTo(os.Stderr, level)
}

// newConsoleHandlerTo 构造写指定 writer 的控制台 handler。
func newConsoleHandlerTo(w io.Writer, level slog.Level) slog.Handler {
	opts := &slog.HandlerOptions{
		Level: level,
		// 去掉 time 字段，控制台输出更紧凑
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}
	return slog.NewTextHandler(w, opts)
}

// newFileHandler 构造文件 handler：完整格式，debug 级别附带来源行号。
func newFileHandler(w io.Writer, level slog.Level) slog.Handler {
	opts := &slog.HandlerOptions{Level: level}
	if level == slog.LevelDebug {
		opts.AddSource = true
	}
	return slog.NewTextHandler(w, opts)
}

// InitWithWriter 用指定 writer 初始化 logger（仅控制台语义，不绑定文件）。
// 适用于测试或输出到自定义 writer 等场景。
func InitWithWriter(w io.Writer, level string) (Logger, error) {
	lvl, err := parseLevel(level)
	if err != nil {
		return nil, err
	}
	return &multiLogger{console: slog.New(newConsoleHandlerTo(w, lvl))}, nil
}

// Init 初始化 logger，同时输出到控制台(stderr)和文件。
//
// 参数：
//   - level: 日志级别 debug / info / warn / error
//   - logFile: 日志文件路径
//     ""  → 使用默认 ~/.trae/logs/trae.log
//     "off" → 不写文件，仅控制台
//     其他 → 展开路径后写入（支持 ~/ 前缀）
//
// 控制台始终输出（满足快速定位问题的需求），文件用于持久化排查。
func Init(level, logFile string) (Logger, error) {
	lvl, err := parseLevel(level)
	if err != nil {
		return nil, err
	}

	console := slog.New(newConsoleHandler(lvl))

	// logFile == "off"：仅控制台，不写文件
	if strings.EqualFold(logFile, "off") {
		l := &multiLogger{console: console}
		slog.SetDefault(console)
		return l, nil
	}

	// 解析文件路径：空用默认，~/ 展开
	path, err := resolveLogPath(logFile)
	if err != nil {
		return nil, fmt.Errorf("resolve log file path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", path, err)
	}

	fileLogger := slog.New(newFileHandler(f, lvl))
	l := &multiLogger{console: console, file: fileLogger, closer: f}
	// 全局副作用：设置 slog.Default，使未持有 logger 引用的代码
	// 也能通过 slog.Info 等输出到控制台+文件。
	slog.SetDefault(slog.New(newMultiHandler(newConsoleHandler(lvl), newFileHandler(f, lvl))))
	return l, nil
}

// multiHandler 聚合两个 handler，使 slog.Default 同时写到控制台和文件。
type multiHandler struct {
	console slog.Handler
	file    slog.Handler
}

func newMultiHandler(console, file slog.Handler) *multiHandler {
	return &multiHandler{console: console, file: file}
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.console.Enabled(ctx, level) || h.file.Enabled(ctx, level)
}

func (h *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	if h.console.Enabled(ctx, r.Level) {
		if err := h.console.Handle(ctx, r.Clone()); err != nil {
			return err
		}
	}
	if h.file.Enabled(ctx, r.Level) {
		if err := h.file.Handle(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &multiHandler{
		console: h.console.WithAttrs(attrs),
		file:    h.file.WithAttrs(attrs),
	}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	return &multiHandler{
		console: h.console.WithGroup(name),
		file:    h.file.WithGroup(name),
	}
}

// resolveLogPath 解析日志文件路径。
//   - "" → 默认 ~/.trae/logs/trae.log（通过 paths 包统一管理）
//   - "~/..." → 展开为 home 目录下的路径
//   - 其他 → 原样返回（绝对路径或相对当前目录）
func resolveLogPath(logFile string) (string, error) {
	if logFile == "" {
		// 默认路径：~/.trae/logs/trae.log
		return paths.UnderTrae("logs", "trae.log")
	}
	// 展开 ~/ 前缀
	if strings.HasPrefix(logFile, "~/") || logFile == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home dir: %w", err)
		}
		return filepath.Join(home, strings.TrimPrefix(logFile, "~")), nil
	}
	return logFile, nil
}

var _ io.Closer = (*multiLogger)(nil)
var _ slog.Handler = (*multiHandler)(nil)
