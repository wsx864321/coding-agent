---
change: tui-render-engine
design-doc: docs/superpowers/specs/2026-06-24-tui-render-engine-design.md
base-ref: de02d8ca9c06666723942ab42aa66e1c8f7b2798
archived-with: 2026-06-25-tui-render-engine
---

# TUI 渲染引擎升级 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 TUI 聊天界面增加推理文本渲染、工具流式输出、Shell 输出折叠、Markdown 语法高亮与 diff 着色、文本选择与复制五大能力。

**Architecture:** 在现有 Bubble Tea Model 上扩展事件系统（新增 ReasoningText / ToolProgress 两种事件类型），在 model.go 中新增状态字段与事件处理分支，在 transcript.go / toolcard.go 中新增渲染逻辑，新增 selection.go 处理文本选择，升级 markdown.go 启用 chroma 语法高亮与 diff 着色。

**Tech Stack:** Go 1.26, Bubble Tea v2, Lipgloss v2, Glamour v2, Chroma v2, atotto/clipboard

## 全局约束

- Go 版本 >= 1.26
- 现有事件类型不访问新字段，零值无副作用
- TuiSink.Emit() 无需修改（channel 类型不变）
- 现有测试无需更新（新功能通过新增测试覆盖）
- 回归标准：go test ./internal/tui/... 全量通过
- 依赖 github.com/atotto/clipboard 已在 go.mod indirect，需提升为 direct
- 依赖 github.com/alecthomas/chroma/v2 已在 go.mod indirect，glamour 已依赖

archived-with: 2026-06-25-tui-render-engine
---

### Task 1: 事件系统扩展 — 新增 ReasoningText 与 ToolProgress 事件类型

**Files:**
- Modify: internal/event/event.go

**Interfaces:**
- Consumes: 无（基础设施层，最先实现）
- Produces: event.Kind 新增常量 ReasoningText/ToolProgress，event.Event 新增字段 ReasoningChunk/ToolCallID/ToolChunk

- [x] **Step 1: 在 event.go 中新增 Kind 常量**

在 const 块末尾（Notice 之后）添加 ReasoningText 和 ToolProgress 两个新常量。

- [x] **Step 2: 在 Event 结构体中新增字段**

在 Event 结构体的 ApprovalRespond 之后、Err 之前插入 ReasoningChunk string、ToolCallID string、ToolChunk string 三个新字段。

- [x] **Step 3: 运行现有测试确认无回归**

```powershell
cd D:\project\coding-agent; go test ./internal/event/... -v
```

预期：所有现有测试 PASS。

- [x] **Step 4: Commit**

```bash
git add internal/event/event.go
git commit -m "feat(event): add ReasoningText and ToolProgress event types"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 2: 事件系统扩展 — 确保 TuiSink 正确转发新事件类型

**Files:**
- Modify: internal/tui/sink_test.go（追加测试）

**Interfaces:**
- Consumes: event.ReasoningText、event.ToolProgress（来自 Task 1）
- Produces: 无新接口，确认 TuiSink.Emit() 无需修改

- [x] **Step 1: 审查 sink.go 确认无需修改**

阅读 internal/tui/sink.go 的 Emit 方法，确认 channel 类型不变，新事件类型通过 select 自动转发。无需代码变更。

- [x] **Step 2: 编写 sink 转发测试**

在 internal/tui/sink_test.go 末尾追加 TestSinkForwardsReasoningText 和 TestSinkForwardsToolProgress 两个测试函数。

- [x] **Step 3: 运行测试确认通过**

```powershell
cd D:\project\coding-agent; go test ./internal/tui/... -run "TestSinkForwards" -v
```

- [x] **Step 4: Commit**

```bash
git add internal/tui/sink_test.go
git commit -m "test(tui): verify sink forwards ReasoningText and ToolProgress events"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 3: 推理文本渲染 — 新增 EntryReasoning 类型与 Model 状态字段

**Files:**
- Modify: internal/tui/message.go
- Modify: internal/tui/model.go

