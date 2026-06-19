# 能力对标：vs Claude Code / Cursor CLI

本文档逐项对标 Claude Code 与 Cursor CLI 的核心能力，明确必须达到的基线，以及差异化方向。

## 对标总览

| 能力域 | Claude Code | Cursor CLI | trae-agent 目标 | 优先级 |
|--------|------------|------------|------------------|--------|
| 流式输出 | ✅ 逐 token | ✅ 逐 token | ✅ 必须达到 | P0 |
| 可中断 | ✅ Ctrl+C / ESC | ✅ | ✅ 必须达到 | P0 |
| 斜杠命令 | ✅ 丰富 | ✅ | ✅ 必须达到 | P0 |
| 内置工具 | ✅ read/write/edit/glob/grep/bash | ✅ | ✅ 必须达到 | P0 |
| TodoWrite | ✅ 内置 | ⚠️ 部分 | ✅ 必须达到 | P0 |
| 子 agent (Task) | ✅ 并行 | ❌ | ✅ 必须达到 | P1 |
| 自动 compact | ✅ | ✅ | ✅ 必须达到 | P1 |
| 会话 resume | ✅ | ✅ | ✅ 必须达到 | P1 |
| 多 Provider | ⚠️ 主推 Anthropic | ✅ | ✅ 必须达到 | P0 |
| MCP | ✅ | ✅ | ✅ 必须达到 | P1 |
| 权限审批 | ✅ | ✅ | ✅ 必须达到 | P1 |
| 单二进制 | ❌ Node.js | ❌ Node.js | ✅ 差异化 | P0 |
| 配置即代码 | ⚠️ | ⚠️ | ✅ 差异化 | P2 |

## P0 能力详解（MVP 必须达到）

### 1. 流式输出与可中断

**Claude Code 行为**：LLM 回复逐 token 流式渲染到终端，工具调用过程实时显示，`Ctrl+C` 中断当前 LLM 调用或工具执行，回到提示符。

**验收标准**：
- LLM 响应通过 SSE / stream API 接收，每 token 即时打印
- 工具执行期间显示进度（如 `● Running bash...`）
- `Ctrl+C` 一次：中断当前步骤，保留已生成内容，回到提示符
- `Ctrl+C` 两次：退出程序
- 中断后状态一致，不残留半完成的文件写入

### 2. 斜杠命令系统

**Claude Code 内置命令**（参考）：`/help` `/clear` `/compact` `/model` `/resume` `/agents` `/cost` `/init` `/review` `/bug` `/config` `/login` `/logout` `/status` `/memory` `/mcp` `/permissions` `/terminal-setup` `/vim` `/release-notes` `/doctor`

**MVP 命令集**：

| 命令 | 功能 |
|------|------|
| `/help` | 列出所有命令 |
| `/clear` | 清空当前会话上下文 |
| `/compact` | 手动触发上下文压缩 |
| `/model` | 切换 provider / model |
| `/status` | 显示当前配置、token 用量、步数 |
| `/resume` | 列出并恢复历史会话 |
| `/agents` | 列出 / 切换可用 agent 配置 |
| `/mcp` | 列出已挂载 MCP server 及工具 |
| `/permissions` | 查看 / 修改权限策略 |
| `/cost` | 显示累计 token 与估算费用 |
| `/exit` | 退出 |

**验收标准**：
- 输入 `/` 自动补全命令名
- 命令参数支持 `--flag value` 形式
- 未知命令给出建议（模糊匹配）
- 命令执行不进入 LLM 循环

### 3. 内置工具集

**对标 Claude Code 工具语义**：

| 工具 | 语义 | 实现 |
|------|------|---------|
| `Read` | 读文件，支持行号、offset/limit | 内置 |
| `Write` | 写文件（覆盖） | 内置 |
| `Edit` | str_replace 精确替换 | 内置 |
| `Glob` | 文件名 glob 匹配 | 内置 |
| `Grep` | 内容正则搜索（ripgrep 语义） | 内置 |
| `Bash` | 执行 shell 命令，支持超时、后台 | 内置 |
| `TodoWrite` | 任务清单管理 | 内置 |
| `Task` | 派发子 agent（P1） | 内置 |

