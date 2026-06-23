---
comet_change: upgrade-tui-core
role: technical-design
canonical_spec: openspec
---

# TUI Core Upgrade — Technical Design

## Context

当前 `internal/tui/` 是 Bubble Tea v1 上 ~400 行的 v0 原型，只有纯文本输出和原始字符串输入。本次将其升级为具备 Markdown 渲染、工具可视化、审批交互和现代输入组件的可日常使用 TUI，参考 DeepSeek-Reasonix 的成熟 TUI 架构。

**参考实现**：DeepSeek-Reasonix（`D:\project\DeepSeek-Reasonix\internal\cli\`），Bubble Tea v2 + bubbles/v2 + goldmark/chroma，~3800 行，40+ 文件。

## Architecture

### 布局

```
┌─── Viewport (bubbles/v2 viewport) ───────────────────┐
│ User message bubble                                    │
│ ● Read("src/main.go")          ← tool card            │
│ ⎿ result (8 lines, collapsed)  ← tool output          │
│ Assistant reply (Markdown rendered via glamour)         │
│                                               scrollbar│
└────────────────────────────────────────────────────────┘
┌─── Approval Banner (conditional) ─────────────────────┐
│ Allow Write("config.yaml")? [y]es [n]o                 │
└────────────────────────────────────────────────────────┘
┌─── Status Bar ────────────────────────────────────────┐
│ ⣾ thinking (3s) │ deepseek-v3 │ idle                  │
└────────────────────────────────────────────────────────┘
┌─── Textarea (bubbles/v2 textarea) ────────────────────┐
│ > multi-line input with cursor                         │
└────────────────────────────────────────────────────────┘
┌─── Help ──────────────────────────────────────────────┐
│ Shift+Enter 换行 · Enter 发送 · Esc 中断 · Ctrl+C 退出│
└────────────────────────────────────────────────────────┘
```

Viewport height = terminal height - textarea height - status bar - help - optional approval banner.

### Module Structure

```
internal/tui/
├── model.go          # Model struct, Init, Update, submit, interrupt
├── view.go           # View(), layout composition, styles
├── message.go        # Message, Role types + transcript entries (tool cards, etc.)
├── stream.go         # StreamChunkMsg, ToolStartMsg, ToolEndMsg, ApprovalRequestMsg, etc.
├── runner.go         # Runner, StreamEmitter interfaces, chanEmitter
├── markdown.go       # Renderer interface + glamour adapter + flushablePrefix
├── toolcard.go       # renderToolCard, tool output collapse
├── approval.go       # approval banner state + rendering
├── statusbar.go      # status bar rendering (model name, spinner, elapsed)
├── *_test.go         # tests for each module
```

## Design Decisions

### D1: Bubble Tea v1 → v2 In-Place Upgrade

直接替换 go.mod 依赖，一次性迁移所有 import path。

**Migration checklist:**
- `github.com/charmbracelet/bubbletea` → `charm.land/bubbletea/v2`
- `github.com/charmbracelet/lipgloss` → `charm.land/lipgloss/v2`
- 新增 `charm.land/bubbles/v2`
- `Model.View() string` → `Model.View() tea.View`（设置 `v.AltScreen = true`）
- `tea.WithAltScreen()` option 移除（改在 View 中声明）
- `tea.KeyMsg.Type` → `tea.KeyMsg` 的 key matching 方式变化
- `tea.WindowSizeMsg` 字段不变

### D2: Bubbles Components

**textarea** 配置:
- `DynamicHeight = true`, max 5 lines
- `KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "shift+enter"))`
- `KeyMap.Submit` → custom: intercept Enter in Model.Update
- `CharLimit = 16384`
- `Prompt = ""`（自行渲染 `> ` 前缀）
- `Focus()` on startup

**viewport** 配置:
- 绑定 `tea.MouseMsg` 滚轮事件
- `MouseWheelDelta = 3`
- 自带 PgUp/PgDn/Home/End 键绑定
- tail-follow: 当 `viewport.AtBottom()` 时，新内容自动 `GotoBottom()`

**spinner** 配置:
- `spinner.Dot` 样式
- tick 频率默认即可
- 在 `busy` 状态时 Init 中启动 `spinner.Tick`

### D3: Markdown Rendering — Glamour + Interface

```go
type MarkdownRenderer interface {
    Render(markdown string, width int) string
}

type glamourRenderer struct{}