**Interfaces:**
- Consumes: 无（纯数据结构）
- Produces: EntryReasoning EntryKind = 5，Model 新增 reasoning/reasoningLineIdx/showReasoning/thinkStart 字段

- [x] **Step 1: 在 message.go 中新增 EntryReasoning 常量**

在 const 块末尾添加 EntryReasoning。

- [x] **Step 2: 在 Model 结构体中新增推理状态字段**

在 turnCancel 之后插入 reasoning *strings.Builder、reasoningLineIdx int、showReasoning bool、thinkStart time.Time。

- [x] **Step 3: 在 New() 中初始化推理字段**

- [x] **Step 4: 运行现有测试确认无回归**

```powershell
cd D:\project\coding-agent; go test ./internal/tui/... -v
```

- [x] **Step 5: Commit**

```bash
git add internal/tui/message.go internal/tui/model.go
git commit -m "feat(tui): add EntryReasoning type and reasoning state fields"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 4: 推理文本渲染 — 处理 ReasoningText 事件与渲染

**Files:**
- Modify: internal/tui/model.go
- Modify: internal/tui/transcript.go

**Interfaces:**
- Consumes: event.ReasoningText、Model.reasoning 等字段
- Produces: ingestReasoningChunk、renderReasoningSummary、renderReasoningEntry 方法

- [x] **Step 1: 在 model.go 的 Update 中新增 ReasoningText 事件处理分支**

- [x] **Step 2: 实现 ingestReasoningChunk 方法**

首次推理文本时创建 EntryReasoning 摘要行，后续更新摘要行内容。

- [x] **Step 3: 实现 renderReasoningSummary 方法**

- [x] **Step 4: 在 transcript.go 顶部新增 reasoningDimStyle**

- [x] **Step 5: 在 renderEntry 中处理 EntryReasoning**

- [x] **Step 6: 实现 renderReasoningEntry 方法**

展开时显示摘要行 + 分隔线 + 推理文本，折叠时仅显示摘要行。

- [x] **Step 7: 运行编译检查**

```powershell
cd D:\project\coding-agent; go build ./internal/tui/...
```

- [x] **Step 8: Commit**

```bash
git add internal/tui/model.go internal/tui/transcript.go
git commit -m "feat(tui): handle ReasoningText events with summary line and rendering"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 5: 推理文本渲染 — 推理完成处理与 Ctrl+O 切换

**Files:**
- Modify: internal/tui/model.go

**Interfaces:**
- Consumes: event.Text、tea.KeyPressMsg（Ctrl+O）
- Produces: 推理完成时摘要行更新，Ctrl+O 切换 showReasoning

- [x] **Step 1: 在 Text 事件处理中检测推理结束**

若 reasoningLineIdx >= 0，将摘要行更新为 renderReasoningSummary(true)，设置 reasoningLineIdx = -1。

- [x] **Step 2: 在 KeyPressMsg 处理中新增 Ctrl+O 快捷键**

- [x] **Step 3: 在 interruptTurn 中重置推理状态**

- [x] **Step 4: 在 submit 中重置推理状态**

- [x] **Step 5: 运行编译检查**

```powershell
cd D:\project\coding-agent; go build ./internal/tui/...
```

- [x] **Step 6: Commit**

```bash
git add internal/tui/model.go
git commit -m "feat(tui): handle reasoning completion on Text event and Ctrl+O toggle"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 6: 推理文本渲染 — 单元测试

**Files:**
- Create: internal/tui/reasoning_test.go

**Interfaces:**
- Consumes: event.ReasoningText、event.Text、Model 推理字段
- Produces: 测试覆盖 ReasoningText 事件处理、折叠/展开切换

- [x] **Step 1: 编写推理文本渲染测试**

创建 internal/tui/reasoning_test.go，包含 TestReasoningTextCreatesSummaryLine、TestReasoningCompletionOnTextEvent、TestReasoningCtrlOToggle、TestReasoningResetOnSubmit。

- [x] **Step 2: 运行测试确认通过**

```powershell
cd D:\project\coding-agent; go test ./internal/tui/... -run "TestReasoning" -v
```

- [x] **Step 3: Commit**

```bash
git add internal/tui/reasoning_test.go
git commit -m "test(tui): add reasoning text rendering tests"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 7: 工具流式输出 — 新增 Model 状态字段

