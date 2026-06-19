# M0 骨架实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立可编译的 Go 项目骨架，支持 `trae version` 和 `trae show-config` 两个子命令，配置多级加载与优先级合并正确。

**Architecture:** 单二进制 CLI，cobra 做命令分发，config 包负责 YAML 多级加载（默认 → 用户 → 项目 → env → flag），logger 包用 slog 写 `~/.trae/logs/`。本里程碑不含 LLM、agent、工具，只搭骨架。

**Tech Stack:** Go 1.22+、spf13/cobra、gopkg.in/yaml.v3、log/slog、stretchr/testify

---

## 文件结构

本里程碑产出/修改的文件：

- Create: `go.mod`
- Create: `go.sum`
- Create: `cmd/trae/main.go` — CLI 入口
- Create: `internal/logger/logger.go` — slog 封装
- Create: `internal/logger/logger_test.go`
- Create: `internal/config/config.go` — 配置类型与加载逻辑
- Create: `internal/config/loader.go` — 多级合并
- Create: `internal/config/config_test.go`
- Create: `internal/cli/root.go` — cobra 根命令
- Create: `internal/cli/version.go` — version 子命令
- Create: `internal/cli/show_config.go` — show-config 子命令
- Create: `Makefile`
- Create: `.golangci.yml`
- Create: `.gitignore`

---

### Task 1: go.mod 初始化与项目骨架

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `cmd/trae/main.go`

- [ ] **Step 1: 初始化 go module**

Run:
```bash
cd /workspace
go mod init github.com/bytedance/trae-agent
```
Expected: 创建 `go.mod`，首行 `module github.com/bytedance/trae-agent`，go 版本 ≥ 1.22

- [ ] **Step 2: 写 .gitignore**

Create `.gitignore`:
```gitignore
# binaries
/bin/
/dist/

# go
*.test
*.out
coverage.txt

# editor
.vscode/
.idea/

# os
.DS_Store
```

- [ ] **Step 3: 写最小 main.go（仅占位）**

Create `cmd/trae/main.go`:
```go
package main

import "fmt"

func main() {
	fmt.Println("trae")
}
```

- [ ] **Step 4: 验证可编译运行**

Run:
```bash
go build -o bin/trae ./cmd/trae && ./bin/trae
```
Expected: 输出 `trae`，退出码 0

- [ ] **Step 5: Commit**

```bash
git add go.mod .gitignore cmd/trae/main.go
git commit -m "feat(m0): init go module and minimal main"
```

---

### Task 2: logger 包

**Files:**
- Create: `internal/logger/logger.go`
- Create: `internal/logger/logger_test.go`

- [ ] **Step 1: 写失败测试**

Create `internal/logger/logger_test.go`:
```go
package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit_writesToFile(t *testing.T) {
	// 用 TMPDIR 替代 HOME，避免污染真实环境
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	l, err := Init("debug")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if l == nil {
		t.Fatal("logger is nil")
	}

	l.Info("hello", "key", "value")

	// 关闭刷新
	if closer, ok := l.(interface{ Close() error }); ok {
		_ = closer.Close()
	}

	logPath := filepath.Join(tmp, ".trae", "logs", "trae.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "hello") {
		t.Errorf("log file does not contain 'hello', got: %s", data)
	}
	if !strings.Contains(string(data), "key=value") {
		t.Errorf("log file does not contain key=value, got: %s", data)
	}
}

func TestInit_invalidLevel(t *testing.T) {
	_, err := Init("invalid")
	if err == nil {
		t.Error("expected error for invalid level, got nil")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
go test ./internal/logger/...
```
Expected: FAIL，编译错误 `undefined: Init`

- [ ] **Step 3: 实现 logger**

Create `internal/logger/logger.go`:
```go
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Init 初始化全局 slog logger，写文件到 ~/.trae/logs/trae.log。
// level: debug / info / warn / error
// 返回的 *slog.Logger 同时作为全局 slog.Default 的后端。
func Init(level string) (*slog.Logger, error) {
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
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger, nil
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

// 用于测试时关闭文件句柄
type closableLogger struct {
	*slog.Logger
	closer io.Closer
}

func (c *closableLogger) Close() error { return c.closer.Close() }
```

