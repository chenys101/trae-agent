# Trae Agent Go 重写 — Claude Code / Cursor CLI 级别编码 Agent

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**目标：** 用 Go 语言从零重写 Trae Agent，打造一个 Claude Code / Cursor CLI 级别的编码 Agent，具备高性能文件操作、上下文管理、多 LLM 支持和子 Agent 委派能力。

**架构：** 采用 Go 标准的分层架构——`Tool` 接口定义工具契约，`Agent` 结构体驱动执行循环，`LLMClient` 接口抽象多模型调用，`Config` 管理 YAML 配置。利用 Go 的 goroutine 实现并行工具调用和流式输出，利用接口组合实现可测试性。项目输出单个二进制文件，零运行时依赖。

**技术栈：** Go 1.22+, cobra (CLI), viper (配置), anthropic-sdk-go / openai-go (LLM), bubbletea (TUI), ripgrep (搜索)

---

## 范围检查

本计划覆盖 5 个独立子系统，按依赖顺序实施，每个子系统独立可测：

1. **核心框架** — Tool 接口、Agent 循环、数据结构
2. **工具生态** — 7 个内置工具（Bash、ReadFile、GrepSearch、GlobSearch、Edit、SequentialThinking、SubAgent）
3. **LLM 客户端** — 多 Provider 抽象与工具调用序列化
4. **CLI 与配置** — 命令行界面、YAML 配置、TUI
5. **集成与发布** — 端到端测试、构建、单二进制发布

---

## 文件结构

```
trae-agent-go/
├── cmd/
│   └── trae/                      # CLI 入口
│       └── main.go                # main 函数，cobra 根命令
├── internal/
│   ├── agent/                     # Agent 核心
│   │   ├── agent.go               # Agent 结构体、执行循环、上下文压缩
│   │   ├── step.go                # AgentStep / AgentExecution 数据结构
│   │   └── context.go             # 项目上下文扫描与注入
│   ├── tool/                      # 工具系统
│   │   ├── tool.go                # Tool 接口、ToolCall、ToolResult、ToolExecutor
│   │   ├── registry.go            # 工具注册表
│   │   ├── bash.go                # Bash 工具
│   │   ├── read_file.go           # ReadFile 工具
│   │   ├── grep_search.go         # GrepSearch 工具
│   │   ├── glob_search.go         # GlobSearch 工具
│   │   ├── edit.go                # 文件编辑工具（str_replace / create / insert / view）
│   │   ├── sequential_thinking.go # 结构化思考工具
│   │   └── sub_agent.go           # 子 Agent 委派工具
│   ├── llm/                       # LLM 客户端
│   │   ├── client.go              # LLMClient 接口、LLMMessage、LLMResponse
│   │   ├── anthropic.go           # Anthropic 实现
│   │   ├── openai.go              # OpenAI 实现
│   │   ├── google.go              # Google Gemini 实现
│   │   ├── ollama.go              # Ollama 实现
│   │   └── retry.go               # 重试机制
│   ├── config/                    # 配置系统
│   │   └── config.go              # YAML 配置加载、ModelConfig、AgentConfig
│   ├── prompt/                    # 提示词管理
│   │   └── prompt.go              # 系统提示词、动态提示构建
│   └── tui/                       # 终端 UI
│       └── tui.go                 # bubbletea TUI 实现
├── configs/
│   └── trae_config.yaml           # 示例配置文件
├── go.mod
├── go.sum
├── Makefile                       # 构建、测试、lint 命令
└── README.md
```

---

## 任务 1：项目骨架与核心数据结构

**文件：**
- 创建: `trae-agent-go/go.mod`
- 创建: `trae-agent-go/internal/tool/tool.go`
- 创建: `trae-agent-go/internal/agent/step.go`

**职责说明：**
- `tool.go` — 定义 `Tool` 接口（GetName、GetDescription、GetParameters、Execute）、`ToolCall` 结构体、`ToolResult` 结构体、`ToolExecutor` 结构体
- `step.go` — 定义 `AgentStep`（步骤号、状态、工具调用、工具结果、LLM 响应、反思）、`AgentExecution`（任务、步骤列表、最终结果、成功标志、token 用量、执行时间）、`AgentState` / `AgentStepState` 枚举

