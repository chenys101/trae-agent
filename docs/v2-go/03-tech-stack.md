# 技术选型

## 语言与版本

- **Go 1.22+**（要求 1.22 是为了用 `range over int`、增强的 `for` 循环、`slices`/`maps` 标准库）
- **无 cgo**：纯 Go 静态编译，便于交叉编译与单二进制分发
- **module path**：`github.com/bytedance/trae-agent/v2`（待确认）

## 核心依赖选型

### CLI 与终端

| 用途 | 选型 | 理由 | 备选 |
|------|------|------|------|
| flag 解析 | `spf13/cobra` | 生态成熟，子命令、补全、help 一站式 | `urfave/cli` |
| 流式渲染 | 自研 + `mattn/go-runewidth` | 需要精细控制光标、增量重绘、tool call 卡片 | `charmbracelet/lipgloss`（样式） |
| 终端能力探测 | `muesli/termenv` | 检测颜色支持、是否 TTY、宽高 | `mattn/go-isatty` |
| 行编辑 / 补全 | `chzyer/readline` 或 `peterh/liner` | 多行输入、历史、补全 | 自研 raw mode |
| 信号处理 | 标准库 `os/signal` | Ctrl+C 中断 | - |

**决策点**：是否引入 `bubbletea`？倾向于**不引入**——bubbletea 是全屏 TUI 框架，与 Claude Code 风格的流式文本交互不匹配，且会接管渲染循环限制灵活性。改用 lipgloss 做样式 + 自研流式渲染。

### LLM 客户端

| Provider | 选型 | 理由 |
|----------|------|------|
| Anthropic | 自研 HTTP 客户端 | 官方无 Go SDK，直接调 `/v1/messages` 流式接口 |
| OpenAI | `sashabaranov/go-openai` 或自研 | 社区库成熟，但流式 tool_call 支持需验证；倾向自研以统一抽象 |
| Google Gemini | 自研 | 官方 Go SDK 重，直接调 REST |
| OpenRouter | 复用 OpenAI 兼容客户端 | 协议兼容 |
| Ollama | 自研 | REST 简单 |

**统一原则**：所有 provider 实现统一的 `Provider` 接口，输出统一的 `StreamEvent` channel。**不依赖各厂商 SDK**，直接用 `net/http` + SSE 解析，避免 SDK 抽象泄漏和版本耦合。

### HTTP 与 SSE

- `net/http`（标准库）
- SSE 解析自研（行级 scanner 即可）
- 超时与重试：`context` + 指数退避（参考现有 Python `retry_utils.py` 思路）

### JSON Schema

- `xeipuuv/gojsonschema` 或 `qri-io/jsonschema`
- 用于工具参数校验和暴露给 LLM

### 配置

- YAML：`gopkg.in/yaml.v3`
- 环境变量：`caarlos0/env` 或标准库 `os.Getenv`
- 多级合并：自研

### 日志

- `log/slog`（Go 1.21+ 标准库结构化日志）
- 不引入 zap/zerolog，减少依赖
- 日志默认写 `~/.trae/logs/trae.log`，不污染终端

### 文件与路径

- `path/filepath`（标准库）
- glob：`mattn/go-zglob`（支持 `**`）或 `bmatcuk/doublestar`
- 主目录展开：`os.UserHomeDir`

### 搜索（grep 工具）

- **不内嵌 ripgrep 二进制**（违反单二进制原则）
- 自研基于 `doublestar` + `bufio.Scanner` 的 grep 实现
- 大文件流式扫描，支持 `-i` `-n` `-C` `--type` 等常用 flag
- 性能目标：在 10 万行代码库上 < 1s

### 进程执行（bash 工具）

- `os/exec` 标准库
- 支持：超时、后台运行、stdout/stderr 流式捕获、退出码
- shell：`/bin/sh -c`（跨平台兼容）

### MCP

- 自研 MCP 客户端（JSON-RPC over stdio / SSE）
- 不依赖官方 SDK（Go 生态 MCP SDK 不成熟）
- 兼容 MCP 1.0 规范

### 测试

- 标准库 `testing`
- 断言：`stretchr/testify`（社区事实标准）
- HTTP mock：`httptest` 标准库
- 覆盖率目标：核心包 ≥ 70%

### 构建与发布

- `Makefile`：`build` / `test` / `lint` / `release`
- lint：`golangci-lint`（配置 `.golangci.yml`）
- 跨平台编译：`goreleaser`（产出 macOS/Linux amd64+arm64）
- 单二进制 < 30MB

## 不引入的依赖（明确拒绝）

| 依赖 | 拒绝理由 |
|------|----------|
| `bubbletea` | 全屏 TUI，与流式文本交互冲突 |
| 各厂商 LLM SDK | 抽象泄漏，版本耦合，统一自研 |
| `ripgrep` 外部二进制 | 违反单二进制原则 |
| `zap` / `zerolog` | `slog` 已足够 |
| `viper` | 配置复杂度不需要，自研多级合并更轻 |
| `cobra` 的 completions 子包 | 补全自研更可控（待定） |

## 版本与兼容

- Go 版本：1.22+
- 最低支持 macOS 11+ / Linux glibc 2.31+
- 不支持 32 位
- API 稳定性：v2 内 `pkg/` 对外稳定，`internal/` 不承诺

## 依赖总量目标

- 直接依赖 ≤ 15 个
- 间接依赖 ≤ 80 个
- `go.sum` 可审计，无可疑来源
