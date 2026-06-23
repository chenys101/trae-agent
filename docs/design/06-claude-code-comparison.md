# Claude Code vs trae-agent 功能对比

对比时间：2026-06-22，基于 Claude Code v2.1.x 与 trae-agent solo-go 分支（commit 81c5cc8）。

---

## 一、CLI 启动与运行模式

| 能力 | Claude Code | trae-agent | 状态 |
|------|-------------|------------|------|
| 交互式 REPL | `claude` | `trae interactive` | ✅ 对齐 |
| 带问题启动 | `claude "任务"` | `trae run "prompt"` | ✅ 对齐 |
| 一次性执行后退出 | `claude -p "查询"` | `trae run "prompt"`（非交互） | ✅ 对齐 |
| 继续上次会话 | `claude -c` / `claude -r` | `trae interactive` 内 `/resume` | ⚠️ 需先进入 REPL |
| 恢复指定会话 | `claude -r <session-id>` | `/resume <session-id>` | ⚠️ 仅 REPL 内 |
| `--json` 结构化输出 | `claude -p --output-format json` | `trae run --json` | ✅ 对齐 |
| `--trajectory` 轨迹记录 | 无原生（靠 `--debug`） | `trae run --trajectory` | ✅ trae 领先 |
| `--model` 切换 | `claude --model` | `trae run --model` | ✅ 对齐 |
| `--provider` 切换 | 仅 Anthropic | `trae --provider anthropic/openai` | ✅ trae 领先（多 provider） |
| `--add-dir` 多工作目录 | `claude --add-dir` | ❌ | ❌ 缺失 |
| `--allowedTools` 工具白名单 | `claude --allowedTools` | ❌ | ❌ 缺失 |
| `--dangerously-skip-permissions` | ✅ | ❌ | ❌ 缺失 |
| 图片输入（Ctrl+V 粘贴截图） | ✅ | ❌ | ❌ 缺失 |
| stdin 管道输入 | `cat file \| claude -p` | ❌ | ❌ 缺失 |

## 二、斜杠命令

| 命令 | Claude Code | trae-agent | 状态 |
|------|-------------|------------|------|
| `/help` | ✅ | ✅ | ✅ |
| `/clear` | ✅ | ✅ | ✅ |
| `/compact [instructions]` | ✅ 带聚焦指令 | ✅（无参数） | ⚠️ 缺聚焦指令 |
| `/cost` | ✅ | ✅ | ✅ |
| `/model` | ✅ 可切换 | ⚠️ 仅显示 | ❌ 不可运行时切换 |
| `/status` | ✅ | ✅ | ✅ |
| `/permissions` | ✅ 查看/修改 | ✅ 查看/清除 | ⚠️ 缺运行时添加规则 |
| `/mcp` | ✅ 管理 + OAuth | ✅ 仅查看 | ⚠️ 缺 OAuth/管理 |
| `/sessions` `/resume` | ✅（`/resume`） | ✅ | ✅ |
| `/init` 生成 CLAUDE.md | ✅ | ❌ | ❌ 缺失 |
| `/memory` 编辑记忆文件 | ✅ | ❌ | ❌ 缺失 |
| `/agents` 自定义子 agent | ✅ | ❌ | ❌ 缺失 |
| `/add-dir` 添加工作目录 | ✅ | ❌ | ❌ 缺失 |
| `/bug` 报告 bug | ✅ | ❌ | ❌ 缺失 |
| `/config` 设置界面 | ✅ | ❌（仅 `show-config`） | ⚠️ 缺交互式配置 |
| `/doctor` 健康检查 | ✅ | ❌ | ❌ 缺失 |
| `/login` `/logout` | ✅ | ❌ | ❌ 缺失（无账号体系） |
| `/pr_comments` PR 评论 | ✅ | ❌ | ❌ 缺失 |
| `/review` 代码审查 | ✅ | ❌ | ❌ 缺失 |
| `/rewind` 回滚对话/代码 | ✅ | ❌ | ❌ 缺失 |
| `/sandbox` 沙箱 bash | ✅ | ❌（D12 决策不做） | ❌ 主动放弃 |
| `/terminal-setup` Shift+Enter | ✅ | ❌ | ❌ 缺失 |
| `/usage` 用量限制 | ✅ | ❌ | ❌ 缺失 |
| `/vim` vim 模式 | ✅ | ❌ | ❌ 缺失 |
| 自定义斜杠命令 | ✅ `.claude/commands/*.md` | ❌ | ❌ 缺失（P2） |

## 三、内置工具

