---
change: upgrade-tui-core
design-doc: docs/superpowers/specs/2026-06-23-upgrade-tui-core-design.md
base-ref: d900d929d4bfe49010fc302349e6e9ce7e96d7a7
---

# TUI Core Upgrade 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: 使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 按任务逐步实施。步骤使用 checkbox（`- [ ]`）语法跟踪进度。

**Goal:** 将 `internal/tui/` 从 Bubble Tea v1 纯文本原型升级为具备 Markdown 渲染、工具可视化、审批交互和现代输入/滚动组件的可日常使用 TUI。

**Architecture:** 采用 **D1** 原地升级 Bubble Tea v2 + **D2** bubbles/v2 组件（textarea/viewport/spinner）；**D8** TranscriptEntry 模型驱动 viewport 内容；**D5** 扩展 StreamEmitter 将 agent 工具/审批事件桥接到 TUI；**D3** glamour 封装 MarkdownRenderer 接口；**D4** 段落边界 flush 处理流式 Markdown；**D6** 审批模态横幅；**D7** runewidth 修复 CJK 宽度。

**Tech Stack:** Go 1.26、`charm.land/bubbletea/v2`、`charm.land/bubbles/v2`、`charm.land/lipgloss/v2`、`github.com/charmbracelet/glamour`（或 `charm.land/glamour/v2` 若可用）、`github.com/mattn/go-runewidth`

**参考实现:** DeepSeek-Reasonix `D:\project\DeepSeek-Reasonix\internal\cli\`（`chat_tui.go`、`toolcard.go`、`flushableMarkdownPrefix`、`configureChatTextarea`）

## Global Constraints

- 不修改 `chat` / `once` 命令行为；仅 `tui` 子命令和 `RunStreaming` 路径变更。
- 快捷键语义固定：`Enter` 发送、`Shift+Enter`/`Alt+Enter` 换行、`Esc` 中断当前轮、`Ctrl+C` 退出、`PgUp`/`PgDn`/`Home`/`End` 翻页、鼠标滚轮滚动。
- 流式事件 MUST 通过 `tea.Msg` 进入 Update；禁止 goroutine 直接修改 Model。
- `StreamEmitter` 接口定义在 `internal/agent/emitter.go`（**D5**），避免 agent↔tui 循环依赖。
- Markdown 首版使用 **glamour**（**D3**），通过 `MarkdownRenderer` 接口封装，后续可替换为 goldmark walker。
- 审批并发模型：`chan bool` + `sync.Once` + `ctx.Done()`（**D6**）；Esc 中断通过 context cancel 传播为 denied。
- 每个 task 完成后：`tasks.md` 打勾 → `git commit`（不得积攒）。
- 验证命令：`go build ./cmd/...`、`go test ./internal/tui/... -count=1`、`go test ./internal/agent/... -count=1`。

## 设计决策索引

| 决策 | 内容 | 本计划对应 Task |
|------|------|----------------|
| **D1** | Bubble Tea v1→v2 原地升级，`View() tea.View`，移除 `WithAltScreen` | Task 1 |
| **D2** | bubbles/v2 textarea + viewport + spinner | Task 2 |
| **D3** | Glamour + `MarkdownRenderer` 接口 | Task 8 |
| **D4** | 流式 Markdown 段落边界 flush（`flushableMarkdownPrefix`） | Task 9 |
| **D5** | 扩展 `StreamEmitter`，修改 `RunStreaming` 签名 | Task 5 |
| **D6** | TUI 内嵌审批横幅模态（y/n） | Task 7 |
| **D7** | `go-runewidth` 替代 `utf8.RuneLen` | Task 4 |
| **D8** | `[]TranscriptEntry` + 三区布局 | Task 3, Task 10 |

## 目标文件结构

```
internal/tui/
├── model.go          # Model、Init、Update、submit、interrupt、布局尺寸
├── view.go           # View() tea.View、布局拼接
├── message.go        # TranscriptEntry、EntryKind（D8）
├── stream.go         # StreamChunkMsg、ToolStartMsg、ToolEndMsg、ApprovalRequestMsg
├── runner.go         # Runner、chanEmitter（实现 agent.StreamEmitter）
├── markdown.go       # MarkdownRenderer 接口 + glamour 适配（D3）
├── markdown_test.go
├── toolcard.go       # renderToolCard、工具输出折叠（参考 Reasonix toolcard.go）
├── toolcard_test.go
├── approval.go       # pendingApproval 状态 + renderApprovalBanner（D6）
├── approval_test.go
├── statusbar.go      # 状态栏渲染：spinner + 模型名 + 耗时（D2）
├── width.go          # wrapText with runewidth（D7）
├── width_test.go
├── flush.go          # flushableMarkdownPrefix（D4）
├── flush_test.go
└── *_test.go         # 现有测试迁移

internal/agent/
├── emitter.go        # StreamEmitter 接口 + context 注入（D5）
├── emitter_asker.go  # 通过 emitter 实现 permission.Asker（D6）
└── loop.go           # invokeTool / loopStepWithText 扩展