**Files:**
- Modify: internal/tui/model.go

**Interfaces:**
- Consumes: 无（纯数据结构）
- Produces: Model 新增 toolStreamIdx/toolStreamID/toolTail/toolPartial/toolLineCount/toolStreamStart 字段

- [x] **Step 1: 在 Model 结构体中新增流式输出状态字段**

- [x] **Step 2: 在 New() 中初始化 toolStreamIdx = -1**

- [x] **Step 3: 运行编译检查**

```powershell
cd D:\project\coding-agent; go build ./internal/tui/...
```

- [x] **Step 4: Commit**

```bash
git add internal/tui/model.go
git commit -m "feat(tui): add tool streaming output state fields"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 8: 工具流式输出 — 处理 ToolProgress 事件与尾部截断

**Files:**
- Modify: internal/tui/model.go

**Interfaces:**
- Consumes: event.ToolProgress、Model 流式字段
- Produces: ingestToolProgress、renderToolStreamBlock 方法，尾部截断算法

- [x] **Step 1: 在 Update 中新增 ToolProgress 事件处理分支**

- [x] **Step 2: 实现 ingestToolProgress 方法**

首次 ToolProgress 时创建流式输出块，后续追加 chunk，按换行符分割完整行 push 到 toolTail（最多 20 行）。

- [x] **Step 3: 实现 renderToolStreamBlock 方法**

渲染 working Ns 头部 + 尾部行 + N lines 尾部摘要。

- [x] **Step 4: 运行编译检查**

```powershell
cd D:\project\coding-agent; go build ./internal/tui/...
```

- [x] **Step 5: Commit**

```bash
git add internal/tui/model.go
git commit -m "feat(tui): handle ToolProgress events with tail truncation"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 9: 工具流式输出 — ToolResult 折叠与 Drain Loop

**Files:**
- Modify: internal/tui/model.go

**Interfaces:**
- Consumes: event.ToolResult、Model 流式字段
- Produces: ToolResult 时将流式输出转为折叠摘要，Drain Loop 批量处理高频事件

- [x] **Step 1: 在 ToolResult 处理中检测流式输出结束**

若 toolStreamIdx >= 0，将流式输出块转为折叠摘要，重置流式状态字段。

- [x] **Step 2: 实现 Drain Loop**

循环从 channel 读取最多 maxEventDrain（512）个事件，最后一次性 syncViewportContent。

- [x] **Step 3: 定义 maxEventDrain = 512 常量**

- [x] **Step 4: 实现 ingestDrainEvent 辅助方法**

- [x] **Step 5: 运行编译检查**

```powershell
cd D:\project\coding-agent; go build ./internal/tui/...
```

- [x] **Step 6: Commit**

```bash
git add internal/tui/model.go
git commit -m "feat(tui): add ToolResult stream collapse and drain loop"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 10: 工具流式输出 — 单元测试

**Files:**
- Create: internal/tui/stream_test.go

**Interfaces:**
- Consumes: event.ToolProgress、event.ToolResult、Model 流式字段
- Produces: 测试覆盖 ToolProgress 事件处理、尾部截断、行数计数

- [x] **Step 1: 编写工具流式输出测试**

创建 internal/tui/stream_test.go，包含 TestToolProgressCreatesStreamBlock、TestToolProgressTailTruncation、TestToolProgressLineCount、TestToolResultCollapsesStream。

- [x] **Step 2: 运行测试确认通过**

```powershell
cd D:\project\coding-agent; go test ./internal/tui/... -run "TestTool" -v
```

- [x] **Step 3: Commit**

```bash
git add internal/tui/stream_test.go
git commit -m "test(tui): add tool streaming output tests"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 11: Shell 输出折叠/展开 — 新增 Model 状态字段

**Files:**
- Modify: internal/tui/model.go

**Interfaces:**
- Consumes: 无（纯数据结构）
- Produces: Model 新增 shellOutputs/shellExpanded 字段