注意：测试里用了类型断言 `interface{ Close() error }`，但当前实现返回的是 `*slog.Logger`，没有 Close 方法。需要调整实现，让 Init 返回的 logger 可关闭。修正版：

Create `internal/logger/logger.go`（覆盖上面）:
```go
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Logger 包装 slog.Logger，使其可关闭底层文件。
type Logger struct {
	*slog.Logger
	file *os.File
}

// Close 关闭日志文件。
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Init 初始化 logger，写文件到 ~/.trae/logs/trae.log。
// level: debug / info / warn / error
func Init(level string) (*Logger, error) {
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
	l := &Logger{
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

// 确保编译期接口实现
var _ io.Closer = (*Logger)(nil)
```

- [ ] **Step 4: 运行测试确认通过**

Run:
```bash
go test ./internal/logger/... -v
```
Expected: PASS，两个测试用例通过

- [ ] **Step 5: Commit**

```bash
git add internal/logger/
git commit -m "feat(m0): add logger package with slog file output"
```

---

### Task 3: config 包

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/loader.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: 写配置类型与默认值**

Create `internal/config/config.go`:
```go
package config

// Config 是合并后的最终配置。
type Config struct {
	DefaultProvider string                    `yaml:"default_provider"`
	Providers       map[string]ProviderConfig `yaml:"model_providers"`
	LogLevel        string                    `yaml:"log_level"`
}

// ProviderConfig 单个 LLM provider 配置。
type ProviderConfig struct {
	APIKey       string `yaml:"api_key"`
	Provider     string `yaml:"provider"`
	BaseURL      string `yaml:"base_url"`
	DefaultModel string `yaml:"default_model"`
	MaxTokens    int    `yaml:"max_tokens"`
	Temperature  float64 `yaml:"temperature"`
}

// Default 返回内置默认配置。
func Default() Config {
	return Config{
		DefaultProvider: "",
		Providers:       map[string]ProviderConfig{},
		LogLevel:        "info",
	}
}
```

- [ ] **Step 2: 写失败测试**

Create `internal/config/config_test.go`:
```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_defaultOnly(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	// 不写任何配置文件，应返回默认值
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if cfg.DefaultProvider != "" {
		t.Errorf("DefaultProvider = %q, want empty", cfg.DefaultProvider)
	}
}

func TestLoad_userConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	userCfg := filepath.Join(tmp, ".trae", "config.yaml")
	writeFile(t, userCfg, `
default_provider: anthropic
log_level: debug
`)
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.DefaultProvider != "anthropic" {
		t.Errorf("DefaultProvider = %q, want anthropic", cfg.DefaultProvider)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}

func TestLoad_projectOverridesUser(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	userCfg := filepath.Join(tmp, ".trae", "config.yaml")
	writeFile(t, userCfg, `
default_provider: anthropic
log_level: debug
`)
	// 项目级配置覆盖用户级
	projCfg := filepath.Join(tmp, "project", ".trae", "config.yaml")
	writeFile(t, projCfg, `
default_provider: openai
`)
	cfg, err := Load(LoadOptions{ProjectDir: filepath.Join(tmp, "project")})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.DefaultProvider != "openai" {
		t.Errorf("DefaultProvider = %q, want openai (project override)", cfg.DefaultProvider)
	}
	// 未被覆盖的字段保留用户级
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug (from user)", cfg.LogLevel)
	}
}

func TestLoad_envOverridesAll(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	userCfg := filepath.Join(tmp, ".trae", "config.yaml")
	writeFile(t, userCfg, `