cmd/cli/
├── tui.go            # tea.NewProgram 移除 WithAltScreen（D1）
├── tui_runner.go     # RunStreaming(ctx, emit) 适配（D5）
└── chat_setup.go     # setupTuiAgent 使用 emitterAsker（D6）
```

---

## Task 1: Bubble Tea v2 依赖升级与 API 迁移（D1）

**输入:** 当前 v1 代码（`go.mod` 中 `bubbletea v1.3.4`）；Design Doc D1 迁移清单。

**输出:** 项目编译通过；`Model.View()` 返回 `tea.View`；所有 TUI 测试适配 v2 API。

**验证标准:** `go build ./cmd/...` 成功；`go test ./internal/tui/... -count=1` 全部 PASS。

**Files:**
- Modify: `go.mod`, `go.sum`
- Modify: `internal/tui/model.go`, `internal/tui/view.go`
- Modify: `internal/tui/model_test.go`, `internal/tui/keymap_test.go`, `internal/tui/runner_test.go`
- Modify: `cmd/cli/tui.go`

**Interfaces:**
- Produces: `func (m Model) View() tea.View`；`tea.KeyPressMsg` 键处理（v2 替代 `tea.KeyMsg.Type`）

- [x] **Step 1: 升级 go.mod 依赖**

```bash
go get charm.land/bubbletea/v2@latest
go get charm.land/lipgloss/v2@latest
go get charm.land/bubbles/v2@latest
go get github.com/mattn/go-runewidth@latest
go mod tidy
```

预期：`go.mod` 中 `github.com/charmbracelet/bubbletea` 被替换为 `charm.land/bubbletea/v2`。

- [x] **Step 2: 批量替换 import path**

在所有 `internal/tui/` 和 `cmd/cli/tui.go` 中：

```go
// Before
tea "github.com/charmbracelet/bubbletea"
"github.com/charmbracelet/lipgloss"

// After
tea "charm.land/bubbletea/v2"
"charm.land/lipgloss/v2"
```

- [x] **Step 3: 迁移 View() 返回 tea.View（D1）**

`internal/tui/view.go`:

```go
func (m Model) View() tea.View {
    if m.quitting {
        return tea.NewView("")
    }
    content := /* 现有 joinLines 逻辑 */
    v := tea.NewView(content)
    v.AltScreen = true
    v.MouseMode = tea.MouseModeCellMotion
    return v
}
```

参考 Reasonix `chat_tui.go:2358-2360`。

- [x] **Step 4: 迁移 cmd/cli/tui.go（D1）**

```go
// Before
p := tea.NewProgram(m, tea.WithAltScreen())

// After
p := tea.NewProgram(m)
```

- [x] **Step 5: 迁移 KeyMsg → KeyPressMsg（D1）**

v2 使用 `tea.KeyPressMsg`，键匹配改为 `msg.String()` 或 `key.Matches(msg, binding)`：

```go
case tea.KeyPressMsg:
    switch msg.String() {
    case "ctrl+c":
        // ...
    case "esc":
        // ...
    case "enter":
        return m.submit()
    }
```

测试文件同步更新：`tea.KeyMsg{Type: tea.KeyCtrlC}` → `tea.KeyPressMsg{Code: tea.KeyCtrlC}` 或构造 helper。

- [x] **Step 6: 修复测试并验证**

```bash
go test ./internal/tui/... -count=1 -v
go build ./cmd/...
```

预期：所有现有测试 PASS，行为与 v1 等价（纯文本模式暂保留）。

- [x] **Step 7: Commit**

```bash
git add go.mod go.sum internal/tui/ cmd/cli/tui.go
git commit -m "refactor(tui): migrate Bubble Tea v1 to v2 API"
```

勾选 `tasks.md` 1.1、1.2、1.6、1.7。

---

## Task 2: bubbles/v2 组件嵌入（D2）

**输入:** Task 1 完成的 v2 Model 骨架。

**输出:** Model 嵌入 `textarea.Model`、`viewport.Model`、`spinner.Model`；多行输入、viewport 滚动、busy 时 spinner 动画可用。

**验证标准:** textarea 支持 Shift+Enter 换行、Enter 在 Model 层拦截提交；viewport 响应 PgUp/PgDn/滚轮；busy 时 spinner tick 运行。

**Files:**
- Modify: `internal/tui/model.go`, `internal/tui/view.go`
- Create: `internal/tui/components.go`（可选：textarea/viewport/spinner 初始化 helper）
- Modify: `internal/tui/model_test.go`, `internal/tui/keymap_test.go`

**Interfaces:**
- Consumes: `tea.View`、`tea.KeyPressMsg`（Task 1）
- Produces: `textarea.Model`、`viewport.Model`、`spinner.Model` 嵌入 Model；`configureTextarea()` 配置

- [x] **Step 1: 编写 textarea 配置测试**

```go
func TestTextareaShiftEnterInsertsNewline(t *testing.T) {
    m := New()
    m.textarea.Focus()
    m.textarea.SetValue("line1")
    // 模拟 shift+enter — 通过 Update 路由到 textarea
    next, _ := m.Update(tea.KeyPressMsg{...}) // shift+enter binding
    updated := next.(Model)
    if !strings.Contains(updated.textarea.Value(), "\n") {
        t.Fatal("expected newline after shift+enter")
    }
}
```

- [x] **Step 2: 实现 textarea 初始化（D2，参考 Reasonix configureChatTextarea）**

```go
import (
    "charm.land/bubbles/v2/key"
    "charm.land/bubbles/v2/textarea"
)