**验收标准**：
- 每个工具有清晰的 JSON schema，LLM 可正确调用
- 工具结果按 Claude Code 风格格式化（行号、文件链接）
- 支持并行工具调用（同一 LLM 响应中多个 tool_call 并发执行）
- 工具错误友好可读，不暴露原始 stack

### 4. 多 LLM Provider

**必须支持**：
- Anthropic（Claude 系列）
- OpenAI（GPT 系列）
- Google（Gemini）
- OpenRouter（聚合）
- Ollama（本地）
- 任意 OpenAI 兼容端点（`base_url` 可配）

**验收标准**：
- 统一的 `Provider` 接口，新增 provider 只实现接口
- 流式响应统一抽象为 `<-chan StreamEvent`
- 工具调用格式自动适配各 provider 差异
- 配置可在 YAML / 环境变量 / CLI flag 间覆盖

## P1 能力详解（MVP 之后立即补齐）

### 5. 子 agent 委派（Task 工具）

**Claude Code 行为**：主 agent 通过 Task 工具派发独立子 agent，子 agent 有自己的上下文和工具集，并行执行，完成后返回摘要。主 agent 上下文不被子 agent 的细节污染。

**验收标准**：
- `Task` 工具参数：`subagent_type`、`description`、`prompt`
- 子 agent 拥有独立 message history，不共享主上下文
- 多个 Task 调用可并行（goroutine + channel 汇总）
- 子 agent 失败不崩溃主流程，返回错误摘要
- 内置子 agent 类型：`search`（只读探索）、`general-purpose`（完整工具）

### 6. 自动 compact

**Claude Code 行为**：当上下文接近 token 上限时，自动用一次 LLM 调用压缩历史对话，保留任务目标和关键决策，丢弃细节。

**验收标准**：
- 实时统计当前上下文 token 数
- 达到阈值（默认 80%）自动触发 compact
- compact 后注入摘要消息，保留 system prompt 和最近 N 轮
- `/compact` 可手动触发
- compact 事件在 UI 明确提示

### 7. 会话 resume

**验收标准**：
- 每个会话自动持久化到 `~/.trae/sessions/<id>.json`
- `/resume` 列出最近会话（时间、任务摘要、步数）
- 选择后恢复完整 message history 和工具状态
- 支持跨进程 resume（重启后可恢复）

### 8. MCP 支持

**验收标准**：
- 兼容 MCP 规范（stdio / SSE 传输）
- 配置文件声明 MCP server，启动时自动连接并发现工具
- MCP 工具与内置工具统一调度
- `/mcp` 命令查看状态

### 9. 权限与安全

**Claude Code 行为**：工具调用前根据策略决定 allow / ask / deny，危险操作（如 `rm -rf`、写仓库外文件）需用户确认。

**验收标准**：
- 权限策略文件 `~/.trae/permissions.json`
- 策略维度：工具名、命令模式、路径 glob
- 三种决策：`allow` / `ask` / `deny`
- 运行时拦截，ask 时交互式确认
- 危险命令模式内置黑名单（`rm -rf /`、`git push --force` 等）

## P2 差异化能力（后续迭代）

### 10. 配置即代码

- 项目根 `.trae/` 目录存放 agent 配置、prompt、工具声明
- 团队可共享配置，纳入版本控制
- 支持项目级 / 用户级 / 全局三级配置覆盖

### 11. 可观测性

- 内置轨迹记录（兼容现有 JSON 格式或新格式）
- OpenTelemetry trace 导出
- `/cost` 实时费用统计

### 12. 脚本化与 headless

- `trae run --headless "task"` 非交互模式，输出 JSON
- 可作为 CI 步骤嵌入
- stdin 接受管道输入

## 不对标的能力（明确放弃）

- **IDE 内嵌**：Claude Code 有 VS Code 扩展，本期不做
- **全屏 TUI**：保持流式文本，不做 bubbletea 全屏应用
- **多模态输入**：图片 / 截图输入暂不支持