- [x] **Step 1: 在 Model 结构体中新增 Shell 输出字段**

- [x] **Step 2: 在 New() 中初始化**

- [x] **Step 3: 运行编译检查**

```powershell
cd D:\project\coding-agent; go build ./internal/tui/...
```

- [x] **Step 4: Commit**

```bash
git add internal/tui/model.go
git commit -m "feat(tui): add shell output collapse state fields"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 12: Shell 输出折叠/展开 — ToolResult 处理与存储

**Files:**
- Modify: internal/tui/model.go

**Interfaces:**
- Consumes: event.ToolResult、shellOutputs/shellExpanded
- Produces: bash 工具输出存储到 shellOutputs，超过 1MB 截断

- [x] **Step 1: 在 ToolResult 处理中识别 bash 工具**

- [x] **Step 2: 实现 1MB 截断保护**

- [x] **Step 3: 修改 ToolResult 渲染逻辑**

- [x] **Step 4: 运行编译检查**

```powershell
cd D:\project\coding-agent; go build ./internal/tui/...
```

- [x] **Step 5: Commit**

```bash
git add internal/tui/model.go
git commit -m "feat(tui): store bash output in shellOutputs with 1MB truncation"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 13: Shell 输出折叠/展开 — Ctrl+B 切换与原地重写

**Files:**
- Modify: internal/tui/model.go
- Modify: internal/tui/transcript.go

**Interfaces:**
- Consumes: tea.KeyPressMsg（Ctrl+B）、shellOutputs/shellExpanded
- Produces: Ctrl+B 反向扫描最近 bash 输出块，切换展开/折叠

- [x] **Step 1: 在 KeyPressMsg 处理中新增 Ctrl+B 快捷键**

- [x] **Step 2: 实现 toggleShellExpand 方法**

- [x] **Step 3: 在 ToolResult 处理中将 toolCallID 写入 Raw**

- [x] **Step 4: 运行编译检查**

```powershell
cd D:\project\coding-agent; go build ./internal/tui/...
```

- [x] **Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/transcript.go
git commit -m "feat(tui): implement Ctrl+B shell output toggle with in-place rewrite"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 14: Shell 输出折叠/展开 — 单元测试

**Files:**
- Create: internal/tui/shell_test.go

**Interfaces:**
- Consumes: event.ToolResult、shellOutputs/shellExpanded
- Produces: 测试覆盖 Shell 输出存储、展开/折叠、Ctrl+B 切换

- [x] **Step 1: 编写 Shell 输出折叠测试**

创建 internal/tui/shell_test.go，包含 TestShellOutputStoredOnBashResult、TestShellOutputCollapsedByDefault、TestShellOutputShortNoStorage、TestShellOutputCtrlBToggle、TestShellOutputTruncation。

- [x] **Step 2: 运行测试确认通过**

```powershell
cd D:\project\coding-agent; go test ./internal/tui/... -run "TestShell" -v
```

- [x] **Step 3: Commit**

```bash
git add internal/tui/shell_test.go
git commit -m "test(tui): add shell output collapse tests"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 15: Markdown 渲染升级 — 配置 Chroma 语法高亮

**Files:**
- Modify: internal/tui/markdown.go

**Interfaces:**
- Consumes: glamour.WithChromaStyle()
- Produces: NewGlamourRenderer() 返回启用 chroma 语法高亮的渲染器

- [x] **Step 1: 在 NewGlamourRenderer 中启用 chroma**

在 glamour.NewTermRenderer 调用中添加 glamour.WithChromaStyle() 选项。

- [x] **Step 2: 定义 chromaStyle 变量**

使用 monokai 或 fallback 样式。

- [x] **Step 3: 运行编译检查**

```powershell
cd D:\project\coding-agent; go build ./internal/tui/...
```

- [x] **Step 4: Commit**

```bash
git add internal/tui/markdown.go
git commit -m "feat(tui): enable chroma syntax highlighting in glamour renderer"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 16: Markdown 渲染升级 — Diff 视图着色

**Files:**
- Modify: internal/tui/markdown.go