func newTextarea() textarea.Model {
    ti := textarea.New()
    ti.Prompt = ""
    ti.CharLimit = 16384
    ti.DynamicHeight = true
    ti.SetHeight(1)
    ti.MaxHeight = 5
    ti.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "shift+enter"))
    ti.Focus()
    return ti
}
```

- [x] **Step 3: 实现 viewport 初始化（D2）**

```go
import "charm.land/bubbles/v2/viewport"

func newViewport() viewport.Model {
    vp := viewport.New(viewport.WithWidth(80))
    vp.MouseWheelEnabled = true
    vp.MouseWheelDelta = 3
    return vp
}
```

- [x] **Step 4: 实现 spinner 初始化（D2）**

```go
import "charm.land/bubbles/v2/spinner"

func newSpinner() spinner.Model {
    s := spinner.New()
    s.Spinner = spinner.Dot
    return s
}
```

- [x] **Step 5: 重构 Model 结构与 Update 路由**

```go
type Model struct {
    transcript   []TranscriptEntry // Task 3 先可暂用 []Message 过渡
    textarea     textarea.Model
    viewport     viewport.Model
    spinner      spinner.Model
    // ... 保留 busy, runner, streamCh 等
}
```

Update 中：
- `tea.WindowSizeMsg` → 调整 viewport/textarea 尺寸
- `tea.MouseMsg` → 转发给 viewport（非审批模态时）
- `spinner.TickMsg` → 转发给 spinner
- Enter → 拦截提交（不传给 textarea）；其他键 → 转发 textarea
- busy 时 Init 返回 `spinner.Tick`；StreamDone 停止

- [x] **Step 6: tail-follow 逻辑（D2）**

```go
func (m Model) followTailIfAtBottom() Model {
    if m.viewport.AtBottom() {
        m.viewport.GotoBottom()
    }
    return m
}
```

新 transcript 内容写入后调用。

- [x] **Step 7: 运行测试**

```bash
go test ./internal/tui/... -run 'Textarea|Viewport|Spinner' -count=1 -v
```

- [x] **Step 8: Commit**

勾选 `tasks.md` 1.3、1.4、1.5。

---

## Task 3: Transcript 数据模型（D8）

**输入:** Task 2 的 Model 结构。

**输出:** `[]TranscriptEntry` 替代 `[]Message`；viewport 内容由 entries 拼接；resize 时可从 Raw 重渲染。

**验证标准:** 单元测试覆盖各 EntryKind 追加与 viewport 内容生成；用户/助手消息正确显示。

**Files:**
- Modify: `internal/tui/message.go`
- Modify: `internal/tui/model.go`, `internal/tui/view.go`
- Create: `internal/tui/transcript.go`（appendEntry、renderTranscript、rebuildViewport）
- Create: `internal/tui/transcript_test.go`

**Interfaces:**
- Produces:
  ```go
  type EntryKind int
  const (
      EntryUserMessage EntryKind = iota
      EntryAssistantChunk
      EntryToolCard
      EntryToolOutput
      EntryError
  )
  type TranscriptEntry struct {
      Kind    EntryKind
      Content string // pre-rendered ANSI
      Raw     string // raw for re-render on resize
  }
  func (m *Model) appendEntry(e TranscriptEntry)
  func (m Model) renderTranscriptContent() string
  ```

- [x] **Step 1: 编写 failing test**

```go
func TestAppendUserEntryUpdatesViewport(t *testing.T) {
    m := New()
    m.width, m.height = 80, 24
    m = m.appendEntry(TranscriptEntry{Kind: EntryUserMessage, Content: "user bubble", Raw: "hello"})
    content := m.renderTranscriptContent()
    if !strings.Contains(content, "user bubble") {
        t.Fatalf("missing entry: %s", content)
    }
}
```

- [x] **Step 2: 实现 TranscriptEntry 与 append 逻辑（D8）**

用户消息暂用 lipgloss 简单气泡样式（无需 Markdown）；助手 chunk 在 Task 8 接入 renderer。

- [x] **Step 3: 重构 submit/stream 使用 transcript**

`submit()` 追加 `EntryUserMessage`；`StreamChunkMsg` 暂直接追加 raw 文本到 pending（Task 9 改 flush）。

- [x] **Step 4: viewport.SetContent(renderTranscriptContent())**

每次 transcript 变更后 rebuild viewport content 并 tail-follow。

- [x] **Step 5: WindowSizeMsg 触发 re-render**

从 Raw 字段重新渲染 assistant entries（width 变化时）。

- [x] **Step 6: 迁移现有 model_test 使用 transcript API**

- [x] **Step 7: Commit**

---

## Task 4: CJK 显示宽度修复（D7）

**输入:** 现有 `wrapText`（`model.go:283-313`，使用 `utf8.RuneLen`）。

**输出:** `internal/tui/width.go` 使用 `runewidth.RuneWidth()`；中文/emoji 换行正确。

**验证标准:** `width_test.go` 覆盖中文、日文、emoji 混合场景；行宽不超过终端宽度。

**Files:**
- Create: `internal/tui/width.go`, `internal/tui/width_test.go`
- Modify: `internal/tui/model.go`（移除旧 wrapText，调用 width 包）

**Interfaces:**
- Produces: `func WrapText(text string, width int) []string`

- [x] **Step 1: 编写 CJK failing test（D7）**

```go
func TestWrapTextCJKDoubleWidth(t *testing.T) {
    // "你好世界" = 4 runes × 2 width = 8; width=6 should break after 3 chars
    lines := WrapText("你好世界", 6)
    if len(lines) != 2 {
        t.Fatalf("got %d lines, want 2: %v", len(lines), lines)
    }
}