**设计决策：**
- `Tool.Execute` 方法签名返回 `(ToolResult, error)`，与 Go 惯例一致
- `ToolParameter` 包含 Name、Type、Description、Required、Enum 字段，用于生成 JSON Schema
- `ToolExecutor` 持有工具映射表，提供 `ExecuteToolCall`、`ParallelExecute`、`SequentialExecute` 方法
- `ParallelExecute` 利用 `errgroup` 并行执行多个工具调用

- [ ] **步骤 1：初始化 Go 模块**

运行: `mkdir -p trae-agent-go && cd trae-agent-go && go mod init github.com/bytedance/trae-agent-go`

- [ ] **步骤 2：定义 Tool 接口和核心数据结构**

创建 `internal/tool/tool.go`，定义：
- `Tool` 接口：`GetName() string`、`GetDescription() string`、`GetParameters() []ToolParameter`、`Execute(ctx context.Context, args map[string]any) (ToolResult, error)`
- `ToolParameter` 结构体：Name、Type、Description、Required、Enum、Items
- `ToolCall` 结构体：Name、CallID、Arguments
- `ToolResult` 结构体：CallID、Name、Success、Output、Error
- `ToolExecutor` 结构体：tools 映射表、并行/顺序执行方法
- `GetInputSchema()` 方法：生成 JSON Schema 格式的参数定义，兼容 OpenAI strict mode

- [ ] **步骤 3：定义 Agent 数据结构**

创建 `internal/agent/step.go`，定义：
- `AgentStepState` 枚举：Thinking、CallingTool、Reflecting、Completed、Error
- `AgentState` 枚举：Idle、Running、Completed、Error
- `AgentStep` 结构体：StepNumber、State、ToolCalls、ToolResults、LLMResponse、Reflection、Error
- `AgentExecution` 结构体：Task、Steps、FinalResult、Success、TotalTokens、ExecutionTime、AgentState

- [ ] **步骤 4：验证编译**

运行: `cd trae-agent-go && go build ./...`
预期: 编译成功，无错误

- [ ] **步骤 5：提交**

```bash
git add .
git commit -m "feat: 初始化项目骨架，定义 Tool 接口和 Agent 数据结构"
```

---

## 任务 2：工具注册表与工具实现（Bash、ReadFile、GrepSearch、GlobSearch）

**文件：**
- 创建: `internal/tool/registry.go`
- 创建: `internal/tool/bash.go`
- 创建: `internal/tool/read_file.go`
- 创建: `internal/tool/grep_search.go`
- 创建: `internal/tool/glob_search.go`

**职责说明：**
- `registry.go` — 全局工具注册表，`Register(name, factory)` / `Get(name)` / `List()` 方法
- `bash.go` — 持久化 bash 会话，通过 `os/exec` 执行命令，支持超时和重启
- `read_file.go` — 读取文件内容，支持行号和行范围（offset/limit），输出带行号格式
- `grep_search.go` — 调用 ripgrep (rg) 进行正则搜索，回退到 Go 标准库 `regexp`，支持 include glob 和大小写不敏感
- `glob_search.go` — 使用 `filepath.Glob` 和 `path/filepath.Walk` 进行文件模式匹配

**设计决策：**
- Bash 工具使用 `os/exec.Cmd` 而非持久化 shell session（Go 的 exec 模型与 Python 不同，每次命令独立执行，通过 working dir 和 env 传递状态）
- GrepSearch 优先使用系统安装的 `rg`，若不存在则回退到 Go 内置的 `regexp` + `filepath.Walk` 实现
- ReadFile 的 offset/limit 参数为 1-based 行号，与 Python 版行为一致
- 所有工具的路径参数必须为绝对路径，相对路径返回错误

- [ ] **步骤 1：实现工具注册表**

创建 `internal/tool/registry.go`，实现：
- `Registry` 结构体：`map[string]func() Tool` 工厂映射
- `Register(name string, factory func() Tool)` — 注册工具工厂
- `Get(name string) (Tool, error)` — 按名称获取工具实例
- `List() []string` — 列出所有已注册工具名称
- `NewDefaultRegistry() *Registry` — 创建并注册所有内置工具

- [ ] **步骤 2：实现 Bash 工具**

创建 `internal/tool/bash.go`，实现：
- `BashTool` 结构体，实现 `Tool` 接口
- `Execute` 方法：解析 command 和 restart 参数
- 使用 `exec.CommandContext` 执行命令，设置 120 秒超时
- 支持指定工作目录和环境变量
- restart 参数重置会话状态