**Interfaces:**
- Consumes: glamour 渲染后的 ANSI 文本
- Produces: 检测 diff 代码块并叠加 lipgloss 着色

- [x] **Step 1: 实现 diff 代码块检测**

在 Render 方法中，渲染完成后检测 markdown 是否包含 diff 代码块。

- [x] **Step 2: 实现 isDiffMarkdown 检测函数**

- [x] **Step 3: 实现 applyDiffColoring 着色函数**

+ 开头绿色，- 开头红色，@@ 开头青色。

- [x] **Step 4: 运行编译检查**

```powershell
cd D:\project\coding-agent; go build ./internal/tui/...
```

- [x] **Step 5: Commit**

```bash
git add internal/tui/markdown.go
git commit -m "feat(tui): add diff view coloring with lipgloss overlay"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 17: Markdown 渲染升级 — 单元测试

**Files:**
- Modify: internal/tui/markdown_test.go（追加测试）

**Interfaces:**
- Consumes: MarkdownRenderer、diff 着色
- Produces: 测试覆盖 chroma 语法高亮、diff 着色

- [x] **Step 1: 编写 Markdown 升级测试**

追加 TestChromaSyntaxHighlighting、TestDiffViewColoring、TestNonDiffCodeBlockUnchanged。

- [x] **Step 2: 运行测试确认通过**

```powershell
cd D:\project\coding-agent; go test ./internal/tui/... -run "TestChroma|TestDiff|TestNonDiff" -v
```

- [x] **Step 3: Commit**

```bash
git add internal/tui/markdown_test.go
git commit -m "test(tui): add chroma and diff coloring tests"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 18: 文本选择与复制 — 新增 selection 结构体与 Model 字段

**Files:**
- Create: internal/tui/selection.go
- Modify: internal/tui/model.go

**Interfaces:**
- Consumes: 无（纯数据结构）
- Produces: selection 结构体、Model 新增 sel selection 字段

- [x] **Step 1: 创建 selection.go 定义 selection 结构体**

包含 startLine/startCol/endLine/endCol/active 字段和 empty() 方法。

- [x] **Step 2: 在 Model 中新增 sel 字段**

- [x] **Step 3: 运行编译检查**

```powershell
cd D:\project\coding-agent; go build ./internal/tui/...
```

- [x] **Step 4: Commit**

```bash
git add internal/tui/selection.go internal/tui/model.go
git commit -m "feat(tui): add selection struct and model field"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 19: 文本选择与复制 — 鼠标交互处理

**Files:**
- Modify: internal/tui/model.go

**Interfaces:**
- Consumes: tea.MouseMsg、sel selection
- Produces: 鼠标选择状态机

- [x] **Step 1: 在 Update 中新增 MouseMsg 选择处理**

左键按下开始选择，拖动扩展，释放保持，单击取消。

- [x] **Step 2: 处理选择时滚轮/PgUp/PgDn 扩展选择范围**

- [x] **Step 3: 运行编译检查**

```powershell
cd D:\project\coding-agent; go build ./internal/tui/...
```

- [x] **Step 4: Commit**

```bash
git add internal/tui/model.go
git commit -m "feat(tui): implement mouse selection state machine"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 20: 文本选择与复制 — 选择区域高亮渲染

**Files:**
- Modify: internal/tui/view.go
- Modify: internal/tui/selection.go

**Interfaces:**
- Consumes: sel selection、viewport 渲染内容
- Produces: 选中行叠加 lipgloss.Reverse(true) 反色样式

- [x] **Step 1: 在 selection.go 中实现坐标映射辅助方法**

containsLine、highlightLine、highlightRange。

- [x] **Step 2: 在 View() 中应用选择高亮**

- [x] **Step 3: 运行编译检查**

```powershell
cd D:\project\coding-agent; go build ./internal/tui/...
```

- [x] **Step 4: Commit**