func TestWrapTextEmoji(t *testing.T) {
    lines := WrapText("Hi 👋 there", 8)
    // emoji width varies; assert no line exceeds width via runewidth.StringWidth
    for _, ln := range lines {
        if runewidth.StringWidth(ln) > 8 {
            t.Fatalf("line %q exceeds width 8", ln)
        }
    }
}
```

- [x] **Step 2: 实现 WrapText（D7）**

```go
import "github.com/mattn/go-runewidth"

func WrapText(text string, width int) []string {
    if width <= 0 { return []string{text} }
    var lines []string
    var current strings.Builder
    currentW := 0
    flush := func() {
        if current.Len() > 0 {
            lines = append(lines, current.String())
            current.Reset()
            currentW = 0
        }
    }
    for _, r := range text {
        rw := runewidth.RuneWidth(r)
        if currentW+rw > width && currentW > 0 {
            flush()
        }
        current.WriteRune(r)
        currentW += rw
    }
    flush()
    return lines
}
```

- [x] **Step 3: 更新 prefix 宽度计算（tasks 2.2）**

用户消息 prefix 宽度用 `runewidth.StringWidth(prefix)` 而非 `len(prefix)`。

- [x] **Step 4: 运行测试**

```bash
go test ./internal/tui/... -run WrapText -count=1 -v
```

- [x] **Step 5: Commit**

勾选 `tasks.md` 2.1、2.2、2.3。

---

## Task 5: StreamEmitter 扩展与 Agent 集成（D5）

**输入:** 当前 `StreamEmitter`（仅 OnChunk/OnDone/OnError）；`loop.go` invokeTool 无 emitter 回调。

**输出:** `internal/agent/emitter.go` 定义完整接口；`RunStreaming` 签名变更；loop 在工具执行前后 emit 事件；TUI chanEmitter 转发新 Msg 类型。

**验证标准:** agent 单元测试验证 invokeTool 调用 OnToolStart/OnToolEnd；runner_test 验证 channel 收到 ToolStartMsg。

**Files:**
- Create: `internal/agent/emitter.go`, `internal/agent/emitter_test.go`
- Modify: `internal/agent/loop.go`, `internal/agent/agent.go`
- Modify: `internal/tui/runner.go`, `internal/tui/stream.go`
- Modify: `cmd/cli/tui_runner.go`

**Interfaces:**
- Produces:
  ```go
  // internal/agent/emitter.go
  type StreamEmitter interface {
      OnChunk(text string)
      OnToolStart(name string, args string)
      OnToolEnd(name string, result string, isError bool)
      OnApprovalRequest(name string, args map[string]any, respond func(bool))
      OnDone()
      OnError(err error)
  }
  type emitterContextKey struct{}
  func WithEmitter(ctx context.Context, e StreamEmitter) context.Context
  func EmitterFromContext(ctx context.Context) StreamEmitter
  ```

- [x] **Step 1: 创建 emitter.go 与测试 stub**

```go
type recordingEmitter struct {
    tools []string
}
func (r *recordingEmitter) OnToolStart(name, args string) { r.tools = append(r.tools, "start:"+name) }
func (r *recordingEmitter) OnToolEnd(name, result string, isError bool) { r.tools = append(r.tools, "end:"+name) }
// ... other methods no-op
```

- [x] **Step 2: 扩展 stream.go 消息类型（D5）**

```go
type ToolStartMsg struct { Name, Args string }
type ToolEndMsg struct { Name, Result string; IsError bool }
type ApprovalRequestMsg struct {
    Name string
    Args map[string]any
    Respond func(bool)
}
```

- [x] **Step 3: 修改 loopStepWithText 签名（D5）**

```go
func (a *Agent) loopStepWithText(ctx context.Context, emitter StreamEmitter) (final string, err error) {
    onText := func(s string) {
        if emitter != nil { emitter.OnChunk(s) }
    }
    msg, usage, streamErr := provider.CollectWithText(ch, onText)
    // ...
}
```

- [x] **Step 4: 修改 invokeTool 注入工具事件（D5）**

```go
func (a *Agent) invokeTool(ctx context.Context, tc provider.ToolCall, emitter StreamEmitter) string {
    // parse args ...
    if emitter != nil {
        emitter.OnToolStart(name, tc.Arguments)
        defer func() {
            isErr := strings.HasPrefix(result, "Error:") || strings.HasPrefix(result, "Permission denied")
            emitter.OnToolEnd(name, result, isErr)
        }()
    }
    // existing permission + execute ...
}
```

同步更新 `executeBatch` 传递 emitter（从 context 或参数）。

- [x] **Step 5: 修改 RunStreaming 签名（D5）**

```go
func (a *Agent) RunStreaming(ctx context.Context, userInput string, emitter StreamEmitter) (string, error) {
    ctx = WithEmitter(ctx, emitter)
    // loop: loopStepWithText(ctx, emitter)
}
```

- [x] **Step 6: 更新 tui_runner.go 与 chanEmitter**

```go
func (r agentRunner) RunTurn(ctx context.Context, prompt string, emit tui.StreamEmitter) error {
    _, err := r.agent.RunStreaming(ctx, prompt, emit) // emit 实现 agent.StreamEmitter
    // ...
}
```

`internal/tui/runner.go` 中 `StreamEmitter` 改为 type alias 或嵌入 `agent.StreamEmitter`：

```go
type StreamEmitter = agent.StreamEmitter
```

chanEmitter 实现 OnToolStart/OnToolEnd/OnApprovalRequest。

- [x] **Step 7: 运行测试**

```bash
go test ./internal/agent/... ./internal/tui/... -count=1
```

- [x] **Step 8: Commit**

勾选 `tasks.md` 3.1–3.6。

---

## Task 6: 工具调用可视化（D5 + D8）

**输入:** Task 5 的 ToolStartMsg/ToolEndMsg；Task 3 的 TranscriptEntry。

**输出:** `toolcard.go` 渲染工具卡片；工具输出折叠展示；Update 处理工具事件更新 transcript 与 spinner 文案。

**验证标准:** `toolcard_test.go` 验证卡片格式；集成测试验证 ToolStart→ToolEnd 事件流。

**Files:**
- Create: `internal/tui/toolcard.go`, `internal/tui/toolcard_test.go`
- Modify: `internal/tui/model.go`, `internal/tui/statusbar.go`（busy 时显示工具名）

**Interfaces:**
- Produces:
  ```go
  func renderToolCard(name, args string, width int) string
  func renderToolOutput(result string, maxLines int) (content string, collapsed bool)
  const toolOutputCollapseLines = 8
  ```

- [x] **Step 1: 编写 toolcard test（参考 Reasonix toolcard.go）**

```go
func TestRenderToolCardTruncatesLongArgs(t *testing.T) {
    long := strings.Repeat("x", 200)
    card := renderToolCard("Read", `{"path":"`+long+`"}`, 60)
    if !strings.Contains(card, "●") || !strings.Contains(card, "Read") {
        t.Fatalf("unexpected card: %s", card)
    }
    if runewidth.StringWidth(stripANSI(card)) > 80 {
        t.Fatal("card too wide")
    }
}
```

- [x] **Step 2: 实现 renderToolCard（D8 工具卡片样式）**

格式：`● ToolName("args summary")`，使用 lipgloss 颜色区分 read/write/error。
参数摘要：从 JSON args 提取主要字段（path/command 等），超长截断。

- [x] **Step 3: 实现 renderToolOutput 折叠（>8 行）**

```go
func renderToolOutput(result string, maxLines int) string {
    lines := strings.Split(result, "\n")
    if len(lines) <= maxLines {
        return connectorBlock(lines) // "  ⎿  " gutter，参考 Reasonix
    }
    visible := lines[:maxLines]
    summary := fmt.Sprintf("  ⎿  (%d lines, collapsed)", len(lines))
    return connectorBlock(visible) + "\n" + summary
}
```

- [x] **Step 4: Update 处理 ToolStartMsg / ToolEndMsg**

```go
case ToolStartMsg:
    m.statusLabel = "running " + msg.Name + "..."
    m, cmd = m.spinnerUpdate(msg)
