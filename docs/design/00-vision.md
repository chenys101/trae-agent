# 愿景与目标

## 一句话愿景

用 Go 语言从零实现一个达到 Claude Code、Cursor CLI 同等水平的命令行编码 Agent，作为 trae-agent 的下一代实现。

## 为什么要重写

现有 Python 实现（`trae_agent/`）存在以下结构性限制：

1. **交互体验上限低**：当前 REPL 基于同步请求-响应，无法做到流式输出、可中断、低延迟反馈，而 Claude Code / Cursor CLI 的核心体验正是流式 + 可中断。
2. **并发模型受限**：Python 的 GIL + asyncio 在并行工具调用、子 agent 委派、后台任务等场景下表达力不足，难以支撑 Claude Code 的 Task 并行子任务模式。
3. **分发与依赖**：Python 依赖 uv/venv/pip 生态，安装链路长；Go 可编译为单二进制，零依赖分发，更贴近 CLI 工具用户预期。
4. **性能与资源**：长会话上下文管理、token 计数、大文件读取在 Python 下开销高，Go 的内存模型和 goroutine 更适合常驻 CLI 进程。

## 目标用户

- 在终端工作的开发者（macOS / Linux / WSL）
- 需要在大型代码库中做复杂多步改造的工程师
- 希望可扩展、可脚本化、可嵌入 CI 的 Agent 用户

## 核心目标（必须达到）

| # | 目标 | 验收标准 |
|---|------|----------|
| G1 | 单二进制分发 | `go build` 产出一个无外部依赖的可执行文件 |
| G2 | 流式交互式 REPL | 输出逐 token 流式渲染，`Ctrl+C` 可中断当前步骤 |
| G3 | 斜杠命令系统 | 内置 `/help` `/clear` `/compact` `/model` `/resume` `/agents` 等 |
| G4 | 内置工具集 | read / write / edit / glob / grep / bash / task 内置（TodoWrite 规划中） |
| G5 | 子 agent 委派 | Task 工具可派发独立子 agent 并行执行，结果回汇 |
| G6 | 多 LLM Provider | OpenAI / Anthropic 已实现；Google / OpenRouter / Ollama 规划中（兼容 OpenAI 协议端点可用） |
| G7 | 上下文管理 | 自动 compact 长对话、token 预算感知、会话持久化与 resume |
| G8 | 权限与安全 | 工具调用审批、危险命令拦截、文件读写白名单 |
| G9 | MCP 支持 | 兼容 Model Context Protocol，可挂载第三方 MCP server |
| G10 | 可扩展 | 工具、agent、provider、命令均可通过接口扩展，无需改核心 |

## 非目标（本期不做）

- GUI / TUI 全屏应用（保持 CLI 流式文本交互）
- IDE 插件集成（留给后续）
- 自训练模型 / 本地推理（只对接外部 LLM API）
- 团队协作 / 多用户（单用户单机工具）

## 历史

本项目前身为 Python 实现（`trae_agent/`，保留在 `main` 分支），因交互体验、并发模型、分发方式的根本性限制，决定用 Go 从零重写。Go 实现位于 `solo-go` 分支，不保留、不兼容、不共存 Python 代码。

Go 重写时可参考原 Python 实现的设计思路（prompt 设计、工具语义、轨迹记录），但代码全部重写。

## 命名

- 项目代号：**trae-agent**
- 二进制名：`trae`（CLI 入口）
- Go module path：`github.com/bytedance/trae-agent`

## 成功指标

- 在 SWE-bench 风格的本地任务上，成功率不低于原 Python 实现
- 冷启动到首 token < 500ms（不含 LLM 网络延迟）
- 单二进制 < 30MB（静态编译，不含 cgo）
- 长会话（100+ 轮）内存稳定，无泄漏