```bash
git add internal/tui/view.go internal/tui/selection.go
git commit -m "feat(tui): render selection highlight with reverse style"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 21: 文本选择与复制 — Ctrl+C 复制到剪贴板

**Files:**
- Modify: internal/tui/model.go
- Modify: internal/tui/selection.go
- Modify: go.mod

**Interfaces:**
- Consumes: sel selection、github.com/atotto/clipboard
- Produces: Ctrl+C 在选中时复制文本到剪贴板

- [x] **Step 1: 将 clipboard 依赖提升为 direct**

```powershell
cd D:\project\coding-agent; go get github.com/atotto/clipboard@v0.1.4
```

- [x] **Step 2: 在 selection.go 中实现 extractSelectedText 方法**

- [x] **Step 3: 修改 Ctrl+C 处理逻辑**

先检测选择状态，有选择则复制并重置，否则走原有退出逻辑。

- [x] **Step 4: 运行编译检查**

```powershell
cd D:\project\coding-agent; go build ./internal/tui/...
```

- [x] **Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/selection.go go.mod go.sum
git commit -m "feat(tui): implement Ctrl+C copy selection to clipboard"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 22: 文本选择与复制 — 单元测试

**Files:**
- Create: internal/tui/selection_test.go

**Interfaces:**
- Consumes: selection 结构体、extractSelectedText
- Produces: 测试覆盖选择状态机、文本提取

- [x] **Step 1: 编写文本选择测试**

包含 TestSelectionEmpty、TestSelectionContainsLine、TestExtractSelectedText、TestSelectionResetOnCtrlC。

- [x] **Step 2: 运行测试确认通过**

```powershell
cd D:\project\coding-agent; go test ./internal/tui/... -run "TestSelection|TestExtract" -v
```

- [x] **Step 3: Commit**

```bash
git add internal/tui/selection_test.go
git commit -m "test(tui): add text selection and clipboard tests"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 23: Diff 视图渲染 — diffMaxLines 与折叠

**Files:**
- Modify: internal/tui/model.go
- Modify: internal/tui/toolcard.go

**Interfaces:**
- Consumes: diff 检测逻辑、Model 状态
- Produces: diffMaxLines 字段、diff 块超过阈值时折叠渲染

- [x] **Step 1: 在 Model 中新增 diffMaxLines 字段**

- [x] **Step 2: 在 toolcard.go 中实现 diff 块折叠渲染**

- [x] **Step 3: 在 ToolResult 处理中集成 diff 折叠**

- [x] **Step 4: 运行编译检查**

```powershell
cd D:\project\coding-agent; go build ./internal/tui/...
```

- [x] **Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/toolcard.go
git commit -m "feat(tui): add diffMaxLines and diff block collapse rendering"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 24: Diff 视图渲染 — /diff-fold 斜杠命令

**Files:**
- Modify: internal/tui/model.go

**Interfaces:**
- Consumes: 用户输入文本（以 /diff-fold 开头）
- Produces: 解析 /diff-fold N 命令，更新 diffMaxLines

- [x] **Step 1: 在 submit 中检测 /diff-fold 命令**

- [x] **Step 2: 运行编译检查**

```powershell
cd D:\project\coding-agent; go build ./internal/tui/...
```

- [x] **Step 3: Commit**

```bash
git add internal/tui/model.go
git commit -m "feat(tui): implement /diff-fold slash command"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 25: Diff 视图渲染 — 单元测试

**Files:**
- Create: internal/tui/diffview_test.go

**Interfaces:**
- Consumes: diff 检测、diffMaxLines、/diff-fold 命令
- Produces: 测试覆盖 diff 格式检测、折叠、斜杠命令

- [x] **Step 1: 编写 Diff 视图测试**

包含 TestDiffFormatDetection、TestDiffBlockCollapse、TestDiffFoldCommand、TestDiffFoldDisable。

- [x] **Step 2: 运行测试确认通过**

```powershell
cd D:\project\coding-agent; go test ./internal/tui/... -run "TestDiff" -v
```

- [x] **Step 3: Commit**

```bash
git add internal/tui/diffview_test.go
git commit -m "test(tui): add diff view and /diff-fold command tests"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 26: 集成测试与验证 — 全量回归测试

**Files:**
- 无文件变更（运行测试套件）