case ToolEndMsg:
    m.appendEntry(TranscriptEntry{Kind: EntryToolCard, Content: renderToolCard(...), Raw: ...})
    m.appendEntry(TranscriptEntry{Kind: EntryToolOutput, Content: renderToolOutput(msg.Result, 8), Raw: msg.Result})
    m.statusLabel = "thinking"
```

- [x] **Step 5: 运行测试**

```bash
go test ./internal/tui/... -run Tool -count=1 -v
```

- [x] **Step 6: Commit**

勾选 `tasks.md` 4.1–4.4。

---

## Task 7: 审批交互（D6）

**输入:** Task 5 的 OnApprovalRequest / ApprovalRequestMsg；当前 `setupTuiAgent` 全自动拒绝。

**输出:** TUI 审批横幅；y/n 按键路由；agent 通过 emitterAsker 阻塞等待；Esc/context cancel 返回 denied。

**验证标准:** approval_test 验证模态切换与 respond 单次调用；Esc 中断时 agent 收到 false。

**Files:**
- Create: `internal/agent/emitter_asker.go`, `internal/tui/approval.go`, `internal/tui/approval_test.go`
- Modify: `internal/tui/model.go`, `internal/tui/view.go`
- Modify: `cmd/cli/chat_setup.go`

**Interfaces:**
- Produces:
  ```go
  type pendingApproval struct {
      toolName string
      args     map[string]any
      respond  func(bool)
  }
  func requestApproval(ctx context.Context, emitter StreamEmitter, name string, args map[string]any) bool
  func renderApprovalBanner(a pendingApproval, width int) string
  ```

- [x] **Step 1: 实现 requestApproval（D6，Design Doc 代码）**

```go
func requestApproval(ctx context.Context, emitter StreamEmitter, name string, args map[string]any) bool {
    ch := make(chan bool, 1)
    var once sync.Once
    respond := func(ok bool) { once.Do(func() { ch <- ok; close(ch) }) }
    emitter.OnApprovalRequest(name, args, respond)
    select {
    case approved := <-ch:
        return approved
    case <-ctx.Done():
        respond(false)
        return false
    }
}
```

- [x] **Step 2: 实现 emitterAsker（permission.Asker via context）**

```go
// internal/agent/emitter_asker.go
type EmitterAsker struct{}
func (EmitterAsker) Ask(ctx context.Context, name string, args map[string]any, reason string) bool {
    emitter := EmitterFromContext(ctx)
    if emitter == nil { return false }
    return requestApproval(ctx, emitter, name, args)
}
```

- [x] **Step 3: 修改 chat_setup.go（D6）**

```go
func setupTuiAgent(cmd *cobra.Command) (*chatSetup, error) {
    asker := agent.EmitterAsker{}
    return setupAgentWithAsker(cmd, asker)
}
```

- [x] **Step 4: Model 审批模态（D6）**

```go
case ApprovalRequestMsg:
    m.approval = &pendingApproval{toolName: msg.Name, args: msg.Args, respond: msg.Respond}
