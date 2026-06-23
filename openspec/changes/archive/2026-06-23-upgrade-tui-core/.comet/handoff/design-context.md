# Comet Design Handoff

- Change: upgrade-tui-core
- Phase: design
- Mode: compact
- Context hash: 6dc8c6b20f38fb0a183671623731511922d9bc755b4e0381f52b2d9989ede379

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/upgrade-tui-core/proposal.md

- Source: openspec/changes/upgrade-tui-core/proposal.md
- Lines: 1-32
- SHA256: 7a0216d4ed2b852da73184404d13f743a51a559bad1db10fd692d394cba1fe61

```md
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
```

## openspec/changes/upgrade-tui-core/design.md

- Source: openspec/changes/upgrade-tui-core/design.md
- Lines: 1-158
- SHA256: 68b7f6c0321c75e4d4a547fa3680af92550dc5b53a1aefa22e61b32a4011d0ae

[TRUNCATED]

```md
## Context

当前 TUI（`internal/tui/`，~400 行）是 Bubble Tea v1 上的最小原型：

- **输入**：原始 `string` 拼接，无光标移动、无多行、无粘贴
- **视口**：手动 `[]string` 切片，仅 j/k/↑↓ 单行滚动
- **输出**：纯文本 `role: content` 格式，无 Markdown 渲染
- **事件**：`StreamEmitter` 仅有 `OnChunk`/`OnDone`/`OnError` 三种文本事件，工具调用完全不可见
- **审批**：`permission.AskerFunc` 硬编码返回 `false`，所有需权限操作被拒绝
- **进度**：无 spinner/状态指示
- **CJK**：`utf8.RuneLen`（字节长度）用于换行计算，中文字符被当作 1-3 宽度而非 2 显示宽度

参考目标 DeepSeek-Reasonix 的 TUI 是 Bubble Tea v2 上 ~3800 行的成熟产品，具备完整的 transcript、工具卡片、Markdown 渲染、审批交互、多主题等能力。

## Goals / Non-Goals

**Goals:**

- 升级到 Bubble Tea v2 + bubbles/v2，获得 `textarea`、`viewport`、`spinner` 等组件
- 实现 Markdown ANSI 渲染（标题、列表、代码块、表格、粗体/斜体），代码块带语法高亮
- 在聊天流中展示工具调用（工具名 + 参数摘要 + 输出折叠）
- 提供交互式审批横幅（y/n/a），替代全自动拒绝
- 显示 spinner + 耗时 + 状态栏
- 修复 CJK 字符显示宽度
- 保持跨平台兼容（Windows/macOS/Linux）

**Non-Goals:**

- 不做 chat 命令功能迁移（斜杠命令、会话恢复等）
- 不做全功能主题系统
- 不做全屏管理面板（MCP manager、skill picker）
- 不做文本选择/复制
- 不做自动补全菜单
- 不做 Reasoning/思考过程展示
- 不做 Termux/移动终端特殊适配

## Decisions

### D1: 升级到 Bubble Tea v2

**选择**: 从 `bubbletea v1.3.4` 升级到 `bubbletea/v2`

**理由**: 用户明确选择 v2。v2 的关键收益：
- `tea.View` 结构体替代 `string` 返回值，支持 `AltScreen`/`MouseMode` 声明式配置
- 更好的 Windows Terminal / ConPTY 支持
- `bubbles/v2` 的 `textarea`/`viewport` 组件是成熟的现成方案

**影响**: 所有 `internal/tui/` 文件和 `cmd/cli/tui.go` 需要适配 v2 API。`Model.View()` 返回值从 `string` 改为 `tea.View`。`tea.KeyMsg` 结构变化。`tea.NewProgram` 选项 API 变化。

**备选方案**: 保持 v1 + 用 bubbles v0.x — 被否决，因为 v0 bubbles 的 textarea/viewport 缺少 v2 的改进。

### D2: 引入 bubbles/v2 组件替代手工实现

**选择**: 使用 `bubbles/v2` 的 `textarea`、`viewport`、`spinner` 组件

**理由**: 
- `textarea` 提供多行输入、光标移动、IME 支持、粘贴处理、动态高度 — 这些自己实现至少 500+ 行
- `viewport` 提供滚动条、PgUp/PgDn/Home/End、鼠标滚轮 — 当前手动实现仅 50 行且功能极简
- `spinner` 提供动画进度指示 — 几行配置即可

**影响**: `Model` 结构从 `input string` + `scrollOffset int` 变为嵌入 `textarea.Model` + `viewport.Model` + `spinner.Model`。输入和滚动逻辑全部委托给组件。

### D3: Markdown 渲染方案

**选择**: goldmark 解析 + 自定义 ANSI terminal renderer

**理由**: 参考 Reasonix 的 `md.go` 方案。goldmark 是 Go 生态最成熟的 Markdown 解析器，支持 GFM 扩展。需要自定义 renderer 将 AST 转为 lipgloss-styled ANSI 字符串，而非 HTML。

**scope**: 首版支持：标题（带颜色/粗体）、段落、列表（有序/无序）、代码块（带 chroma 语法高亮）、内联代码、粗体/斜体、引用块、GFM 表格。不支持：数学公式、图片、脚注。

**备选方案**: 
- `glamour`（Charm 出品的 Markdown terminal renderer）— 开箱即用但定制性差，且对流式逐段渲染支持不足
- 纯正则替换 — 太脆弱，无法正确处理嵌套结构

### D4: 流式 Markdown 渲染策略

**选择**: 段落边界刷新（paragraph-boundary flush）

**理由**: 参考 Reasonix 的 `flushableMarkdownPrefix` 策略。LLM 流式输出 token 粒度太细，直接渲染会导致代码块围栏 ```` ``` ```` 被半渲染。解决方案：缓冲流式文本，仅在完整段落边界（连续空行 + 不在 fenced code block 内）时刷新渲染。