func (g glamourRenderer) Render(md string, width int) string {
    r, _ := glamour.NewTermRenderer(
        glamour.WithAutoStyle(),
        glamour.WithWordWrap(width),
    )
    out, _ := r.Render(md)
    return out
}
```

glamour 内部使用 goldmark + chroma，开箱支持：标题、列表、代码块（语法高亮）、表格、引用、粗体/斜体、内联代码。

后续如需更精细控制（如 Reasonix 风格的 dim gutter、accent color fenced blocks），替换为自定义 goldmark walker 实现，调用方不变。

### D4: Streaming Markdown — Paragraph-Boundary Flush

```go
func flushableMarkdownPrefix(buf string) (renderable, remaining string) {
    // 从 buf 末尾向前找最后一个安全边界：
    // 1. 跟踪 fenced code block 状态（``` 计数）
    // 2. 在 code block 外找到连续空行（\n\n）
    // 3. 返回空行之前的内容作为 renderable
    // 4. 如果没找到安全边界，返回空（等待更多 token）
}
```

流程:
1. `StreamChunkMsg` → 追加到 `pending strings.Builder`
2. `renderable, remaining = flushableMarkdownPrefix(pending.String())`
3. 如果 renderable 非空：`rendered = mdRenderer.Render(renderable, width)` → 追加到 transcript
4. `pending.Reset(); pending.WriteString(remaining)`
5. `StreamDoneMsg` → flush 所有 remaining

### D5: Event System — Typed StreamEmitter

```go
type StreamEmitter interface {
    OnChunk(text string)
    OnToolStart(name string, args string)
    OnToolEnd(name string, result string, isError bool)
    OnApprovalRequest(name string, args map[string]any, respond func(bool))
    OnDone()
    OnError(err error)
}
```

**Agent 层变更** (`internal/agent/loop.go`):

```go
func (a *Agent) loopStepWithText(ctx context.Context, emitter StreamEmitter) (string, error) {
    // ... existing flow ...
    msg, usage, streamErr := provider.CollectWithText(ch, emitter.OnChunk)
    // ... existing flow ...
}

func (a *Agent) invokeTool(ctx context.Context, tc ToolCall, emitter StreamEmitter) string {
    if emitter != nil {
        emitter.OnToolStart(tc.Name, tc.Arguments)
        defer func() { emitter.OnToolEnd(tc.Name, result, isErr) }()
    }
    // ... existing permission check ...
    // If checker says "ask user" and emitter has OnApprovalRequest:
    //   approved := requestApproval(ctx, emitter, tc.Name, args)
    //   if !approved { return "Permission denied" }
    // ... existing tool execution ...
}
```

**审批请求-响应**:

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

**RunStreaming 签名变更**:

```go
// Before:
func (a *Agent) RunStreaming(ctx context.Context, userInput string, onText func(string)) (string, error)

// After:
func (a *Agent) RunStreaming(ctx context.Context, userInput string, emitter StreamEmitter) (string, error)
```

`StreamEmitter` 定义在 `internal/tui/runner.go`（或提取到公共位置如 `internal/agent/emitter.go`）。由于只有 TUI 使用 `RunStreaming`，放在 tui 包也可接受；但如果 agent 包需要引用类型，则应放在 agent 包或独立的 `internal/streaming` 包避免循环依赖。

**推荐**：接口定义在 `internal/agent/emitter.go`（agent 包需要在 loop.go 中调用），TUI 包 import agent 包来实现该接口。

### D6: Approval Interaction

**Model 状态**:
```go
type pendingApproval struct {
    toolName string
    args     map[string]any
    respond  func(bool)
}
```

当 `m.approval != nil` 时：
- View 渲染审批横幅在 viewport 和 textarea 之间
- Update 拦截 `y`/`n` 按键，其他按键被忽略
- textarea 和 viewport 不接收事件

### D7: CJK Width Fix

替换 `wrapText` 中 `utf8.RuneLen(r)` → `runewidth.RuneWidth(r)`。同时确保 glamour 的 WordWrap 使用终端实际宽度。

### D8: Transcript Model

消息区不再是简单的 `[]Message`，而是 `[]TranscriptEntry`:

```go
type EntryKind int
const (
    EntryUserMessage EntryKind = iota
    EntryAssistantChunk    // rendered markdown
    EntryToolCard          // ● ToolName(args)
    EntryToolOutput        // collapsed output
    EntryError
)

type TranscriptEntry struct {
    Kind     EntryKind
    Content  string  // pre-rendered ANSI string
    Raw      string  // raw content (for re-render on resize)
}
```

Viewport 的 content 是所有 transcript entries 的 rendered strings 拼接。resize 时从 Raw 重新渲染。

## Testing Strategy

| 层面 | 方法 | 覆盖 |
|------|------|------|
| Markdown | `glamourRenderer.Render()` 输入已知 MD → 验证包含 ANSI codes | 代码块、表格、列表 |
| flushablePrefix | 单元测试各种边界 | 半代码块、嵌套块、空输入 |
| Tool card | `renderToolCard()` 输入固定参数 → 验证输出格式 | 长参数截断、错误标记 |
| Approval | mock emitter + ApprovalRequestMsg → 验证模态切换 + respond | y/n/Esc timeout |
| Event flow | mock Runner 发送预设事件序列 → 验证 Model 状态变化 | 完整流 |
| CJK | wrapText 中日文 + emoji → 验证行宽 | 混合宽度 |
| v2 migration | 现有测试编译通过 + 行为不变 | 基础回归 |

## Open Questions

- glamour v2 是否已发布（`charm.land/glamour/v2`）？如果只有 v1（`github.com/charmbracelet/glamour`），可能与 lipgloss/v2 有类型不兼容。需要在依赖引入时验证。
- viewport 的 tail-follow 在审批横幅出现/消失时需要重新计算高度，可能导致一帧闪烁。