```

审批模态时 Update 拦截 y/n，忽略 textarea/viewport 事件：

```go
func (m Model) handleApprovalKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
    if m.approval == nil { return m, nil }
    switch msg.String() {
    case "y", "Y":
        m.approval.respond(true)
        m.approval = nil
    case "n", "N":
        m.approval.respond(false)
        m.approval = nil
    }
    return m, nil
}
```

- [x] **Step 5: renderApprovalBanner（D6）**

```
Allow Write("config.yaml")? [y]es [n]o
```

- [x] **Step 6: 编写 approval_test.go**

测试 ApprovalRequestMsg → y → respond(true) 被调用一次；n → respond(false)；Esc → context cancel → false。

- [x] **Step 7: Commit**

勾选 `tasks.md` 5.1–5.5。

---

## Task 8: Markdown ANSI 渲染（D3）

**输入:** Design Doc D3 glamour 方案；Open Question 关于 glamour v2 兼容性。

**输出:** `markdown.go` 实现 `MarkdownRenderer` 接口；glamour 适配器；基础元素渲染测试。

**验证标准:** 代码块/列表/表格/粗体测试 PASS；输出含 ANSI escape codes。

**Files:**
- Create: `internal/tui/markdown.go`, `internal/tui/markdown_test.go`
- Modify: `go.mod`（glamour 依赖）

**Interfaces:**
- Produces:
  ```go
  type MarkdownRenderer interface {
      Render(markdown string, width int) string
  }
  func NewGlamourRenderer() MarkdownRenderer
  ```

- [x] **Step 1: 验证 glamour 依赖兼容性**

```bash
go get github.com/charmbracelet/glamour@latest
# 若与 lipgloss/v2 冲突，尝试 charm.land/glamour/v2
go test -run=NonExistent ./internal/tui/... # compile check
```

- [x] **Step 2: 编写 markdown test（D3）**

```go
func TestGlamourRendererCodeBlock(t *testing.T) {
    r := NewGlamourRenderer()
    md := "# Title\n\n```go\nfunc main() {}\n```"
    out := r.Render(md, 80)
    if !strings.Contains(out, "main") {
        t.Fatalf("missing code content: %s", out)
    }
    if !strings.Contains(out, "\x1b[") { // ANSI present
        t.Fatal("expected ANSI styling")
    }
}
```

- [x] **Step 3: 实现 glamourRenderer（D3）**

```go
func NewGlamourRenderer() MarkdownRenderer { return glamourRenderer{} }

type glamourRenderer struct{}