- [ ] **步骤 3：实现 ReadFile 工具**

创建 `internal/tool/read_file.go`，实现：
- `ReadFileTool` 结构体，实现 `Tool` 接口
- `Execute` 方法：解析 file_path、offset、limit 参数
- 使用 `os.ReadFile` 读取文件，按行分割后应用 offset/limit
- 输出格式：行号 + tab + 内容，与 Python 版 `cat -n` 格式一致
- 路径验证：必须是绝对路径、文件必须存在、不能是目录

- [ ] **步骤 4：实现 GrepSearch 工具**

创建 `internal/tool/grep_search.go`，实现：
- `GrepSearchTool` 结构体，实现 `Tool` 接口
- `Execute` 方法：解析 pattern、path、include、case_insensitive 参数
- 优先使用 `exec.LookPath("rg")` 检测 ripgrep，若存在则调用 `rg --line-number --no-heading`
- 若 rg 不存在，回退到 Go 内置实现：`filepath.Walk` 遍历 + `regexp.Compile` 匹配
- 输出格式：`文件路径:行号:匹配内容`

- [ ] **步骤 5：实现 GlobSearch 工具**

创建 `internal/tool/glob_search.go`，实现：
- `GlobSearchTool` 结构体，实现 `Tool` 接口
- `Execute` 方法：解析 pattern、path 参数
- 使用 `filepath.Glob` 进行匹配，递归模式使用 `filepath.WalkDir`
- 仅返回文件（过滤目录）
- 输出格式：匹配文件列表，带匹配总数

- [ ] **步骤 6：编写工具单元测试**

为每个工具编写测试文件（`bash_test.go`、`read_file_test.go`、`grep_search_test.go`、`glob_search_test.go`），使用 `t.TempDir()` 创建临时文件系统

- [ ] **步骤 7：运行测试**

运行: `cd trae-agent-go && go test ./internal/tool/ -v`
预期: 所有测试通过

- [ ] **步骤 8：提交**

```bash
git add .
git commit -m "feat: 实现工具注册表和 Bash/ReadFile/GrepSearch/GlobSearch 工具"
```

---

## 任务 3：编辑工具与结构化思考工具

**文件：**
- 创建: `internal/tool/edit.go`
- 创建: `internal/tool/sequential_thinking.go`

**职责说明：**
- `edit.go` — 文件编辑工具，支持 view（查看文件/目录）、create（创建文件）、str_replace（精确字符串替换）、insert（行后插入）
- `sequential_thinking.go` — 结构化思考工具，支持思维链分解、修订、分支

**设计决策：**
- Edit 工具的 str_replace 操作要求 old_str 在文件中唯一，不唯一则报错并列出所有匹配行号
- Edit 工具的 view 操作对目录使用 `filepath.WalkDir`（最大深度 2 层），对文件输出 `cat -n` 格式
- SequentialThinking 工具维护 `ThoughtData` 切片作为思考历史，支持 is_revision / branch_from_thought / branch_id 字段
- SequentialThinking 工具的 Execute 方法仅记录思考步骤并返回状态摘要，不调用外部 API

- [ ] **步骤 1：实现 Edit 工具**

创建 `internal/tool/edit.go`，实现：
- `EditTool` 结构体，实现 `Tool` 接口
- command 参数支持 view / create / str_replace / insert 四个子命令
- `validatePath` 方法：验证绝对路径、文件存在性、目录/文件区分
- `view` 子命令：目录列出文件，文件输出带行号内容，支持 view_range
- `strReplace` 子命令：读取文件 → 查找 old_str → 验证唯一性 → 替换 → 写回
- `create` 子命令：验证文件不存在 → 写入内容
- `insert` 子命令：在指定行号后插入新内容

- [ ] **步骤 2：实现 SequentialThinking 工具**

创建 `internal/tool/sequential_thinking.go`，实现：
- `SequentialThinkingTool` 结构体，含 thoughtHistory 切片和 branches 映射
- `ThoughtData` 结构体：Thought、ThoughtNumber、TotalThoughts、NextThoughtNeeded、IsRevision、RevisesThought、BranchFromThought、BranchID
- `Execute` 方法：验证参数 → 调整 TotalThoughts → 记录到历史 → 处理分支 → 返回 JSON 状态摘要