| 工具 | Claude Code | trae-agent | 状态 |
|------|-------------|------------|------|
| Read | ✅ 行号/offset/limit | ✅ 完全对齐 | ✅ |
| Write | ✅ | ✅ 原子写 | ✅ |
| Edit (str_replace) | ✅ | ✅ 唯一性校验 | ✅ |
| Glob | ✅ | ✅ `**` 支持 | ✅ |
| Grep | ✅（内嵌 ripgrep） | ✅ 纯 Go 自研 | ⚠️ 性能弱于 rg |
| Bash | ✅ | ✅ 超时/进程组/输出截断 | ✅ |
| TodoWrite | ✅ | ✅ | ✅ |
| Task（子 agent） | ✅ 多类型 | ✅ search/general-purpose | ✅ |
| WebFetch | ✅ | ❌ | ❌ 缺失 |
| WebSearch | ✅ | ❌ | ❌ 缺失 |
| NotebookEdit | ✅ Jupyter | ❌ | ❌ 缺失 |
| BashOutput（后台任务） | ✅ | ❌ | ❌ 缺失 |
| KillShell | ✅ | ❌ | ❌ 缺失 |
| MultiEdit（批量编辑） | ✅ | ❌ | ❌ 缺失 |
| TaskCreate/TaskUpdate/TaskList | ✅（新版任务系统） | ❌（用 TodoWrite 替代） | ⚠️ 语义不同 |
| MCP 工具发现 | ✅ | ✅ stdio | ✅ |

## 四、LLM Provider

| Provider | Claude Code | trae-agent | 状态 |
|----------|-------------|------------|------|
| Anthropic | ✅ 原生 | ✅ 自研 SSE | ✅ |
| OpenAI | ⚠️（兼容端点） | ✅ 自研 SSE | ✅ trae 领先 |
| Google Gemini | ❌ | ❌ | — |
| OpenRouter | ❌ | ⚠️（兼容 OpenAI 端点可用） | ⚠️ |
| Ollama | ❌ | ⚠️（兼容 OpenAI 端点可用） | ⚠️ |
| Bedrock/Vertex | ✅ | ❌ | ❌ 缺失 |
| 流式输出 | ✅ | ✅ | ✅ |
| 重试退避 | ✅ | ✅ 指数退避+jitter | ✅ |

## 五、上下文管理

| 能力 | Claude Code | trae-agent | 状态 |
|------|-------------|------------|------|
| 自动 compact | ✅ | ✅ 阈值 100k token | ✅ |
| 手动 compact | ✅ 带聚焦指令 | ✅（无聚焦） | ⚠️ |
| token 计数 | ✅ 精确（tiktoken） | ⚠️ 估算（rune/2） | ⚠️ 精度低 |
| 会话持久化 | ✅ | ✅ JSON `~/.trae/sessions/` | ✅ |
| 会话 resume | ✅ | ✅ | ✅ |
| CLAUDE.md 项目记忆 | ✅ 自动加载 | ❌ | ❌ 缺失 |
| 分层记忆（user/project） | ✅ | ❌ | ❌ 缺失 |
| 上下文窗口感知 | ✅ | ✅ | ✅ |

## 六、权限与安全

| 能力 | Claude Code | trae-agent | 状态 |
|------|-------------|------------|------|
| 工具调用审批 | ✅ | ✅ y/n/always | ✅ |
| 危险命令拦截 | ✅ | ✅ rm -rf/mkfs/dd/fork bomb | ✅ |
| 规则持久化 | ✅ `.claude/settings.json` | ✅ `~/.trae/permissions.json` | ✅ |
| 文件读写白名单 | ✅ | ❌（仅命令级） | ❌ 缺失 |
| 沙箱隔离 | ✅ `/sandbox` | ❌（D12 主动放弃） | ❌ 主动放弃 |
| `--allowedTools` | ✅ | ❌ | ❌ 缺失 |
| 环境变量脱敏 | ✅ | ✅ filterEnv KEY/SECRET/TOKEN | ✅ |

## 七、子 Agent

| 能力 | Claude Code | trae-agent | 状态 |
|------|-------------|------------|------|
| 并行子 agent | ✅ | ✅ goroutine | ✅ |
| 只读 search agent | ✅ | ✅ | ✅ |
| general-purpose agent | ✅ | ✅ | ✅ |
| 自定义子 agent 类型 | ✅ `/agents` 定义 | ❌ | ❌ 缺失 |
| 子 agent 上下文隔离 | ✅ | ✅ | ✅ |
| 子 agent 工具受限 | ✅ | ✅ RestrictedRegistry | ✅ |

## 八、MCP

| 能力 | Claude Code | trae-agent | 状态 |
|------|-------------|------------|------|
| stdio transport | ✅ | ✅ | ✅ |
| SSE/HTTP transport | ✅ | ❌ | ❌ 缺失 |
| OAuth 认证 | ✅ | ❌ | ❌ 缺失 |
| 工具自动发现 | ✅ | ✅ | ✅ |
| 多 server 并发 | ✅ | ✅ | ✅ |
| `/mcp` 管理 | ✅ 连接/OAuth/重启 | ⚠️ 仅查看 | ⚠️ |

## 九、交互体验

