# 架构设计

## 顶层架构

```
┌─────────────────────────────────────────────────────────────┐
│                        CLI Entry (cmd/trae)                  │
│                   flag 解析 / 子命令分发                       │
└──────────────┬──────────────────────────────────┬───────────┘
               │                                  │
               ▼                                  ▼
┌──────────────────────────┐         ┌─────────────────────────┐
│      REPL Loop           │         │    Headless Runner      │
│  (interactive mode)      │         │   (run / CI mode)       │
│  - 流式渲染               │         │  - 一次性执行            │
│  - 斜杠命令               │         │  - JSON 输出            │
│  - Ctrl+C 中断            │         │                         │
└────────────┬─────────────┘         └────────────┬────────────┘
             │                                    │
             └──────────────┬─────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                      Agent Runtime                           │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │ Agent Loop  │  │ Context Mgr │  │  Sub-agent Manager  │  │
│  │ (think/act) │  │ (compact)   │  │  (Task tool)        │  │
│  └──────┬──────┘  └─────────────┘  └─────────────────────┘  │
│         │                                                     │
│         ▼                                                     │
│  ┌─────────────────────────────────────────────────────┐    │
│  │              Tool Dispatcher                         │    │
│  │   (parallel / sequential, permission gate)          │    │
│  └──────┬──────────────────────────────────────────────┘    │
└─────────┼───────────────────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────────────────┐
│                      Tool Layer                              │
│  read │ write │ edit │ glob │ grep │ bash │ task             │
│  ─────────────────────────────────────────────────────────  │
│              MCP Tools (动态发现)                             │
└─────────────────────────────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────────────────┐
│                    LLM Provider Layer                        │
│  anthropic │ openai │ (google/openrouter/ollama 规划中)      │
│  (统一 StreamEvent 抽象, 统一 tool_call 格式)                 │
└─────────────────────────────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────────────────┐
│              Infra / Cross-cutting                           │
│  config │ session store │ trajectory │ permission │ logger   │
└─────────────────────────────────────────────────────────────┘
```

## 目录结构

```
trae-agent/                      # 仓库根 = Go module 根
├── go.mod                       # module github.com/bytedance/trae-agent
├── go.sum
├── cmd/
│   └── trae/
│       └── main.go              # CLI 入口
├── internal/
│   ├── agent/                   # Agent Runtime
│   │   ├── loop.go              # think-act 循环 + Event 类型
│   │   ├── context.go           # 上下文管理 / token 计数
│   │   ├── compact.go           # compact 实现
│   │   ├── prompt.go            # system prompt
│   │   └── subagent.go          # 子 agent 委派 (SubagentRunner)
│   ├── cli/
│   │   ├── root.go              # cobra 根命令 + config 加载
│   │   ├── run.go               # run 子命令 + buildAgent
│   │   ├── interactive.go       # interactive 子命令
│   │   ├── repl.go              # 交互式 REPL + Asker 实现
│   │   ├── render.go            # 流式渲染
│   │   ├── command.go           # 斜杠命令注册与分发
│   │   ├── interrupt.go         # Ctrl+C 处理
│   │   ├── permissions_cmd.go   # /permissions 命令
│   │   ├── mcp_cmd.go           # /mcp 命令
│   │   ├── show_config.go       # show-config 子命令
│   │   └── version.go           # version 子命令
│   ├── headless/
│   │   └── runner.go            # 非交互执行
│   ├── tool/
│   │   ├── tool.go              # Tool 接口
│   │   ├── dispatcher.go        # 并行/串行调度 + 权限门
│   │   ├── read.go
│   │   ├── write.go
│   │   ├── edit.go
│   │   ├── glob.go
│   │   ├── grep.go
│   │   ├── bash.go
│   │   ├── todo.go              # TodoWrite 工具 (任务清单)
│   │   └── task.go              # Task 工具 (子 agent 派发)
│   ├── llm/
│   │   ├── provider.go          # Provider 接口 + StreamEvent
│   │   ├── anthropic.go         # Anthropic SSE 流式
│   │   ├── openai.go            # OpenAI SSE 流式
│   │   └── retry.go             # 指数退避重试
│   ├── mcp/
│   │   ├── client.go            # MCP 客户端 (JSON-RPC over stdio)
│   │   ├── protocol.go          # MCP 协议类型
│   │   ├── tool.go              # MCPTool 适配器
│   │   └── manager.go           # 多 server 生命周期管理
│   ├── config/
│   │   ├── config.go            # 配置结构 + 默认值
│   │   └── loader.go            # YAML 多级加载与合并
│   ├── session/
│   │   └── store.go             # 会话持久化 + resume
│   ├── permission/
│   │   ├── policy.go            # 权限策略评估 + 内置规则
│   │   ├── store.go             # ~/.trae/permissions.json 持久化
│   │   └── asker.go             # Asker 接口 + AutoAsker/CallbackAsker
│   ├── trajectory/
│   │   └── recorder.go          # JSONL 轨迹记录
│   ├── cost/
│   │   └── pricing.go           # 模型价格表 + 费用估算
│   └── logger/
│       └── logger.go            # slog 封装
├── docs/                        # 设计文档
│   └── design/
├── Makefile
├── .goreleaser.yml              # 跨平台发布配置
└── README.md
```