func (g glamourRenderer) Render(md string, width int) string {
    r, err := glamour.NewTermRenderer(
        glamour.WithAutoStyle(),
        glamour.WithWordWrap(width),
    )
    if err != nil { return md }
    out, err := r.Render(md)
    if err != nil { return md }
    return out
}
```

- [x] **Step 4: 补充列表/表格/引用测试**

- [x] **Step 5: Model 注入 mdRenderer，助手消息渲染时使用**

用户消息保持简单文本气泡；助手 EntryAssistantChunk 使用 renderer。

- [x] **Step 6: Commit**

勾选 `tasks.md` 6.1–6.8（glamour 替代 goldmark walker，符合 Design Doc D3）。

---

## Task 9: 流式 Markdown 段落边界刷新（D4）

**输入:** Task 8 的 MarkdownRenderer；当前 StreamChunkMsg 直接追加逻辑。

**输出:** `flush.go` 实现 `flushableMarkdownPrefix`；pending buffer + 段落 flush 流程；StreamDone 时 flush 剩余。

**验证标准:** flush_test 覆盖半代码块、跨段落、空输入；参考 Reasonix `chat_render_test.go:170-179`。

**Files:**
- Create: `internal/tui/flush.go`, `internal/tui/flush_test.go`
- Modify: `internal/tui/model.go`

**Interfaces:**
- Produces:
  ```go
  func flushableMarkdownPrefix(buf string) (renderable, remaining string)
  ```

- [x] **Step 1: 编写 flush test（D4，移植 Reasonix 逻辑）**

```go
func TestFlushablePrefixHalfCodeBlock(t *testing.T) {
    open := "intro line\n\n```go\ncode"
    renderable, remaining := flushableMarkdownPrefix(open)
    if renderable != "intro line" {
        t.Fatalf("renderable=%q, want intro line", renderable)
    }
    if !strings.Contains(remaining, "```") {
        t.Fatal("code block should remain buffered")
    }
}

func TestFlushablePrefixCompleteBlock(t *testing.T) {
    closed := "```go\ncode\n```\n\nmore"
    renderable, _ := flushableMarkdownPrefix(closed)
    if !strings.Contains(renderable, "```") {
        t.Fatalf("complete block should be renderable: %q", renderable)
    }
}
```

- [x] **Step 2: 实现 flushableMarkdownPrefix（D4，参考 Reasonix chat_tui.go:2082-2099）**

```go
func flushableMarkdownPrefix(buf string) (renderable, remaining string) {
    lines := strings.Split(buf, "\n")
    inFence := false
    boundary := -1
    for i, ln := range lines {
        t := strings.TrimSpace(ln)
        if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
            inFence = !inFence
            continue
        }
        if !inFence && t == "" {
            boundary = i
        }
    }
    if boundary <= 0 {
        return "", buf
    }
    return strings.Join(lines[:boundary], "\n"), strings.Join(lines[boundary:], "\n")
}
```

- [x] **Step 3: 重构 StreamChunkMsg 处理（D4）**

```go
case StreamChunkMsg:
    m.pending.WriteString(msg.Text)
    if renderable, rest := flushableMarkdownPrefix(m.pending.String()); renderable != "" {
        rendered := m.mdRenderer.Render(renderable, m.contentWidth())
        m.appendAssistantRendered(rendered, renderable)
        m.pending.Reset()
        m.pending.WriteString(rest)
    }
    m = m.followTailIfAtBottom()
```

- [x] **Step 4: StreamDoneMsg flush 剩余 pending**

```go
case StreamDoneMsg:
    if m.pending.Len() > 0 {
        rendered := m.mdRenderer.Render(m.pending.String(), m.contentWidth())
        m.appendAssistantRendered(rendered, m.pending.String())
        m.pending.Reset()
    }
```

- [x] **Step 5: 运行测试**

```bash
go test ./internal/tui/... -run Flush -count=1 -v
```

- [x] **Step 6: Commit**

勾选 `tasks.md` 7.1–7.4。

---

## Task 10: 布局整合与状态栏（D2 + D8）

**输入:** Task 2-9 全部模块。

**输出:** 完整三区布局；bottomHeight 动态计算；状态栏显示模型名 + spinner + 耗时；帮助文本更新。

**验证标准:** 不同终端尺寸下 viewport 高度正确；审批横幅出现/消失时布局重算；View 输出包含 help 行。

**Files:**
- Create: `internal/tui/statusbar.go`
- Modify: `internal/tui/view.go`, `internal/tui/model.go`

**Interfaces:**
- Produces:
  ```go
  func (m Model) bottomHeight() int
  func renderStatusBar(m Model) string
  func (m Model) layout() // viewport H = term H - bottomHeight
  ```

- [x] **Step 1: 实现 bottomHeight（D8）**

```go
func (m Model) bottomHeight() int {
    h := 1 // status bar
    h += 1 // help
    h += m.textarea.Height()
    if m.approval != nil {
        h += 2 // approval banner
    }
    if m.lastError != "" {
        h += 1
    }
    return h
}
```

- [x] **Step 2: WindowSizeMsg 布局（D8）**

```go
case tea.WindowSizeMsg:
    m.width, m.height = msg.Width, msg.Height
    vpHeight := m.height - m.bottomHeight()
    if vpHeight < 1 { vpHeight = 1 }
    m.viewport.Width = msg.Width
    m.viewport.Height = vpHeight
    m.textarea.SetWidth(msg.Width)
    m.rebuildTranscript() // re-render from Raw at new width
