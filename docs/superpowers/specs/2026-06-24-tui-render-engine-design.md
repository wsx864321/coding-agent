---
comet_change: tui-render-engine
role: technical-design
canonical_spec: openspec
---

# TUI 渲染引擎升级 — 技术设计

## 1. 事件系统扩展

### 1.1 新增事件类型

在 `internal/event/event.go` 中扁平扩展 `Event` 结构体：

```go
type Event struct {
    Kind  Kind
    Level Level

    // 现有字段（不变）
    Text       string
    ToolName   string
    ToolArgs   string
    ToolOutput string
    ToolIsErr  bool
    ApprovalName    string
    ApprovalArgs    map[string]any
    ApprovalRespond func(bool)
    Err error

    // 新增字段
    ReasoningChunk string // 增量推理文本（ReasoningText 事件）
    ToolCallID     string // 工具调用标识（ToolProgress 事件）
    ToolChunk      string // 增量输出文本（ToolProgress 事件）
}
```

新增 Kind 常量：

```go
const (
    Text Kind = iota
    ToolDispatch
    ToolResult
    ApprovalRequest
    TurnDone
    Notice
    ReasoningText   // 新增
    ToolProgress    // 新增
)
```

### 1.2 向后兼容

- 现有事件类型不访问新字段，零值无副作用
- `TuiSink.Emit()` 无需修改（channel 类型不变）
- 现有测试无需更新

## 2. 推理文本渲染

### 2.1 Transcript Entry 类型

新增 `EntryReasoning`：

```go
const (
    EntryUserMessage EntryKind = iota
    EntryAssistantChunk
    EntryToolCard
    EntryToolOutput
    EntryError
    EntryReasoning   // 新增
)
```

### 2.2 Model 状态字段

```go
reasoning       *strings.Builder // 累积推理文本
reasoningLineIdx int             // 摘要行在 transcript 中的索引，-1 表示无
showReasoning   bool             // Ctrl+O 切换展开/折叠
thinkStart      time.Time        // 推理开始时间
```

### 2.3 渲染流程

1. **ReasoningText 事件** → 追加到 `reasoning` builder，创建/更新摘要行（spinner + "thinking…"）
2. **首个 Text 事件**（推理结束）→ 摘要行更新为 "▎ thought for Ns"，推理文本折叠
3. **Ctrl+O** → 切换 `showReasoning`，原地重写 transcript 块
4. **折叠状态**：仅显示摘要行（dim 样式）
5. **展开状态**：摘要行 + 推理文本（dim 样式），两者之间有视觉边界

### 2.4 与流式 Markdown 的交互

- 推理期间 `pending` builder 不累积（推理文本不进 Markdown 渲染器）
- 推理结束后才开始累积 `pending`（回答文本）
- 推理块和回答块在 transcript 中独立存储

## 3. 工具流式输出

### 3.1 Model 状态字段

```go
toolStreamIdx   int      // 流式输出块在 transcript 中的索引，-1 表示无
toolStreamID    string   // 当前流式输出的 toolCallID
toolTail        []string // 尾部保留行（最多 20 行）
toolPartial     string   // 当前未完成行
toolLineCount   int      // 总行数计数
toolStreamStart time.Time // 流式开始时间
```

### 3.2 尾部截断算法

- 保留最后 20 行 + 当前未完成行
- 每收到 `\n` 时 push 到 `toolTail`，超过 20 行时 shift 最旧行
- 不在行中间截断（保留完整行）

### 3.3 渲染流程

1. **ToolDispatch** → 创建工具卡片 entry，记录 `toolStreamIdx`
2. **ToolProgress** → 追加 chunk，原地重写流式输出块："⎿ working · Ns" + 尾部行 + "⎿ N lines"
3. **ToolResult** → 流式输出转为折叠摘要（前 8 行 + "⎿ N lines, collapsed"）

### 3.4 Drain Loop

```go
case agentEventMsg:
    m.ingestEvent(e)
    for drained := 0; drained < maxEventDrain; drained++ {
        select {
        case e2 := <-m.eventCh:
            m.ingestEvent(e2)
        default:
            break
        }
    }
    m.syncViewportContent()
```

