## Why

当前 TUI 是 ~400 行的 v0 原型：纯文本输出、单行字符串输入、无工具调用可视化、无审批交互、无进度指示。日常使用中每个交互维度都有明显痛点——无法多行输入、看不到工具在做什么、代码输出无格式、滚动靠 j/k 一行一行翻。参考 DeepSeek-Reasonix 的成熟 TUI 架构（Bubble Tea v2 + bubbles 组件 + goldmark Markdown + chroma 高亮），本次将 TUI 从"能跑"升级到"可日常使用"。

## What Changes

- **升级 Bubble Tea v1→v2**：获得 `tea.View` 结构体、改进的鼠标/终端支持
- **引入 bubbles/v2 组件**：`textarea`（多行输入、光标移动、IME、粘贴）、`viewport`（滚动条、PgUp/PgDn/Home/End、鼠标滚轮）、`spinner`（进度指示）
- **Markdown 渲染**：引入 goldmark + 自定义 ANSI renderer，支持标题/列表/粗体/代码块；代码块使用 chroma 语法高亮
- **工具调用可视化**：在聊天流中展示工具名称、参数和输出摘要（类似 Reasonix 的工具卡片）
- **审批交互横幅**：需要权限的工具调用弹出交互式审批 UI（y/n），替代当前的全自动拒绝
- **CJK 显示修复**：使用 `go-runewidth` 替代 `utf8.RuneLen` 计算字符显示宽度
- **状态栏**：底部显示模型名称、busy 状态、耗时等基础信息
- **流式优化**：参考 Reasonix 的段落边界刷新策略，避免 Markdown 半渲染问题

## Capabilities

### New Capabilities

_无新增独立 capability_

### Modified Capabilities

- `tui-chat-interface`: 从 v0 纯文本原型升级为具备 Markdown 渲染、工具可视化、审批交互、现代输入/滚动组件的完整 TUI 体验

## Impact

- **`internal/tui/`**：全部文件重构 — model/view/stream/runner/message 均需适配 v2 API 和新组件
- **`cmd/cli/tui.go` + `tui_runner.go`**：适配新的 Runner/StreamEmitter 接口，支持工具事件传递
- **`go.mod`**：新增依赖 `bubbletea/v2`、`bubbles/v2`、`lipgloss/v2`、`goldmark`、`chroma/v2`、`go-runewidth`
- **`internal/agent/`**：可能需要扩展 `RunStreaming` 回调以暴露工具调用/结果事件
- **测试**：现有 `internal/tui/*_test.go` 需全部适配 v2 API