| 能力 | Claude Code | trae-agent | 状态 |
|------|-------------|------------|------|
| 流式 token 渲染 | ✅ | ✅ | ✅ |
| Ctrl+C 中断当前步 | ✅ | ✅ 单击中断/双击退出 | ✅ |
| Ctrl+D 退出 | ✅ | ✅ | ✅ |
| 工具调用卡片 | ✅ | ✅ lipgloss 紫色 | ✅ |
| `/` 命令补全 | ✅ | ✅ readline PrefixCompleter | ✅ |
| 文件路径补全 | ✅ | ❌ | ❌ 缺失 |
| 多行输入（Shift+Enter） | ✅ | ⚠️ 依赖 readline | ⚠️ |
| 图片粘贴 | ✅ | ❌ | ❌ 缺失 |
| vim 模式 | ✅ | ❌ | ❌ 缺失 |
| 主题切换 | ✅ | ❌ | ❌ 缺失 |
| 256 色/真彩色 | ✅ | ✅ lipgloss 自动探测 | ✅ |

## 十、工程化

| 能力 | Claude Code | trae-agent | 状态 |
|------|-------------|------------|------|
| 单二进制分发 | ❌ Node.js | ✅ 12MB 静态 | ✅ trae 领先 |
| 跨平台 | ✅ mac/linux/win | ⚠️ mac/linux（WSL） | ⚠️ 缺原生 win |
| goreleaser | — | ✅ | ✅ |
| 轨迹记录 | ⚠️ `--debug` | ✅ JSONL `--trajectory` | ✅ trae 领先 |
| 费用统计 | ✅ | ✅ 10 模型价格表 | ✅ |
| 日志 | ✅ | ✅ slog `~/.trae/logs/` | ✅ |
| 测试覆盖 | — | ✅ 40 个测试文件 | ✅ |
| 自动更新 | ✅ `claude update` | ❌ | ❌ 缺失 |
| 插件/扩展机制 | ✅ | ⚠️ 接口可扩展但无插件系统 | ⚠️ |

## 十一、差异化与领先项

### trae-agent 领先 Claude Code 的点

1. **单二进制 12MB** vs Node.js 运行时（核心目标 G1 达成）
2. **多 Provider 原生支持**（OpenAI 自研 SSE）vs Claude Code 仅 Anthropic
3. **`--trajectory` 原生轨迹记录** JSONL 格式，便于回放分析
4. **Go 并发模型** goroutine 天然适合并行子 agent
5. **无外部依赖**（ripgrep 等纯 Go 自研）

### trae-agent 落后 Claude Code 的关键缺口

| 优先级 | 缺口 | 影响 |
|--------|------|------|
| P0 | CLAUDE.md 项目记忆 / `/init` | 长任务上下文丢失，无法沉淀项目知识 |
| P0 | WebFetch / WebSearch 工具 | 无法查实时信息、文档 |
| P0 | `/model` 运行时切换 | 必须重启切换模型 |
| P1 | 自定义斜杠命令（`.claude/commands/*.md`） | 无法沉淀常用 prompt |
| P1 | `/agents` 自定义子 agent | 无法定义专用 agent 角色 |
| P1 | `/compact` 带聚焦指令 | 压缩无法保留特定主题 |
| P1 | 文件路径补全 | REPL 体验差 |
| P1 | 精确 token 计数（tiktoken） | compact 阈值不准 |
| P1 | MCP SSE/HTTP transport + OAuth | MCP 生态覆盖不全 |
| P2 | MultiEdit 批量编辑 | 多处改效率低 |
| P2 | `/rewind` 回滚 | 误操作不可逆 |
| P2 | `/review` 代码审查 | 缺专用工作流 |
| P2 | stdin 管道输入 | 无法 `cat \| trae` |
| P2 | 图片输入 | 多模态缺失 |
| P2 | `/doctor` 健康检查 | 排障困难 |

## 十二、总结

**核心闭环已对齐**：流式 REPL + 工具循环 + 子 agent + compact + resume + 权限 + MCP + 轨迹 + 费用 + JSON 模式，M0-M8 全部完成，P0 工具集（含 TodoWrite）齐全。

**最大差距集中在三块**：

1. **记忆系统**（CLAUDE.md / `/init` / `/memory`）— 影响长任务质量
2. **多模态与外部信息**（WebFetch/WebSearch/图片）— 影响能力边界
3. **可定制性**（自定义命令/自定义 agent/运行时切模型）— 影响工作流沉淀

建议下一阶段优先做 **CLAUDE.md 项目记忆 + WebFetch/WebSearch + `/model` 运行时切换**，这三项性价比最高。

---

*参考来源：*
- [Claude Code Slash Commands](https://claude.yourdocs.dev/docs/claude-code/slash-commands)
- [Claude Code 命令大全](https://blog.csdn.net/weimeilayer/article/details/160797461)
- [Claude Code 终极使用指南 2026](https://blog.csdn.net/weixin_45284808/article/details/161429319)
- [CLI Coding Agents Comparison](https://objects.githubusercontent.com/github-production-repository-file-5c1aeb/65494454/22561419)
- [Claude Code 2.1.1 Major Update](https://claudecode.app/blog/claude-code-2-1-1-major-update)