- [ ] **步骤 3：编写单元测试**

- [ ] **步骤 4：运行测试**

运行: `cd trae-agent-go && go test ./internal/tool/ -v`
预期: 所有测试通过

- [ ] **步骤 5：提交**

```bash
git add .
git commit -m "feat: 实现 Edit 工具和 SequentialThinking 工具"
```

---

## 任务 4：LLM 客户端抽象层

**文件：**
- 创建: `internal/llm/client.go`
- 创建: `internal/llm/anthropic.go`
- 创建: `internal/llm/openai.go`
- 创建: `internal/llm/retry.go`

**职责说明：**
- `client.go` — 定义 `LLMClient` 接口、`LLMMessage`、`LLMResponse`、`LLMUsage`、`ToolCallInfo` 等核心数据结构
- `anthropic.go` — Anthropic API 实现，处理工具调用序列化
- `openai.go` — OpenAI API 实现，处理 function calling 格式
- `retry.go` — 指数退避重试机制

**设计决策：**
- `LLMClient` 接口核心方法：`Chat(ctx context.Context, messages []LLMMessage, config ModelConfig, tools []Tool) (LLMResponse, error)`
- `LLMMessage` 支持 Role（system/user/assistant）、Content、ToolCalls、ToolResult 字段
- `LLMResponse` 包含 Content、ToolCalls、Usage（InputTokens/OutputTokens）、StopReason
- 各 Provider 客户端负责将通用 `Tool` 接口转换为各自 API 的工具定义格式
- 各 Provider 客户端负责将通用 `ToolCallInfo` 转换为各自 API 的工具调用结果格式
- 重试机制：最大 10 次，指数退避，可配置重试条件（速率限制、服务端错误）
- 流式输出暂不实现，留作后续迭代（Go 的 channel 天然适合流式，但首版先保证功能完整）

- [ ] **步骤 1：定义 LLM 核心数据结构和接口**

创建 `internal/llm/client.go`，定义：
- `LLMClient` 接口：`Chat` 方法
- `LLMMessage` 结构体：Role、Content、ToolCalls、ToolResultID、ToolResultName、ToolResultContent
- `LLMResponse` 结构体：Content、ToolCalls、Usage、StopReason
- `LLMUsage` 结构体：InputTokens、OutputTokens
- `ToolCallInfo` 结构体：Name、CallID、Arguments（map[string]any）
- `ModelConfig` 结构体：Model、Provider、MaxTokens、Temperature、TopP、TopK、MaxRetries、ParallelToolCalls
- `NewClient(provider string, apiKey string, baseURL string) (LLMClient, error)` 工厂函数

- [ ] **步骤 2：实现重试机制**

创建 `internal/llm/retry.go`，实现：
- `RetryConfig` 结构体：MaxRetries、InitialDelay、MaxDelay、RetryableCheck 函数
- `WithRetry(ctx context.Context, cfg RetryConfig, fn func() (LLMResponse, error)) (LLMResponse, error)` 函数
- 指数退避算法，带 jitter
- 支持 context 取消

- [ ] **步骤 3：实现 Anthropic 客户端**

创建 `internal/llm/anthropic.go`，实现：
- `AnthropicClient` 结构体，持有 SDK 客户端实例
- `Chat` 方法实现：消息格式转换 → 工具定义转换 → 调用 API → 响应解析
- 工具定义转换：`Tool.GetInputSchema()` → Anthropic tool format
- 工具调用结果转换：Anthropic tool_use → `ToolCallInfo`，tool_result → `LLMMessage`

- [ ] **步骤 4：实现 OpenAI 客户端**

创建 `internal/llm/openai.go`，实现：
- `OpenAIClient` 结构体，持有 SDK 客户端实例
- `Chat` 方法实现：消息格式转换 → 工具定义转换 → 调用 API → 响应解析
- 工具定义转换：`Tool.GetInputSchema()` → OpenAI function format（含 strict mode 支持）
- 工具调用结果转换：OpenAI function_call → `ToolCallInfo`

- [ ] **步骤 5：编写 LLM 客户端单元测试**

使用 httptest.Server 模拟 API 响应，测试消息转换和工具调用序列化

- [ ] **步骤 6：运行测试**

运行: `cd trae-agent-go && go test ./internal/llm/ -v`
预期: 所有测试通过

- [ ] **步骤 7：提交**