```

Full source: openspec/changes/upgrade-tui-core/design.md

## openspec/changes/upgrade-tui-core/tasks.md

- Source: openspec/changes/upgrade-tui-core/tasks.md
- Lines: 1-73
- SHA256: aaa3a462076ae42e8bf2c8a3a3372cbdc7d9e83339c5684427ad208692fdc2fc

```md
## 1. Bubble Tea v2 迁移 + 基础组件引入

- [ ] 1.1 升级 go.mod 依赖：bubbletea v1→v2、引入 bubbles/v2、lipgloss/v2、go-runewidth
- [ ] 1.2 迁移 `internal/tui/model.go` 到 v2 API：Model.View() 返回 tea.View、tea.KeyMsg 适配、tea.WindowSizeMsg 适配
- [ ] 1.3 引入 bubbles/v2 textarea 替代 string 输入：配置 Shift+Enter 换行、Enter 提交、动态高度（1-5 行）、CharLimit、IME 支持
- [ ] 1.4 引入 bubbles/v2 viewport 替代手动滚动：鼠标滚轮、PgUp/PgDn/Home/End、滚动条、tail-follow（流式时自动跟底）
- [ ] 1.5 引入 bubbles/v2 spinner：配置 spinner 样式、在 busy 状态显示动画 + 耗时计数
- [ ] 1.6 迁移 `cmd/cli/tui.go`：适配 v2 的 tea.NewProgram 选项（WithAltScreen → tea.View.AltScreen）
- [ ] 1.7 修复所有现有测试适配 v2 API（model_test.go、keymap_test.go、runner_test.go）

## 2. CJK 显示宽度修复

- [ ] 2.1 引入 `go-runewidth` 依赖，重写 `wrapText` 函数使用 `runewidth.RuneWidth()` 计算显示宽度
- [ ] 2.2 更新 `renderMessageLines` 中 prefix 宽度计算为显示宽度
- [ ] 2.3 添加 CJK 换行的单元测试（中文、日文、emoji 混合场景）

## 3. 事件系统扩展（工具调用 + 审批）

- [ ] 3.1 扩展 `StreamEmitter` 接口：增加 OnToolStart(name, args)、OnToolEnd(name, result, err)、OnApprovalRequest(name, args, respond)
- [ ] 3.2 定义新的 tea.Msg 类型：ToolStartMsg、ToolEndMsg、ApprovalRequestMsg、ApprovalResponseMsg
- [ ] 3.3 修改 `internal/agent/loop.go` 的 loopStepWithText：在 invokeTool 前后调用 emitter 的 OnToolStart/OnToolEnd
- [ ] 3.4 修改 `internal/agent/loop.go` 的 invokeTool：在 permission.Check 判定需要确认时调用 OnApprovalRequest 并等待 respond 回调
- [ ] 3.5 更新 `cmd/cli/tui_runner.go`：适配扩展后的 StreamEmitter 接口，传递所有事件到 channel
- [ ] 3.6 更新 chanEmitter：增加 OnToolStart、OnToolEnd、OnApprovalRequest 的 channel 实现

## 4. 工具调用可视化（TUI 侧）