**Interfaces:**
- Consumes: 所有功能模块
- Produces: go test ./internal/tui/... 全量通过确认

- [x] **Step 1: 运行全量 TUI 测试**

```powershell
cd D:\project\coding-agent; go test ./internal/tui/... -v
```

- [x] **Step 2: 运行全量 event 测试**

```powershell
cd D:\project\coding-agent; go test ./internal/event/... -v
```

- [x] **Step 3: 运行项目全量测试**

```powershell
cd D:\project\coding-agent; go test ./... -v
```

- [x] **Step 4: 记录回归验证**

确认 go test ./... 输出无 FAIL。

archived-with: 2026-06-25-tui-render-engine
---

### Task 27: 集成测试与验证 — 帮助文本更新

**Files:**
- Modify: internal/tui/view.go

**Interfaces:**
- Consumes: 所有新快捷键
- Produces: 更新 helpText 常量

- [x] **Step 1: 更新 helpText 常量**

包含 Ctrl+O 推理、Ctrl+B Shell、Ctrl+C 退出/复制。

- [x] **Step 2: 运行编译检查**

```powershell
cd D:\project\coding-agent; go build ./...
```

- [x] **Step 3: Commit**

```bash
git add internal/tui/view.go
git commit -m "docs(tui): update help text with new keyboard shortcuts"
```

archived-with: 2026-06-25-tui-render-engine
---

### Task 28: 集成测试与验证 — 最终检查清单

**Files:**
- 无文件变更（手动验证）

**Interfaces:**
- Consumes: 所有功能模块
- Produces: 确认所有功能就绪

- [x] **Step 1: 文件变更清单核对**

| 文件 | 状态 |
|------|------|
| internal/event/event.go | 已修改：Kind 常量 + Event 字段 |
| internal/tui/message.go | 已修改：EntryReasoning |
| internal/tui/model.go | 已修改：状态字段 + 事件处理 + 快捷键 |
| internal/tui/transcript.go | 已修改：推理块渲染 |
| internal/tui/toolcard.go | 已修改：流式输出 + Shell 折叠 |
| internal/tui/markdown.go | 已修改：chroma + diff 着色 |
| internal/tui/view.go | 已修改：选择高亮 + 帮助文本 |
| internal/tui/selection.go | 新建：selection 结构体 + 文本提取 |
| internal/tui/reasoning_test.go | 新建：推理测试 |
| internal/tui/stream_test.go | 新建：流式输出测试 |
| internal/tui/shell_test.go | 新建：Shell 折叠测试 |
| internal/tui/selection_test.go | 新建：选择测试 |
| internal/tui/diffview_test.go | 新建：Diff 视图测试 |
| internal/tui/sink_test.go | 已修改：sink 转发测试 |
| internal/tui/markdown_test.go | 已修改：chroma/diff 测试 |
| go.mod | 已修改：atotto/clipboard 提升为 direct |

- [x] **Step 2: 功能验收清单**

- [x] 推理文本：ReasoningText 事件创建摘要行，Ctrl+O 切换展开/折叠
- [x] 工具流式：ToolProgress 事件尾部截断，ToolResult 折叠摘要
- [x] Shell 折叠：bash 输出默认折叠，Ctrl+B 切换展开
- [x] Markdown：chroma 语法高亮，diff 代码块着色
- [x] 文本选择：鼠标拖选高亮，Ctrl+C 复制到剪贴板
- [x] Diff 视图：diff 块折叠，/diff-fold 命令
- [x] 回归：go test ./... 全量通过

- [x] **Step 3: 最终 Commit**

```bash
git add -A
git commit -m "feat(tui): complete TUI render engine upgrade

- ReasoningText/ToolProgress event types
- Reasoning text rendering with Ctrl+O toggle
- Tool streaming output with tail truncation
- Shell output collapse with Ctrl+B toggle
- Chroma syntax highlighting in markdown
- Diff view coloring
- Text selection with mouse and Ctrl+C copy
- /diff-fold slash command
- Full test coverage"
```

archived-with: 2026-06-25-tui-render-engine
---