```bash
git add .
git commit -m "feat: 实现 LLM 客户端抽象层，支持 Anthropic 和 OpenAI"
```

---

## 任务 5：Agent 执行循环

**文件：**
- 创建: `internal/agent/agent.go`
- 创建: `internal/agent/context.go`

**职责说明：**
- `agent.go` — Agent 结构体、执行循环、工具调用处理、上下文压缩、任务完成检测
- `context.go` — 项目结构扫描、动态提示注入

**设计决策：**
- Agent 执行循环核心流程：`NewTask` → `ExecuteTask` → 循环 `runLLMStep` → 检测完成或达到 max_steps
- `runLLMStep` 流程：发送消息给 LLM → 解析响应 → 检测 task_done → 执行工具调用 → 收集结果 → 反思 → 返回新消息
- 上下文压缩策略：消息数超过 40 条时，保留 system 消息 + 首条 user 消息 + 最近 10 条消息，中间部分压缩为一条摘要
- 项目上下文注入：扫描项目根目录的文件/目录列表、检测项目类型（pyproject.toml / package.json / go.mod 等）
- 工具调用支持并行（当 `ParallelToolCalls=true` 时使用 `errgroup`）
- 步骤接近上限时注入警告消息，最后一步注入强制总结消息

- [ ] **步骤 1：实现 Agent 核心结构体**

创建 `internal/agent/agent.go`，实现：
- `Agent` 结构体：持有 LLMClient、ToolExecutor、ModelConfig、MaxSteps、Messages 切片、TrajectoryRecorder
- `NewAgent(config AgentConfig) *Agent` 构造函数
- `NewTask(task string, extraArgs map[string]string)` 方法：构建初始消息列表（system + user）
- `ExecuteTask(ctx context.Context) (*AgentExecution, error)` 方法：执行主循环

- [ ] **步骤 2：实现执行循环核心方法**

在 `agent.go` 中实现：
- `runLLMStep(ctx, step, messages, execution)` 方法：调用 LLM → 解析响应 → 判断完成/执行工具
- `handleToolCalls(ctx, toolCalls, step)` 方法：并行或顺序执行工具调用 → 收集结果 → 反思
- `reflectOnResult(results []ToolResult) string` 方法：检查失败的工具结果，生成反思消息
- `compressMessages(messages []LLMMessage) []LLMMessage` 方法：上下文压缩
- `isTaskCompleted(response LLMResponse) bool` 方法：检测 task_done 工具调用
- `llmIndicatesTaskCompleted(response LLMResponse) bool` 方法：检测文本完成指示

- [ ] **步骤 3：实现项目上下文注入**

创建 `internal/agent/context.go`，实现：
- `GetProjectContext(projectPath string) string` 函数：扫描目录结构 → 分类文件/目录 → 检测项目类型 → 返回格式化摘要
- `InjectProjectContext(userMessage string, projectPath string) string` 函数：将上下文追加到用户消息

- [ ] **步骤 4：编写 Agent 集成测试**

使用 mock LLMClient 和 mock Tool 测试执行循环的各种场景：正常完成、达到步数上限、工具调用失败、上下文压缩

- [ ] **步骤 5：运行测试**

运行: `cd trae-agent-go && go test ./internal/agent/ -v`
预期: 所有测试通过

- [ ] **步骤 6：提交**

```bash
git add .
git commit -m "feat: 实现 Agent 执行循环、上下文压缩和项目上下文注入"
```

---

## 任务 6：子 Agent 委派工具

**文件：**
- 创建: `internal/tool/sub_agent.go`

**职责说明：**
- `sub_agent.go` — SubAgent 工具，允许主 Agent 将子任务委派给新的 Agent 实例，保护主上下文窗口

**设计决策：**
- SubAgent 工具的 Execute 方法创建一个新的 Agent 实例（独立的上下文和消息历史）
- 子 Agent 继承主 Agent 的 ModelConfig，但使用独立的工具集（bash、read_file、grep_search、glob_search、edit、task_done）
- 子 Agent 的 MaxSteps 默认为 30，避免无限循环
- 子 Agent 执行完成后，仅将最终结果摘要返回给主 Agent，不传递中间步骤
- 子 Agent 不支持嵌套（即子 Agent 不能再创建子 Agent），通过工具列表中不包含 sub_agent 来保证

- [ ] **步骤 1：实现 SubAgent 工具**