```

- [x] **Step 3: 实现 statusbar.go（D2）**

```go
func renderStatusBar(m Model) string {
    if m.busy {
        spin := m.spinner.View()
        elapsed := time.Since(m.runStart).Truncate(time.Second)
        label := m.statusLabel
        if label == "" { label = "thinking" }
        return fmt.Sprintf("%s %s (%s)", spin, label, elapsed)
    }
    model := m.modelName // 从 runner/config 注入
    return fmt.Sprintf("%s │ idle", model)
}
```

- [x] **Step 4: 重构 View() 布局拼接（D8）**

```
[viewport content]
[approval banner — optional]
[status bar]
[textarea with > prefix]
[help: Shift+Enter 换行 · Enter 发送 · Esc 中断 · Ctrl+C 退出]
```

参考 Design Doc 布局 ASCII 图。

- [x] **Step 5: 更新帮助文本（tasks 8.5）**

移除 j/k 滚动提示（改由 viewport 原生支持 PgUp/PgDn）。

- [x] **Step 6: Commit**

勾选 `tasks.md` 8.1–8.5。

---

## Task 11: 集成测试与回归验证

**输入:** Task 1-10 全部完成。

**输出:** 完整测试覆盖；`go test ./...` 全绿；手动 smoke test 清单。

**验证标准:** 所有 spec scenario 有对应测试或手动验证项；编译与测试零失败。

**Files:**
- Modify: `internal/tui/model_test.go`, `internal/tui/runner_test.go`, `internal/tui/keymap_test.go`
- Create: `internal/tui/integration_test.go`

- [ ] **Step 1: 更新现有测试适配新 Model 结构（tasks 9.1）**

移除对 `m.input`、`m.scrollOffset`、`m.messages` 的直接引用；改用 textarea/viewport/transcript API。

- [ ] **Step 2: 工具事件流集成测试（tasks 9.2）**

```go
func TestToolEventFlowUpdatesTranscript(t *testing.T) {
    ch := make(chan any, 8)
    m := New()
    m.busy = true
    m.streamCh = ch
    // ToolStartMsg → statusLabel 变化
    // ToolEndMsg → transcript 含 tool card
}
```

- [ ] **Step 3: 审批流程集成测试（tasks 9.3）**

- [ ] **Step 4: CJK + Markdown 组合测试（tasks 9.4）**

- [ ] **Step 5: 全量回归**

```bash
go build ./cmd/...
go test ./... -count=1
```

- [ ] **Step 6: 手动 smoke test 清单（tasks 9.5）**

| 场景 | 操作 | 预期 |
|------|------|------|
| 启动 | `coding-agent tui` | 全屏 TUI，textarea 聚焦 |
| 多行输入 | Shift+Enter ×2 + Enter | 消息发送，textarea 清空 |
| 流式 Markdown | 等待助手回复 | 段落边界刷新，代码块完整 |
| 工具调用 | 触发 Read 工具 | 工具卡片 + 折叠输出 |
| 审批 | 触发 bash rm | 横幅 y/n |
| 中断 | 流式中 Esc | 保留历史，可继续输入 |
| 退出 | Ctrl+C | 安全退出 |

- [ ] **Step 7: Commit + 勾选 tasks.md 9.1–9.5**

---

## Self-Review 清单

### Spec 覆盖

| Requirement | Task |
|-------------|------|
| textarea 多行输入 | Task 2 |
| viewport 滚动/滚轮 | Task 2 |
| 快捷键 Enter/Esc/Ctrl+C | Task 1, 2 |
| Markdown ANSI 渲染 | Task 8 |
| 流式段落边界 | Task 9 |
| 工具调用可视化 | Task 6 |
| 交互式审批 | Task 7 |
| spinner + 耗时 | Task 2, 10 |
| 状态栏模型名 | Task 10 |
| CJK 宽度 | Task 4 |

### 已知风险

1. **glamour v2 兼容性**（Design Doc Open Question）：Task 8 Step 1 先验证；冲突时暂用 v1 glamour + 独立 lipgloss 样式。
2. **审批横幅高度变化闪烁**：Task 10 在 approval 切换时强制 `GotoBottom()` 若 AtBottom。
3. **tasks.md §6 goldmark vs Design Doc D3 glamour**：本计划以 Design Doc 为准，使用 glamour。

### 依赖顺序

```
Task 1 (v2) → Task 2 (components) → Task 3 (transcript)
     ↓
Task 4 (CJK) ─────────────────────────────┐
     ↓                                     │
Task 5 (emitter) → Task 6 (toolcard)       │
              → Task 7 (approval)          │
Task 8 (markdown) → Task 9 (flush) ────────┤
                                           ↓
                              Task 10 (layout) → Task 11 (integration)
```

---

**Plan complete and saved to `docs/superpowers/plans/2026-06-23-upgrade-tui-core.md`.**

**Two execution options:**

1. **Subagent-Driven（推荐）** — 每个 Task 派发独立 subagent，任务间双阶段审查，快速迭代
2. **Inline Execution** — 当前 session 使用 executing-plans 批量执行，checkpoint 暂停审查

**Which approach?**
