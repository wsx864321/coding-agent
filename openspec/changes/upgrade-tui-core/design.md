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

**实现**: 在 `Model` 中维护 `pending strings.Builder`，每次 `StreamChunkMsg` 追加到 pending，调用 `flushablePrefix()` 提取可安全渲染的前缀，渲染后写入 transcript，剩余内容保留在 pending。

### D5: 工具调用事件传递

**选择**: 扩展 `StreamEmitter` 接口，增加工具事件方法

**理由**: 当前 `StreamEmitter` 仅有文本/完成/错误三种事件。要在 TUI 展示工具调用，需要 agent 层将工具调用/结果事件传递给 TUI。

**新接口**:
```go
type StreamEmitter interface {
    OnChunk(text string)
    OnToolStart(name string, args string)
    OnToolEnd(name string, result string, err error)
    OnApprovalRequest(name string, args map[string]any, respond func(bool))
    OnDone()
    OnError(err error)
}
```

**影响**: 需要修改 `internal/agent/loop.go` 的 `loopStepWithText` 和 `invokeTool`，在工具执行前后调用 emitter。`cmd/cli/tui_runner.go` 需要传递扩展的 emitter。`chanEmitter` 需要增加对应的 channel message 类型。

**向后兼容**: `chat` 命令的 `Run()` 不受影响，只有 `RunStreaming` 路径变化。

### D6: 审批交互

**选择**: TUI 内嵌交互式审批横幅

**理由**: 当前 TUI 因为无法安全做 stdin 交互而全自动拒绝权限请求。Bubble Tea 本身管理 stdin，可以安全地在 Update 中处理审批按键。

**实现**: 当 `OnApprovalRequest` 触发时，Model 进入 `pendingApproval` 模态状态。View 渲染审批横幅（显示工具名、参数摘要、`[y]es [n]o` 选项）。用户按键后调用 `respond(bool)` 回调解锁 agent 等待。

### D7: CJK 显示宽度修复

**选择**: 引入 `github.com/mattn/go-runewidth` 替代 `utf8.RuneLen`

**理由**: `utf8.RuneLen` 返回 UTF-8 编码字节数（1-4），而终端渲染需要显示宽度（CJK 字符=2，ASCII=1）。`go-runewidth` 是 Go 生态标准的显示宽度库。

**影响**: `wrapText` 函数重写，使用 `runewidth.RuneWidth()` 计算。lipgloss 和 viewport 已内部依赖 runewidth，不会增加新间接依赖。

### D8: 布局架构

**选择**: 三区布局（transcript + 审批/状态 + input）

**布局**:
```
┌─── Viewport (transcript) ────────────┐
│ 消息流 + 工具卡片 + Markdown 输出    │
│                               scrollbar│
└──────────────────────────────────────┘
┌─── 审批横幅（可选，仅需要时显示）─────┐
└──────────────────────────────────────┘
┌─── Spinner + 状态栏 ─────────────────┐
│ ● thinking (3s) │ model │ ctx-used   │
└──────────────────────────────────────┘
┌─── Textarea (输入区) ────────────────┐
│ > 多行输入...                        │
└──────────────────────────────────────┘
┌─── Help ─────────────────────────────┐
│ Shift+Enter 换行 · Enter 发送 ...    │
└──────────────────────────────────────┘
```

Viewport 高度 = 终端高度 - textarea 高度 - 状态栏 - help - 可选审批横幅。Textarea 动态高度（1-5 行）。

## Risks / Trade-offs

**[Risk] Bubble Tea v2 API 变化导致大量迁移工作** → 测试覆盖率已足够（15+ 测试），迁移后逐个修复。v2 的核心 MVU 模式不变，主要是类型签名变化。

**[Risk] goldmark 自定义 renderer 复杂度** → 首版 scope 有限（不含数学/图片），参考 Reasonix 的 `md.go` 实现模式。可分步迭代：先做代码块+标题，再做表格+列表。

**[Risk] 工具事件传递改动触及 agent 核心代码** → `loopStepWithText` 和 `invokeTool` 的改动是增量的（在现有执行点插入 emitter 调用），不改变控制流。`RunStreaming` 签名需要改变但调用者有限（仅 `tui_runner.go`）。

**[Risk] 审批交互的并发安全** → 审批请求会阻塞 agent goroutine（等待 `respond` 回调），而 Bubble Tea 的 Update 在主线程处理按键后通过 channel 回调。需要确保 respond 只调用一次（使用 `sync.Once`）。

**[Trade-off] 段落边界刷新会引入轻微延迟** → 用户可能感知到输出不是逐字符出现而是逐段出现。但这是 Reasonix 验证过的策略，且避免了代码块闪烁问题。

**[Trade-off] 不升级 chat 命令** → TUI 和 chat 会有功能差异期，但本次专注核心体验优先。