- [ ] 4.1 在 Model 中增加工具调用状态跟踪（activeTools、toolResults）
- [ ] 4.2 在 Update 中处理 ToolStartMsg 和 ToolEndMsg：更新 transcript、切换 spinner 文案
- [ ] 4.3 实现 renderToolCard：工具卡片渲染（`● ToolName("args summary")`），包含颜色标记和参数截断
- [ ] 4.4 实现工具输出折叠展示：结果超过 8 行时折叠，显示行数摘要

## 5. 审批交互（TUI 侧）

- [ ] 5.1 在 Model 中增加 pendingApproval 模态状态（工具名、参数、respond 回调）
- [ ] 5.2 在 Update 中处理 ApprovalRequestMsg：进入审批模态，拦截按键路由
- [ ] 5.3 实现 renderApprovalBanner：显示工具名、参数摘要、`[y]es [n]o` 选项
- [ ] 5.4 处理审批按键（y/n）：调用 respond 回调（sync.Once 保护），退出审批模态
- [ ] 5.5 修改 `cmd/cli/chat_setup.go`：TUI 模式不再使用全自动拒绝 AskerFunc，改为由 TUI 审批

## 6. Markdown ANSI 渲染

- [ ] 6.1 引入 goldmark + chroma/v2 依赖
- [ ] 6.2 实现 mdRenderer 结构体：goldmark AST walker → lipgloss-styled ANSI 字符串
- [ ] 6.3 实现基础元素渲染：标题（粗体+颜色）、段落、粗体/斜体、内联代码（背景色）
- [ ] 6.4 实现列表渲染：有序列表（数字前缀）、无序列表（bullet 前缀）、嵌套缩进
- [ ] 6.5 实现代码块渲染：fenced code block + chroma 语法高亮 + 缩进 gutter
- [ ] 6.6 实现引用块渲染：`│` 前缀 + dim 样式
- [ ] 6.7 实现 GFM 表格渲染：列对齐、表头加粗、边框字符
- [ ] 6.8 添加 Markdown 渲染单元测试（各元素类型 + 嵌套组合）

## 7. 流式 Markdown 渲染优化

- [ ] 7.1 实现 flushableMarkdownPrefix：检测段落边界（空行 + 不在 fenced block 内），返回可安全渲染的前缀
- [ ] 7.2 重构流式更新逻辑：StreamChunkMsg 追加到 pending buffer → flushablePrefix → mdRenderer.Render → 写入 transcript
- [ ] 7.3 在 StreamDoneMsg 时刷新剩余 pending 内容
- [ ] 7.4 添加流式渲染的单元测试（半代码块、跨段落、空输出等边界场景）

## 8. 布局 + 状态栏 + View 整合

- [ ] 8.1 重构 View 函数：三区布局（viewport + 状态/审批 + textarea + help）
- [ ] 8.2 实现 bottomHeight 动态计算：textarea 高度 + 状态栏 + help + 可选审批横幅
- [ ] 8.3 实现状态栏渲染：模型名称 + busy/idle 状态 + 可选耗时
- [ ] 8.4 处理 WindowSizeMsg：调整 viewport/textarea 尺寸
- [ ] 8.5 更新帮助文本：反映新的快捷键映射

## 9. 集成测试 + 回归验证

- [ ] 9.1 更新所有现有测试适配新的 Model 结构（textarea/viewport/spinner 嵌入）
- [ ] 9.2 添加工具调用事件流测试：ToolStartMsg → spinner 文案变化 → ToolEndMsg → 工具卡片渲染
- [ ] 9.3 添加审批流程测试：ApprovalRequestMsg → 审批模态 → y/n 响应 → 恢复正常状态
- [ ] 9.4 添加 CJK + Markdown 渲染组合测试
- [ ] 9.5 确认 `go build ./cmd` 编译通过、`go test ./...` 全部通过
```

## openspec/changes/upgrade-tui-core/specs/tui-chat-interface/spec.md

- Source: openspec/changes/upgrade-tui-core/specs/tui-chat-interface/spec.md
- Lines: 1-107
- SHA256: 179632135ff68f950e11c2d5bf4555a955b7fc461436ea3fbac57cd408f9be82

[TRUNCATED]

```md
## MODIFIED Requirements

### Requirement: 支持消息流与输入交互
系统 MUST 在 TUI 中提供基于 bubbles/v2 textarea 组件的多行输入区域，支持光标移动、多行编辑（Shift+Enter/Alt+Enter 换行）、粘贴和 IME 输入。消息展示区 MUST 使用 bubbles/v2 viewport 组件，支持鼠标滚轮、PgUp/PgDn/Home/End 翻页和滚动条指示。