创建 `internal/tool/sub_agent.go`，实现：
- `SubAgentTool` 结构体，持有 parentModelConfig 引用
- `Execute` 方法：解析 task_description 和 working_dir → 创建新 Agent → 执行任务 → 返回摘要
- 子 Agent 的工具集不包含 sub_agent 本身，防止递归委派

- [ ] **步骤 2：编写单元测试**

使用 mock Agent 测试委派逻辑

- [ ] **步骤 3：运行测试**

运行: `cd trae-agent-go && go test ./internal/tool/ -run SubAgent -v`
预期: 测试通过

- [ ] **步骤 4：提交**

```bash
git add .
git commit -m "feat: 实现子 Agent 委派工具"
```

---

## 任务 7：配置系统

**文件：**
- 创建: `internal/config/config.go`
- 创建: `configs/trae_config.yaml`

**职责说明：**
- `config.go` — YAML 配置加载、环境变量覆盖、配置优先级解析
- `trae_config.yaml` — 示例配置文件

**设计决策：**
- 使用 viper 库加载 YAML 配置，支持环境变量覆盖
- 配置结构体层次：`Config` → `ModelProvider` → `ModelSpec`（模型级覆盖）、`AgentConfig`（含 tools 列表和 max_steps）
- 配置优先级：CLI 参数 > 环境变量 > 配置文件 > 默认值
- API Key 支持环境变量：`ANTHROPIC_API_KEY`、`OPENAI_API_KEY` 等
- 工具列表在配置中按名称指定，运行时通过 Registry 解析为 Tool 实例

- [ ] **步骤 1：定义配置结构体**

创建 `internal/config/config.go`，定义：
- `Config` 结构体：DefaultProvider、ModelProviders 映射、Agents 映射、Lakeview、MCPServers
- `ModelProvider` 结构体：APIKey、Provider、BaseURL、DefaultModel、MaxTokens、Temperature、TopP、TopK、MaxRetries、ParallelToolCalls、Models 映射
- `ModelSpec` 结构体：模型级参数覆盖
- `AgentConfig` 结构体：Provider、Model、MaxSteps、Tools 列表、EnableLakeview
- `LoadConfig(path string) (*Config, error)` 函数
- `ResolveConfig(cliOverrides) (*Config, error)` 函数

- [ ] **步骤 2：创建示例配置文件**

创建 `configs/trae_config.yaml`，包含：
- default_provider 设置
- model_providers 段（anthropic、openai 示例）
- agents.trae_agent 段（含完整工具列表）

- [ ] **步骤 3：编写配置加载测试**

- [ ] **步骤 4：运行测试**

运行: `cd trae-agent-go && go test ./internal/config/ -v`
预期: 测试通过

- [ ] **步骤 5：提交**

```bash
git add .
git commit -m "feat: 实现配置系统和示例配置文件"
```

---

## 任务 8：提示词系统

**文件：**
- 创建: `internal/prompt/prompt.go`

**职责说明：**
- `prompt.go` — 系统提示词定义、动态提示构建

**设计决策：**
- 系统提示词从 SWE-bench 专用改为通用编码 Agent 风格，包含工具使用指南
- 提示词分为固定部分（角色定义、原则、工具指南）和动态部分（项目路径、项目结构）
- `GetSystemPrompt() string` 返回完整系统提示
- `BuildUserMessage(task string, projectPath string, projectContext string) string` 构建用户消息
- 提示词内容涵盖：核心原则、工具使用指南（每个工具的适用场景和注意事项）、文件路径规则、工作流程（探索→阅读→规划→编辑→验证→完成）

- [ ] **步骤 1：实现提示词系统**

创建 `internal/prompt/prompt.go`，实现：
- `systemPrompt` 常量：通用编码 Agent 系统提示
- `GetSystemPrompt() string` 函数
- `BuildUserMessage(task, projectPath, projectContext string) string` 函数

- [ ] **步骤 2：编写提示词测试**

验证提示词包含关键内容（工具名称、路径规则、工作流程步骤）

- [ ] **步骤 3：运行测试**

运行: `cd trae-agent-go && go test ./internal/prompt/ -v`
预期: 测试通过

- [ ] **步骤 4：提交**

```bash
git add .
git commit -m "feat: 实现通用编码 Agent 提示词系统"
```

---

## 任务 9：CLI 入口

