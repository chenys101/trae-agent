# Trae Agent

Go 实现的 CLI coding agent，对标 Claude Code / Cursor CLI。

## 特性

- **流式 LLM 对话**：Anthropic Claude + OpenAI GPT，SSE 流式输出
- **工具闭环**：read / write / edit / glob / grep / bash，agent 自主决策多步执行
- **交互式 REPL**：readline 编辑、斜杠命令、中断处理、会话保存/恢复
- **上下文管理**：自动 compact、token 计数、会话持久化
- **子 agent**：Task 工具派发独立子 agent，并行处理复杂任务
- **权限系统**：内置危险命令拦截（rm -rf /、mkfs 等），交互式确认，规则持久化
- **MCP 集成**：JSON-RPC 2.0 over stdio，多 server 管理，工具自动发现
- **轨迹记录**：JSONL 格式，完整记录 agent 事件流
- **费用估算**：内置模型价格表，实时 USD 估算
- **Headless 模式**：`--json` 结构化输出，`--trajectory` 轨迹记录

## 安装

```bash
go build -o trae ./cmd/trae
# 或
make build
```

## 配置

配置文件位于 `~/.trae/config.yaml`：

```yaml
default_provider: anthropic
model_providers:
  anthropic:
    api_key: sk-ant-xxx
    default_model: claude-3-5-sonnet-20241022
  openai:
    api_key: sk-xxx
    base_url: https://api.openai.com/v1
    default_model: gpt-4o

mcp_servers:
  filesystem:
    command: npx
    args: ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]

max_steps: 20
log_level: info
```

## 使用

### 交互模式

```bash
trae interactive
```

REPL 命令：`/help` `/exit` `/clear` `/status` `/cost` `/model` `/compact` `/sessions` `/resume` `/permissions` `/mcp`

### 单次运行

```bash
trae run "解释 main.go 的逻辑"
trae run --provider openai --model gpt-4o "重构这个函数"
trae run --json "列出所有 TODO"
trae run --trajectory "修复这个 bug"
```

## 架构

```
cmd/trae/              入口
internal/
  agent/               agent loop、子 agent、上下文管理
  cli/                 cobra 命令、REPL、渲染器、斜杠命令
  config/              YAML 配置加载与合并
  cost/                模型价格表与费用估算
  llm/                 LLM provider、Anthropic/OpenAI SSE、重试
  logger/              slog 封装
  mcp/                 MCP client、tool adapter、manager
  permission/          权限策略、规则存储、asker 接口
  session/             会话存储
  tool/                工具接口、内置工具、dispatcher
  trajectory/          JSONL 轨迹记录器
```

## 开发

```bash
make test          # 测试（含 race detector）
make vet           # go vet
make fmt           # 格式化
make release-snapshot  # 本地测试跨平台编译
```

## License

MIT
