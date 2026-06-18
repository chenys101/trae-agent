# 待确认决策点

本文档列出设计阶段尚未拍板、需要你确认的关键决策。每个决策都给出了推荐项与理由，确认后会回写到对应文档。

## D1：Go module path

**选项**：
- A. `github.com/bytedance/trae-agent/v2`（沿用现有仓库归属）
- B. `github.com/bytedance/trae-agent`（不加 v2，作为新目录共存）
- C. 其他组织路径

**推荐**：A。语义版本化标准做法，与现有 Python 实现共存于同仓库不同目录，import path 不冲突。

**影响**：所有 Go 文件的 package 声明与 import。

---

## D2：新代码目录位置

**选项**：
- A. 仓库根下 `v2/`（与 `trae_agent/` 平级）
- B. 仓库根下 `go/`
- C. 新仓库

**推荐**：A。`v2/` 语义清晰，与 `docs/v2-go/` 对应，构建产物 `v2/bin/trae`。

**影响**：Makefile、CI、goreleaser 配置路径。

---

## D3：是否保留旧 Python 实现

**选项**：
- A. 保留在 `trae_agent/`，长期并存
- B. 保留但标记 deprecated，引导迁移
- C. v2 稳定后删除

**推荐**：B。保留可对照参考，但 README 和文档主推 v2，避免新用户误用。

**影响**：README、CI 矩阵、维护成本。

---

## D4：CLI 二进制名

**选项**：
- A. `trae`（简短，但可能与已有命令冲突）
- B. `trae-cli`（沿用现有命名）
- C. `traev2`

**推荐**：A。Claude Code 用 `claude`，Cursor 用 `cursor`，单字更符合 CLI 习惯。冲突由用户 alias 解决。

**影响**：cobra 根命令、文档、安装脚本。

---

## D5：配置文件格式与位置

**选项**：
- A. `~/.trae/config.yaml` + `.trae/config.yaml`（项目级）
- B. `~/.config/trae/config.yaml`（XDG 规范）
- C. TOML

**推荐**：A。YAML 与现有 Python 实现一致，迁移成本低；`~/.trae/` 集中存放 config/sessions/logs/permissions 便于管理。

**影响**：config 加载逻辑、文档。

---

## D6：是否引入 bubbletea / lipgloss

**选项**：
- A. 不引入，自研流式渲染 + 少量 ANSI 转义
- B. 引入 lipgloss 做样式，不用 bubbletea
- C. 引入 bubbletea 全屏 TUI

**推荐**：B。lipgloss 提供颜色/边框/对齐，省去手写 ANSI；bubbletea 的 Elm 架构与流式 agent loop 冲突，不引入。

**影响**：渲染层实现复杂度、依赖数量。

---

## D7：LLM SDK 策略

**选项**：
- A. 全部自研 HTTP 客户端
- B. OpenAI 用 `sashabaranov/go-openai`，其余自研
- C. 尽量用社区 SDK

**推荐**：A。统一 `Provider` 接口要求所有 provider 行为一致，社区 SDK 的抽象差异会泄漏到上层；自研 HTTP + SSE 解析可控且代码量不大。

**影响**：开发量、维护成本、新 provider 接入速度。

---

## D8：grep 工具实现

**选项**：
- A. 纯 Go 自研（doublestar + bufio.Scanner）
- B. 内嵌 ripgrep 二进制
- C. 调用系统 ripgrep（如有）

**推荐**：A。坚持单二进制原则；性能在 10 万行级足够；ripgrep 内嵌违反分发原则。

**影响**：grep 工具性能、二进制大小。

---

## D9：MVP 范围

**选项**：
- A. M0-M3（骨架 + LLM + 工具 + REPL），最小可用交互版
- B. M0-M4（加 compact + resume），完整个人使用版
- C. M0-M5（加子 agent），对标 Claude Code 核心体验

**推荐**：A。先出可演示的交互版快速验证，M4/M5 紧随其后。避免 MVP 过大导致迟迟无法验证。

**影响**：首个里程碑的交付范围。

---

## D10：是否需要 Docker 沙箱

**选项**：
- A. v2 不做，bash 工具直接在主机执行（靠权限门控）
- B. 沿用 Python 版 Docker 沙箱思路
- C. 可选插件，后期补

**推荐**：A。Claude Code / Cursor CLI 默认都是主机执行 + 权限确认，Docker 沙箱增加复杂度且体验割裂；权限门已足够安全。

**影响**：架构简化、安全性边界。

---

## D11：测试策略

**选项**：
- A. 单测为主 + 少量集成测试（mock provider）
- B. 加端到端测试（真实 LLM 调用，CI 跑）
- C. 加 SWE-bench 风格评测

**推荐**：A 起步，B 作为可选 CI job（需 API key secret），C 在 M8 评估阶段引入。

**影响**：CI 配置、开发节奏。

---

## D12：发布节奏

**选项**：
- A. 每个 milestone 打 alpha tag
- B. M3 后发 beta，M8 后发 v2.0.0
- C. 滚动发布，无明确版本

**推荐**：B。M3 是首个完整可用版，适合 beta 收集反馈；M8 正式发布。

**影响**：goreleaser 配置、changelog 维护。

---

## 确认方式

请逐项回复你的选择，或对推荐项提出异议。全部确认后我会：
1. 把决策回写到 `00-vision.md` / `02-architecture.md` / `03-tech-stack.md`
2. 据此产出 M0 的 bite-sized 实施计划（writing-plans 格式）