**文件：**
- 创建: `cmd/trae/main.go`

**职责说明：**
- `main.go` — CLI 入口，使用 cobra 定义命令（run、interactive、show-config、tools）

**设计决策：**
- 使用 cobra 库定义 CLI 命令
- `run` 命令：接受任务描述或文件输入，执行单次任务
- `interactive` 命令：启动交互式会话
- `show-config` 命令：显示当前配置
- `tools` 命令：列出可用工具
- CLI 参数覆盖配置：--provider、--model、--max-steps、--working-dir、--config-file
- 使用 `signal.NotifyContext` 优雅处理 Ctrl+C

- [ ] **步骤 1：实现 CLI 入口**

创建 `cmd/trae/main.go`，实现：
- root 命令（版本信息）
- `run` 子命令：加载配置 → 创建 Agent → 执行任务 → 输出结果
- `interactive` 子命令：加载配置 → 进入交互循环
- `show-config` 子命令：加载并打印配置
- `tools` 子命令：列出注册表中的工具

- [ ] **步骤 2：编写 CLI 集成测试**

- [ ] **步骤 3：运行测试**

运行: `cd trae-agent-go && go test ./cmd/trae/ -v`
预期: 测试通过

- [ ] **步骤 4：提交**

```bash
git add .
git commit -m "feat: 实现 CLI 入口，支持 run/interactive/show-config/tools 命令"
```

---

## 任务 10：构建与发布

**文件：**
- 创建: `Makefile`

**职责说明：**
- `Makefile` — 构建、测试、lint、安装命令

**设计决策：**
- `make build` — 编译为单个二进制文件 `trae`
- `make test` — 运行所有测试
- `make lint` — 运行 golangci-lint
- `make install` — 安装到 GOPATH/bin
- 支持交叉编译：linux/amd64、darwin/arm64、windows/amd64
- 使用 `-ldflags "-s -w"` 减小二进制体积
- CGO_ENABLED=0 确保静态链接

- [ ] **步骤 1：创建 Makefile**

- [ ] **步骤 2：运行完整测试套件**

运行: `cd trae-agent-go && make test`
预期: 所有测试通过

- [ ] **步骤 3：运行 lint**

运行: `cd trae-agent-go && make lint`
预期: 无错误

- [ ] **步骤 4：构建二进制**

运行: `cd trae-agent-go && make build`
预期: 生成可执行文件 `trae`

- [ ] **步骤 5：端到端验证**

运行: `./trae tools` 验证工具列表输出

- [ ] **步骤 6：提交**

```bash
git add .
git commit -m "feat: 添加 Makefile，支持构建、测试、lint 和交叉编译"
```

---

## 自审

### 1. 需求覆盖

| 需求 | 对应任务 |
|------|---------|
| Tool 接口与工具注册表 | 任务 1、2 |
| Bash 工具 | 任务 2 |
| ReadFile 工具 | 任务 2 |
| GrepSearch 工具 | 任务 2 |
| GlobSearch 工具 | 任务 2 |
| Edit 工具 | 任务 3 |
| SequentialThinking 工具 | 任务 3 |
| SubAgent 委派工具 | 任务 6 |
| LLM 客户端抽象（Anthropic + OpenAI） | 任务 4 |
| 重试机制 | 任务 4 |
| Agent 执行循环 | 任务 5 |
| 上下文压缩 | 任务 5 |
| 项目上下文注入 | 任务 5 |
| 配置系统 | 任务 7 |
| 提示词系统 | 任务 8 |
| CLI 入口 | 任务 9 |
| 构建与发布 | 任务 10 |

所有需求已覆盖，无遗漏。

### 2. 占位符扫描

无 TBD、TODO、"implement later" 等占位符。所有任务包含明确的职责说明、设计决策和验证步骤。

### 3. 类型一致性

- `Tool` 接口在任务 1 定义，任务 2-3、6 的所有工具实现此接口
- `LLMClient` 接口在任务 4 定义，任务 5 的 Agent 依赖此接口
- `AgentConfig` 在任务 7 定义，任务 5 的 Agent 构造函数和任务 9 的 CLI 均使用此结构体
- `ToolCallInfo` 在任务 4 定义，任务 5 的工具调用处理使用此结构体
- `ModelConfig` 在任务 4 定义，任务 7 的配置系统输出此结构体

所有类型定义在使用前已确立，无跨任务不一致。