- `maxEventDrain = 512`，防止无限循环
- 每帧最多重渲染一次 viewport

## 4. Shell 输出折叠/展开

### 4.1 Model 状态字段

```go
shellOutputs  map[string]string // toolCallID → 完整输出
shellExpanded map[string]bool   // toolCallID → 是否展开
```

### 4.2 渲染流程

1. **ToolResult (bash)** → 完整输出存入 `shellOutputs[toolCallID]`，默认折叠
2. **折叠渲染**：前 8 行 + "⎿ N lines, collapsed"
3. **短输出（≤8 行）**：直接完整展示，不存储

### 4.3 Ctrl+B 定位与切换

1. 从 `transcript[len-1]` 反向扫描，找第一个关联 toolCallID 在 `shellOutputs` 中的 `EntryToolOutput`
2. 切换 `shellExpanded[toolCallID]`
3. 原地重写 transcript entry
4. 保持 viewport 滚动位置

### 4.4 内存安全

- Shell 输出超过 1MB 时截断（保留最后 1MB），标注 "output truncated"
- 非 bash 工具不存储完整输出

## 5. Markdown 渲染升级

### 5.1 Chroma 语法高亮

```go
func NewGlamourRenderer() MarkdownRenderer {
    return glamourRenderer{}
}

func (g glamourRenderer) Render(md string, width int) string {
    r, err := glamour.NewTermRenderer(
        glamour.WithStylePath("dark"),
        glamour.WithWordWrap(width),
        glamour.WithChromaStyle(&chromaStyle), // 新增
    )
    // ...
}
```

### 5.2 Diff 视图着色

- 检测条件：代码块语言标记为 `diff`
- 在 glamour 渲染后叠加 lipgloss 着色：
  - `+` 开头 → 绿色 foreground
  - `-` 开头 → 红色 foreground
  - `@@` 开头 → 青色 foreground
- 非 diff 代码块行为不变

## 6. 文本选择与复制

### 6.1 Selection 结构体

```go
type selection struct {
    startLine int  // wrappedLines 索引
    startCol  int
    endLine   int
    endCol    int
    active    bool
}
```

### 6.2 鼠标交互

1. 左键按下 → 记录起始位置，`active = true`
2. 拖动 → 更新结束位置，实时高亮
3. 释放 → 保持选择，等待复制
4. 单击 → 取消选择

### 6.3 复制（Ctrl+C）

1. 检测 `selection.active && !selection.empty()`
2. 从 `wrappedLines` 提取文本
3. 调用 `clipboard.WriteAll(text)`
4. 显示 "Copied N characters"
5. 无选择时保持原有 Ctrl+C 行为

### 6.4 高亮渲染

- 选中行叠加 `lipgloss.Reverse(true)` 反色样式
- 首尾行仅高亮选中列范围

## 7. 测试策略

| 类型 | 覆盖 |
|------|------|
| 单元测试 | Event 新字段、flushableMarkdownPrefix 边界、toolTail 截断、selection 坐标映射、diff 检测 |
| 集成测试 | ReasoningText→EntryReasoning、ToolProgress→流式块、Ctrl+B/Ctrl+O/Ctrl+C |
| 回归测试 | `go test ./internal/tui/...` 全量通过 |

## 8. 文件变更清单

| 文件 | 变更类型 |
|------|---------|
| `internal/event/event.go` | 修改：新增 Kind 常量 + Event 字段 |
| `internal/tui/message.go` | 修改：新增 EntryReasoning |
| `internal/tui/model.go` | 修改：新增状态字段 + 事件处理 + 快捷键 |
| `internal/tui/transcript.go` | 修改：新增推理块/流式块渲染 |
| `internal/tui/toolcard.go` | 修改：新增流式输出渲染 + Shell 折叠 |
| `internal/tui/markdown.go` | 修改：chroma 配置 + diff 着色 |
| `internal/tui/view.go` | 修改：文本选择高亮 |
| `internal/tui/selection.go` | 新增：selection 结构体 + 文本提取 |
| `go.mod` | 修改：新增 atotto/clipboard 依赖 |
