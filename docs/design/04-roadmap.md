# 实施路线图

按里程碑组织，每个里程碑产出可演示的可用版本。MVP 之后每个里程碑独立可发布。

## 里程碑总览

| 里程碑 | 主题 | 产出 | 对应能力 |
|--------|------|------|----------|
| M0 | 骨架 | 可编译空壳 + 配置加载 | G1 |
| M1 | LLM 打通 | 单轮对话流式输出 | G2, G6 |
| M2 | 工具闭环 | read/write/edit/bash + agent loop | G4 |
| M3 | 交互式 REPL | 斜杠命令 + 中断 + 补全 | G2, G3 |
| M4 | 上下文管理 | compact + session resume | G7 |
| M5 | 子 agent | Task 工具 + 并行子任务 | G5 |
| M6 | 权限与安全 | 策略评估 + 审批 | G8 |
| M7 | MCP 集成 | MCP 客户端 + 工具发现 | G9 |
| M8 | 打磨与发布 | 轨迹、cost、headless、release | G10 |

---

## M0：骨架（可编译空壳）

**目标**：建立目录结构、go module、CLI 入口、配置加载，能 `trae --version` 和 `trae show-config`。

**任务**：
1. 初始化 `go.mod`（`github.com/bytedance/trae-agent`）、`cmd/trae/main.go`
2. cobra 根命令 + `version` / `show-config` 子命令
3. `internal/config`：YAML 多级加载（默认 / 用户 / 项目 / flag）
4. `internal/logger`：slog 初始化，写 `~/.trae/logs/`
5. Makefile：`build` / `test` / `lint`
6. `.golangci.yml`
7. 单元测试覆盖 config 加载

**验收**：
- `make build` 产出 `./bin/trae`
- `./bin/trae version` 输出版本
- `./bin/trae show-config` 输出合并后配置
- 配置优先级正确（flag > env > project > user > default）

---

## M1：LLM 打通（单轮流式）

**目标**：`trae run "你好"` 能流式打印 LLM 回复，支持至少 Anthropic + OpenAI 两个 provider。

**任务**：
1. `internal/llm/provider.go`：`Provider` 接口、`StreamEvent` 类型、`Request`/`Usage`
2. `internal/llm/anthropic.go`：SSE 流式 + tool_call 格式适配
3. `internal/llm/openai.go`：SSE 流式 + tool_call 格式适配
4. `internal/llm/retry.go`：指数退避重试
5. `internal/headless/runner.go`：单轮调用，流式打印到 stdout
6. `trae run "prompt"` 子命令
7. 测试：httptest mock SSE 流

**验收**：
- `trae run "写一首诗" --provider anthropic` 流式输出
- `--provider openai` 同样可用
- 中途断网自动重试 3 次
- token usage 统计正确

---

## M2：工具闭环（read/write/edit/bash + agent loop）

**目标**：agent 能多步 think-act，调用工具完成简单编码任务。

**任务**：
1. `internal/tool/tool.go`：`Tool` 接口、`Result`、JSON schema 暴露
2. `internal/tool/read.go`：读文件，行号、offset/limit
3. `internal/tool/write.go`：写文件
4. `internal/tool/edit.go`：str_replace，含唯一性校验
5. `internal/tool/glob.go`：`**` glob 匹配
6. `internal/tool/grep.go`：正则搜索，行号、上下文
7. `internal/tool/bash.go`：exec，超时、流式输出
8. `internal/tool/dispatcher.go`：并行/串行调度
9. `internal/agent/loop.go`：think-act 循环，tool_call 收集与回灌
10. `internal/agent/state.go`：步数、task_done 检测
11. system prompt 设计（参考 Claude Code 风格）
12. 测试：每个工具有单测，agent loop 有集成测试（mock provider）

**验收**：
- `trae run "在当前目录创建 hello.py 并运行"` 能完成
- `trae run "找出所有 TODO 注释"` 能用 grep 工具
- 多步任务（读-改-测）能闭环
- 并行 tool_call 正确执行

---

## M3：交互式 REPL

**目标**：`trae interactive` 进入流式交互界面，支持斜杠命令和中断。

**任务**：
1. `internal/cli/repl.go`：主循环、行编辑、多行输入
2. `internal/cli/render.go`：流式 token 渲染、tool call 卡片、错误样式
3. `internal/cli/interrupt.go`：Ctrl+C 一次中断当前步、两次退出
4. `internal/cli/completion.go`：`/` 命令补全、文件路径补全
5. `internal/cli/command.go`：命令注册框架
6. 内置命令：`/help` `/clear` `/model` `/status` `/exit` `/cost`
7. `trae interactive` 子命令
8. 测试：命令分发逻辑单测

**验收**：
- 流式输出逐 token 显示，无闪烁
- `Ctrl+C` 中断当前 LLM 调用，回到提示符
- `/model openai gpt-4o` 切换 provider
- `/clear` 清空上下文
- `/help` 列出所有命令

---

## M4：上下文管理（compact + resume）

**目标**：长会话自动压缩，会话可持久化与恢复。

**任务**：
1. `internal/agent/context.go`：token 计数、阈值检测
2. compact 实现：调一次 LLM 生成摘要，替换历史
3. `/compact` 手动触发
4. `internal/session/store.go`：会话 JSON 持久化
5. `internal/session/resume.go`：会话列表、恢复
6. `/resume` 命令
7. 测试：compact 后 token 数下降、resume 后状态一致