> **注**：`pkg/`（公共 SDK）和 `test/`（集成/e2e 测试目录）为规划中，当前未创建。e2e 测试暂放在 `internal/cli/e2e_test.go`。

## 核心抽象

### Provider 接口

```go
type StreamEvent interface{ isStreamEvent() }

type TextDelta struct{ Content string }
type ToolCallDelta struct{ ID, Name string; ArgsPartial string }
type Done struct{ Usage Usage; StopReason string }
type Error struct{ Err error }

type Provider interface {
    Name() string
    Stream(ctx context.Context, req Request) (<-chan StreamEvent, error)
    // 非流式，内部基于 Stream 实现
}
```

所有 provider 输出统一的事件流，上层不感知各厂商差异。

### Tool 接口

```go
type Tool interface {
    Name() string
    Schema() jsonschema.Schema   // 暴露给 LLM 的参数 schema
    Run(ctx context.Context, args json.RawMessage) Result
}

type Result struct {
    Content string  // 文本结果
    IsError bool
    Meta    map[string]any  // 行号、文件路径等元信息
}
```

### Agent Loop

```go
type Agent struct {
    provider    llm.Provider
    tools       []tool.Tool
    dispatcher  *tool.Dispatcher
    context     *ContextManager
    messages    []Message
    // ...
}

func (a *Agent) Step(ctx context.Context) (Event, error) {
    // 1. 调用 provider.Stream，流式输出 text + tool_calls
    // 2. 渲染 text delta 到 UI
    // 3. 收集完整 tool_calls
    // 4. dispatcher 并行执行工具（过权限门）
    // 5. 工具结果回灌 messages
    // 6. 检查 task_done / 步数上限 / compact 触发
}
```

### REPL 主循环

```go
func (r *REPL) Run() error {
    for {
        line, err := r.readLine()  // 支持多行、补全
        if err == io.EOF { return nil }
        if strings.HasPrefix(line, "/") {
            r.dispatchCommand(line)
            continue
        }
        // 进入 agent step 循环，可被 Ctrl+C 中断
        ctx, cancel := context.WithCancel(r.ctx)
        r.setCurrentCancel(cancel)
        r.agent.Run(ctx, line)
    }
}
```

## 并发模型

- **主循环**：单 goroutine，串行处理用户输入
- **LLM 流式**：provider 在后台 goroutine 写 channel，主循环读
- **工具并行**：dispatcher 用 `errgroup` 并行执行同一批 tool_calls
- **子 agent**：每个 Task 调用起独立 goroutine，结果通过 channel 汇总
- **中断**：`context.Cancel` 传播到 provider 和工具，工具需响应 ctx.Done

## 配置层级

优先级从高到低：

1. CLI flag
2. 环境变量（`TRAE_*`）
3. 项目级 `.trae/config.yaml`
4. 用户级 `~/.trae/config.yaml`
5. 内置默认值

## 会话持久化

- 路径：`~/.trae/sessions/<uuid>.json`
- 内容：messages、tool calls、agent state、metadata（provider/model/token/cost）
- 写入时机：每步结束增量写
- resume：加载 JSON 重建 messages，恢复 agent state

## 权限评估流程

```
tool_call 进入 dispatcher
    ↓
policy.Evaluate(tool, args) → {Allow | Ask | Deny}
    ↓
Allow → 执行
Ask   → UI 弹确认 → 用户选 allow-once / allow-always / deny
Deny  → 返回错误给 LLM
```

策略文件 `~/.trae/permissions.json`，规则按工具维度组织：

```json
{
  "bash": {
    "deny": ["rm -rf /", "git push --force"],
    "ask": ["git push", "docker *"],
    "allow": ["ls *", "cat *", "grep *"]
  },
  "edit": {
    "allow": ["./**"],
    "ask": ["/etc/**", "~/.ssh/**"]
  }
}
```

## 跨平台

- 目标：macOS / Linux / WSL
- 不支持原生 Windows（WSL 可用）
- 文件路径用 `filepath` 包，避免硬编码分隔符
- 终端能力探测：`termenv` / `isatty`