default_provider: anthropic
`)
	t.Setenv("TRAE_DEFAULT_PROVIDER", "openai")
	cfg, err := Load(LoadOptions{})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.DefaultProvider != "openai" {
		t.Errorf("DefaultProvider = %q, want openai (env override)", cfg.DefaultProvider)
	}
}

func TestLoad_flagOverridesAll(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("TRAE_DEFAULT_PROVIDER", "openai")
	cfg, err := Load(LoadOptions{DefaultProvider: "gemini"})
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.DefaultProvider != "gemini" {
		t.Errorf("DefaultProvider = %q, want gemini (flag override)", cfg.DefaultProvider)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

Run:
```bash
go test ./internal/config/... -v
```
Expected: FAIL，`undefined: Load`、`undefined: LoadOptions`

- [ ] **Step 4: 实现 loader**

Create `internal/config/loader.go`:
```go
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadOptions 控制配置加载行为。
type LoadOptions struct {
	ProjectDir      string // 项目根目录，查找 .trae/config.yaml
	DefaultProvider string // flag 覆盖
	LogLevel        string // flag 覆盖
}

// Load 按优先级合并配置：默认 < 用户 < 项目 < env < flag
func Load(opts LoadOptions) (Config, error) {
	cfg := Default()

	// 1. 用户级 ~/.trae/config.yaml
	if userCfg, err := loadYaml(userConfigPath()); err == nil {
		merge(&cfg, userCfg)
	} else if !os.IsNotExist(err) {
		return cfg, fmt.Errorf("load user config: %w", err)
	}

	// 2. 项目级 <project>/.trae/config.yaml
	if opts.ProjectDir != "" {
		projPath := filepath.Join(opts.ProjectDir, ".trae", "config.yaml")
		if projCfg, err := loadYaml(projPath); err == nil {
			merge(&cfg, projCfg)
		} else if !os.IsNotExist(err) {
			return cfg, fmt.Errorf("load project config: %w", err)
		}
	}

	// 3. 环境变量
	if v := os.Getenv("TRAE_DEFAULT_PROVIDER"); v != "" {
		cfg.DefaultProvider = v
	}
	if v := os.Getenv("TRAE_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	// 4. flag
	if opts.DefaultProvider != "" {
		cfg.DefaultProvider = opts.DefaultProvider
	}
	if opts.LogLevel != "" {
		cfg.LogLevel = opts.LogLevel
	}

	return cfg, nil
}

func userConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".trae", "config.yaml")
}

func loadYaml(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// merge 用 src 覆盖 dst，零值不覆盖。
func merge(dst, src Config) {
	if src.DefaultProvider != "" {
		dst.DefaultProvider = src.DefaultProvider
	}
	if src.LogLevel != "" {
		dst.LogLevel = src.LogLevel
	}
	if dst.Providers == nil {
		dst.Providers = map[string]ProviderConfig{}
	}
	for k, v := range src.Providers {
		dst.Providers[k] = v
	}
}
```

- [ ] **Step 5: 拉依赖并运行测试**

Run:
```bash
go mod tidy
go test ./internal/config/... -v
```
Expected: PASS，5 个测试用例全部通过

- [ ] **Step 6: Commit**

```bash
git add internal/config/ go.mod go.sum
git commit -m "feat(m0): add config package with multi-level merge"
```

---

### Task 4: cobra 根命令 + version 子命令

**Files:**
- Create: `internal/cli/root.go`
- Create: `internal/cli/version.go`
- Modify: `cmd/trae/main.go`

- [ ] **Step 1: 写 version 命令测试**

Create `internal/cli/version_test.go`:
```go
package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand_output(t *testing.T) {
	Version = "0.1.0"
	Commit = "abc1234"
	var buf bytes.Buffer
	cmd := NewVersionCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.Run(cmd, []string{})
	out := buf.String()
	if !strings.Contains(out, "0.1.0") {
		t.Errorf("output missing version, got: %s", out)
	}
	if !strings.Contains(out, "abc1234") {
		t.Errorf("output missing commit, got: %s", out)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
go test ./internal/cli/... -v
```
Expected: FAIL，`undefined: NewVersionCmd`、`undefined: Version`、`undefined: Commit`

- [ ] **Step 3: 实现 root 与 version**

Create `internal/cli/root.go`:
```go
package cli

import (
	"github.com/spf13/cobra"
)

// Version, Commit 由 ldflags 注入。
var (
	Version = "dev"
	Commit  = "none"
)

// NewRootCmd 构造根命令。
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "trae",
		Short: "Trae Agent — CLI coding agent",
	}
	root.AddCommand(NewVersionCmd())
	root.AddCommand(NewShowConfigCmd())
	return root
}
```

Create `internal/cli/version.go`:
```go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewVersionCmd 构造 version 子命令。
func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print trae version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "trae %s (commit: %s)\n", Version, Commit)
		},
	}
}
```

- [ ] **Step 4: 更新 main.go**

Overwrite `cmd/trae/main.go`:
```go
package main