**验收**：
- 100 轮对话后自动 compact，UI 提示
- `/resume` 列出最近会话
- 选择会话后能继续对话，保留上下文
- 重启进程后仍可 resume

---

## M5：子 agent（Task 工具）

**目标**：主 agent 可派发并行子 agent。

**任务**：
1. `internal/agent/subagent.go`：子 agent 创建、独立上下文
2. `internal/tool/task.go`：Task 工具，参数 `subagent_type`/`description`/`prompt`
3. 内置子 agent 类型：`search`（只读）、`general-purpose`
4. 并行执行：多个 Task 调用并发，channel 汇总
5. 子 agent 结果摘要回汇主 agent
6. 测试：mock provider 验证并行与隔离

**验收**：
- `trae run "调研这个项目的测试框架，同时找出所有 TODO"` 触发两个并行子 agent
- 子 agent 上下文不污染主 agent
- 子 agent 失败返回错误摘要，不崩溃主流程

---

## M6：权限与安全

**目标**：工具调用前过权限门，危险操作需确认。

**任务**：
1. `internal/permission/policy.go`：策略加载、规则匹配
2. 策略文件 `~/.trae/permissions.json`
3. dispatcher 集成权限门
4. `internal/cli` 交互式确认 UI
5. 内置危险命令黑名单
6. `/permissions` 命令查看与修改
7. 测试：各决策路径单测

**验收**：
- `rm -rf /` 被 deny
- `git push` 弹确认
- `ls` 直接执行
- `allow-always` 记忆到策略文件

---

## M7：MCP 集成

**目标**：可挂载 MCP server，工具自动发现。

**任务**：
1. `internal/mcp/client.go`：JSON-RPC over stdio/SSE
2. `internal/mcp/registry.go`：工具注册到 dispatcher
3. 配置 `mcp_servers` 段
4. `/mcp` 命令查看状态
5. 启动时自动连接、退出时清理
6. 测试：mock MCP server

**验收**：
- 配置 playwright MCP，能调用浏览器工具
- MCP 工具与内置工具统一调度
- `/mcp` 显示已连接 server 和工具列表

---

## M8：打磨与发布

**目标**：可发布 v1.0.0。

**任务**：
1. `internal/trajectory/recorder.go`：轨迹 JSON 记录 ✅
2. `/cost` 费用统计（按 provider 价格表）✅
3. `trae run --json` 非交互模式 ✅
4. `goreleaser` 配置：跨平台编译、changelog ✅
5. README 重写、安装文档 ✅
6. 性能优化：冷启动、内存（待补基线测试）
7. 端到端测试：SWE-bench 风格任务 ✅（e2e_test.go 3 个冒烟测试）

**验收**：
- 单二进制 < 30MB ✅（12MB）
- 冷启动到首 token < 500ms（待测）
- `trae run --json "task"` 输出结构化结果 ✅
- GitHub release 自动产出 macOS/Linux 二进制 ✅（goreleaser 配置就绪）

---

## 里程碑依赖与并行性

```
M0 ── M1 ── M2 ── M3 ── M4 ── M5
                  │
                  └── M6（可与 M4/M5 并行）
                  │
                  └── M7（依赖 M2 工具层）
                        │
                        M8（依赖全部）
```

- M6、M7 可与 M4/M5 部分并行
- M8 必须最后

## 后续迭代（v1.1+）

### v1.1 — 对标 Claude Code / Kimi Code 核心体验差距

以下功能按优先级排序，P0 为最高。

**P0 核心体验（已实现）**
- ✅ 项目记忆系统：`.trae/AGENTS.md` 加载 + `/init` 自动分析生成
- ✅ 自定义斜杠命令：从 `.trae/commands/*.md` 加载，支持 `$ARGUMENTS` 和 `$1/$2` 参数
- ✅ 扩展思考：`think`/`think hard`/`think harder`/`ultrathink` 分级思考预算
- ✅ 计划模式：`/plan` 切换只规划不执行

**P0 待实现**
- 模型运行时切换：`/model <name>` 在 REPL 中热切换模型（当前只读）

**P1 增强能力**
- IDE 集成协议（ACP）：与 Zed/VSCode 联动，获取打开文件、linter 警告等上下文
- Hooks 生命周期钩子：agent 各阶段自动化钩子，接入 CI/CD 流程
- 多模态输入：图片/截图转代码（对标 Kimi Code）
- 代码审查流程：`/review` 安全/性能/风格检测命令
- Anthropic extended thinking 原生支持：通过 API thinking 参数实现真正的扩展思考

**P2 生态扩展**
- 插件/技能系统：第三方插件生态，`/plugin` + `/skills` 管理
- 国内模型 provider：通义千问、DeepSeek、Kimi K2 接入
- 企业级云接入：AWS Bedrock / Google Cloud Vertex
- 超长上下文优化：RAG 分块检索策略，支持大型代码库分析
- 多会话并行：同时处理多个独立任务
- OpenTelemetry trace 导出
- 更多子 agent 类型（test-runner、reviewer）
- Web UI（可选）
- 配置即代码（`.trae/` 项目配置）