#### Scenario: 用户提交一条消息
- **WHEN** 用户在 textarea 输入区输入文本并按 Enter 触发发送
- **THEN** 用户消息出现在消息流中，系统开始处理并展示助手回复

#### Scenario: 用户进行多行输入
- **WHEN** 用户在 textarea 中按 Shift+Enter 或 Alt+Enter
- **THEN** 输入区插入换行而非提交消息，textarea 高度动态扩展（最大 5 行）

#### Scenario: 用户使用鼠标滚轮滚动
- **WHEN** 用户在消息区使用鼠标滚轮
- **THEN** viewport 按滚动步长上下翻滚，滚动条位置同步更新

#### Scenario: 用户使用键盘翻页
- **WHEN** 用户按 PgUp/PgDn/Home/End
- **THEN** viewport 按页翻滚或跳到顶部/底部

### Requirement: 支持基础会话控制快捷键
系统 MUST 提供基础快捷键能力以保证可用性，包括发送、退出会话、中断当前轮与基础导航。默认语义为：`Enter` 发送、`Ctrl+C` 退出会话、`Esc` 中断当前轮、`Shift+Enter` 换行、PgUp/PgDn/Home/End 翻页、鼠标滚轮滚动。

#### Scenario: 用户触发退出快捷键
- **WHEN** 用户在 TUI 中按下 Ctrl+C
- **THEN** 系统安全结束 TUI 会话并返回终端

#### Scenario: 用户中断当前轮但继续会话
- **WHEN** 模型正在流式输出且用户按下 `Esc`
- **THEN** 系统中断当前轮处理，保留当前会话历史并允许用户继续输入下一条消息

## ADDED Requirements

### Requirement: 支持 Markdown ANSI 渲染
系统 MUST 将助手回复中的 Markdown 文本渲染为带 ANSI 样式的终端输出。MUST 支持的元素：标题（带颜色/粗体）、段落、有序/无序列表、代码块（带 chroma 语法高亮）、内联代码（背景色区分）、粗体/斜体、引用块、GFM 表格。

#### Scenario: 助手回复包含代码块
- **WHEN** 助手回复包含 fenced code block（``````` ```language ... ``` ```````）
- **THEN** 代码块以缩进 + 语法高亮样式渲染，语言标识显示在代码块上方或旁边

#### Scenario: 助手回复包含表格
- **WHEN** 助手回复包含 GFM 风格的 Markdown 表格
- **THEN** 表格以对齐的列格式渲染，表头与数据行有视觉区分

#### Scenario: 流式输出中的 Markdown 渲染
- **WHEN** 助手回复正在流式输出
- **THEN** 系统按段落边界刷新渲染，未完成的代码块围栏不会被半渲染

### Requirement: 工具调用可视化
系统 MUST 在聊天流中展示工具调用的名称和参数摘要，以及工具执行结果的折叠展示。用户 MUST 能看到 agent 正在执行什么工具操作。

#### Scenario: agent 调用工具
- **WHEN** agent 发起一次工具调用
- **THEN** 消息流中显示工具卡片，包含工具名称和参数摘要（如 `● Read("src/main.go")`）

#### Scenario: 工具执行完成
- **WHEN** 工具调用返回结果
- **THEN** 工具卡片下方显示结果摘要（超过阈值行数时折叠，显示行数提示）

#### Scenario: 工具执行报错
- **WHEN** 工具调用返回错误
- **THEN** 工具卡片显示红色错误标记和错误消息

### Requirement: 交互式审批
系统 MUST 对需要权限确认的工具调用提供交互式审批横幅，允许用户在 TUI 内批准或拒绝操作，替代当前的全自动拒绝行为。

#### Scenario: 需要权限的工具调用
- **WHEN** agent 发起需要用户确认的工具调用（由 permission.Checker 判定）
- **THEN** TUI 显示审批横幅，包含工具名、参数摘要和操作选项（`[y]es` / `[n]o`）

#### Scenario: 用户批准操作
- **WHEN** 审批横幅显示中用户按 y
- **THEN** 工具调用被执行，审批横幅消失，消息流继续

#### Scenario: 用户拒绝操作
- **WHEN** 审批横幅显示中用户按 n
- **THEN** 工具调用被拒绝并返回 "Permission denied" 结果，agent 收到拒绝结果并继续

### Requirement: 进度指示
```

Full source: openspec/changes/upgrade-tui-core/specs/tui-chat-interface/spec.md