import (
	"fmt"
	"os"

	"github.com/bytedance/trae-agent/internal/cli"
)

func main() {
	root := cli.NewRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 5: 拉依赖并运行测试**

Run:
```bash
go mod tidy
go test ./internal/cli/... -v
```
Expected: PASS

- [ ] **Step 6: 验证 CLI 可运行**

Run:
```bash
go build -o bin/trae ./cmd/trae && ./bin/trae version
```
Expected: 输出 `trae dev (commit: none)`

- [ ] **Step 7: Commit**

```bash
git add internal/cli/ cmd/trae/main.go go.mod go.sum
git commit -m "feat(m0): add cobra root command and version subcommand"
```

---

### Task 5: show-config 子命令

**Files:**
- Create: `internal/cli/show_config.go`
- Create: `internal/cli/show_config_test.go`

- [ ] **Step 1: 写测试**

Create `internal/cli/show_config_test.go`:
```go
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShowConfigCommand_output(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	// 写用户级配置
	userCfg := filepath.Join(tmp, ".trae", "config.yaml")
	os.MkdirAll(filepath.Dir(userCfg), 0o755)
	os.WriteFile(userCfg, []byte("default_provider: anthropic\nlog_level: debug\n"), 0o644)

	var buf bytes.Buffer
	cmd := NewShowConfigCmd()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.Run(cmd, []string{})

	out := buf.String()
	if !strings.Contains(out, "anthropic") {
		t.Errorf("output missing provider, got: %s", out)
	}
	if !strings.Contains(out, "debug") {
		t.Errorf("output missing log level, got: %s", out)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:
```bash
go test ./internal/cli/... -run TestShowConfig -v
```
Expected: FAIL，`undefined: NewShowConfigCmd`

- [ ] **Step 3: 实现 show-config**

Create `internal/cli/show_config.go`:
```go
package cli

import (
	"fmt"

	"github.com/bytedance/trae-agent/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewShowConfigCmd 构造 show-config 子命令，打印合并后配置。
func NewShowConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show-config",
		Short: "Print merged configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.LoadOptions{})
			if err != nil {
				return err
			}
			data, err := yaml.Marshal(cfg)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run:
```bash
go test ./internal/cli/... -v
```
Expected: PASS，version 和 show-config 测试都通过

- [ ] **Step 5: 验证 CLI 可运行**

Run:
```bash
go build -o bin/trae ./cmd/trae && ./bin/trae show-config
```
Expected: 输出 YAML 格式配置，含 `default_provider: ""` `log_level: info`

- [ ] **Step 6: Commit**

```bash
git add internal/cli/show_config.go internal/cli/show_config_test.go
git commit -m "feat(m0): add show-config subcommand"
```

---

### Task 6: Makefile + .golangci.yml

**Files:**
- Create: `Makefile`
- Create: `.golangci.yml`

- [ ] **Step 1: 写 Makefile**

Create `Makefile`:
```makefile
BINARY := trae
BIN_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -X github.com/bytedance/trae-agent/internal/cli.Version=$(VERSION) -X github.com/bytedance/trae-agent/internal/cli.Commit=$(COMMIT)

.PHONY: build test lint clean vet fmt

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) ./cmd/trae

test:
	go test ./... -v

vet:
	go vet ./...

fmt:
	gofmt -s -w .

lint: vet
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed, skipping"

clean:
	rm -rf $(BIN_DIR)
```

- [ ] **Step 2: 写 .golangci.yml**

Create `.golangci.yml`:
```yaml
run:
  timeout: 5m
  go: "1.22"

linters:
  enable:
    - errcheck
    - gosimple
    - govet
    - ineffassign
    - staticcheck
    - unused
    - misspell
    - revive

linters-settings:
  errcheck:
    check-type-assertions: true
```

- [ ] **Step 3: 验证 make build 注入版本号**

Run:
```bash
make build && ./bin/trae version
```
Expected: 输出 `trae <commit> (commit: <short-hash>)`，不再是 `dev`/`none`

- [ ] **Step 4: 验证 make test 通过**

Run:
```bash
make test
```
Expected: 全部测试 PASS

- [ ] **Step 5: 验证 make vet 通过**

Run:
```bash
make vet
```
Expected: 无输出，退出码 0

- [ ] **Step 6: Commit**

```bash
git add Makefile .golangci.yml
git commit -m "feat(m0): add Makefile and golangci config"
```

---

### Task 7: 端到端验收

**Files:** 无新增，验证整体

- [ ] **Step 1: 干净环境构建**

Run:
```bash
make clean && make build
```
Expected: `bin/trae` 生成，无错误

- [ ] **Step 2: version 命令**

Run:
```bash
./bin/trae version
```
Expected: 输出版本与 commit

- [ ] **Step 3: show-config 默认值**

Run:
```bash
./bin/trae show-config
```
Expected: 输出 YAML，`default_provider: ""` `log_level: info`

- [ ] **Step 4: 用户级配置生效**

Run:
```bash
mkdir -p ~/.trae
cat > ~/.trae/config.yaml <<'EOF'
default_provider: anthropic
log_level: debug
EOF
./bin/trae show-config
```
Expected: 输出 `default_provider: anthropic` `log_level: debug`

- [ ] **Step 5: 项目级覆盖用户级**

Run:
```bash
mkdir -p /tmp/trae-test-proj/.trae
cat > /tmp/trae-test-proj/.trae/config.yaml <<'EOF'
default_provider: openai
EOF
# 需要在项目目录下运行才生效——M0 暂用环境变量模拟项目目录
TRAE_PROJECT_DIR=/tmp/trae-test-proj ./bin/trae show-config
```

注意：当前 LoadOptions.ProjectDir 没有从环境变量读取。需要补充：在 `loader.go` 的 Load 里加上 `TRAE_PROJECT_DIR` 读取，或在 show_config.go 里读环境变量传入。**修正方案**：在 `Load` 中读取 `TRAE_PROJECT_DIR` 作为 ProjectDir 默认值。

Modify `internal/config/loader.go`，在 Load 函数开头加：
```go
func Load(opts LoadOptions) (Config, error) {
	cfg := Default()

	// 项目目录可由环境变量指定（测试与 CLI 用）
	if opts.ProjectDir == "" {
		opts.ProjectDir = os.Getenv("TRAE_PROJECT_DIR")
	}
	// ... 后续逻辑不变
```

重新运行 Step 5，Expected: 输出 `default_provider: openai` `log_level: debug`（openai 来自项目级，debug 仍来自用户级）

- [ ] **Step 6: 环境变量覆盖**

Run:
```bash
TRAE_DEFAULT_PROVIDER=gemini ./bin/trae show-config
```
Expected: 输出 `default_provider: gemini`

- [ ] **Step 7: flag 覆盖（M0 暂不接 flag，留 M1）**

跳过，记录到 M1 待办。

- [ ] **Step 8: 全量测试**

Run:
```bash
make test
```
Expected: 全部 PASS

- [ ] **Step 9: 清理测试残留**

Run:
```bash
rm -rf /tmp/trae-test-proj
# ~/.trae/config.yaml 保留，是用户配置
```

- [ ] **Step 10: Commit 验收修正**

```bash
git add internal/config/loader.go
git commit -m "fix(m0): read TRAE_PROJECT_DIR for project config path"
```

- [ ] **Step 11: 推送**

```bash
git push
```

---

## M0 完成标准

- [ ] `make build` 产出 `bin/trae`
- [ ] `./bin/trae version` 输出版本与 commit
- [ ] `./bin/trae show-config` 输出合并后 YAML 配置
- [ ] 配置优先级正确：默认 < 用户 < 项目 < env
- [ ] `make test` 全部 PASS
- [ ] `make vet` 无错误
- [ ] logger 写文件到 `~/.trae/logs/trae.log`
- [ ] 单次 commit 历史清晰，每个 Task 一个 commit
