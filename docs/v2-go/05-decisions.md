# 已确认决策

本文档记录设计阶段已拍板的关键决策。

## D1：Go module path

**决定**：`github.com/bytedance/trae-agent`

不加 `/v2` 后缀。Go 代码直接在仓库根，这是唯一的实现，无需版本化 module path。

---

## D2：代码目录位置

**决定**：仓库根

Go 代码直接放在仓库根目录（`cmd/`、`internal/`、`pkg/`），不设 `v2/` 子目录。Python 代码全部删除。

---

## D3：是否保留旧 Python 实现

**决定**：不保留

Python 代码全部删除，追求干净。Go 是唯一实现。

---

## D4：CLI 二进制名

**决定**：`trae`

简短，与 Claude Code（`claude`）、Cursor（`cursor`）风格一致。

---

## D5：配置文件格式与位置

**决定**：`~/.trae/config.yaml` + `.trae/config.yaml`（项目级）

YAML 格式。`~/.trae/` 集中存放 config/sessions/logs/permissions。

---

## D6：是否引入 bubbletea / lipgloss

**决定**：引入 lipgloss 做样式，不用 bubbletea

lipgloss 提供颜色/边框/对齐，省去手写 ANSI；bubbletea 的 Elm 架构与流式 agent loop 冲突，不引入。

---

## D7：LLM SDK 策略

**决定**：全部自研 HTTP 客户端

统一 `Provider` 接口，所有 provider 行为一致。直接用 `net/http` + SSE 解析，不依赖各厂商 SDK。

---

## D8：grep 工具实现

**决定**：纯 Go 自研（doublestar + bufio.Scanner）

坚持单二进制原则，不内嵌 ripgrep。

---

## D9：MVP 范围

**决定**：M0-M3（骨架 + LLM + 工具 + REPL）

先出可演示的交互版快速验证，M4/M5 紧随其后。

---

## D10：是否需要 Docker 沙箱

**决定**：不做

bash 工具直接在主机执行，靠权限门控。与 Claude Code / Cursor CLI 一致。

---

## D11：测试策略

**决定**：单测为主 + 少量集成测试（mock provider）

端到端测试作为可选 CI job，SWE-bench 评测在 M8 引入。

---

## D12：发布节奏

**决定**：M3 后发 beta，M8 后发 v1.0.0

M3 是首个完整可用版，适合 beta 收集反馈；M8 正式发布。版本号从 v1.0.0 开始（不再是 v2，因为 Python 代码已删除，这就是 trae-agent）。
