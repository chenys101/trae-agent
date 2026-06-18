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
│  read │ write │ edit │ glob │ grep │ bash │ todo │ task      │
│  ─────────────────────────────────────────────────────────  │
│              MCP Tools (动态发现)                             │
└─────────────────────────────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────────────────┐
│                    LLM Provider Layer                        │
│  anthropic │ openai │ google │ openrouter │ ollama │ custom  │
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
│   │   ├── agent.go             # Agent 接口与基础实现
│   │   ├── loop.go              # think-act 循环
│   │   ├── context.go           # 上下文管理 / compact
│   │   ├── subagent.go          # 子 agent 委派
│   │   └── state.go             # 会话状态
│   ├── cli/
│   │   ├── repl.go              # 交互式 REPL
│   │   ├── render.go            # 流式渲染
│   │   ├── command.go           # 斜杠命令注册与分发
│   │   ├── interrupt.go         # Ctrl+C 处理
│   │   └── completion.go        # 命令补全
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
│   │   ├── todo.go
│   │   └── task.go
│   ├── llm/
│   │   ├── provider.go          # Provider 接口 + StreamEvent
│   │   ├── anthropic.go
│   │   ├── openai.go
│   │   ├── google.go
│   │   ├── openrouter.go
│   │   ├── ollama.go
│   │   └── retry.go
│   ├── mcp/
│   │   ├── client.go            # MCP 客户端
│   │   └── registry.go          # MCP 工具注册
│   ├── config/
│   │   ├── config.go            # 配置加载与合并
│   │   └── schema.go            # 配置 schema
│   ├── session/
│   │   ├── store.go             # 会话持久化
│   │   └── resume.go            # 会话恢复
│   ├── permission/
│   │   └── policy.go            # 权限策略评估
│   ├── trajectory/
│   │   └── recorder.go          # 轨迹记录
│   └── logger/
│       └── logger.go
├── pkg/                         # 可对外暴露的公共库
│   ├── api/                     # SDK 入口（headless 调用）
│   └── types/                   # 共享类型
├── test/
│   ├── integration/
│   └── e2e/
├── docs/                        # 设计文档
│   └── design/                  # 从 docs/v2-go/ 迁移
├── Makefile
└── .golangci.yml
```

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
