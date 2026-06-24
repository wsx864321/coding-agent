## Context

当前 agent 运行时输出散布在三种机制中：

1. **`StreamEmitter`**（`internal/agent/emitter.go`）：6 个 callback 方法，通过 context 注入，仅 TUI 模式实现了 `chanEmitter`
2. **`log.Printf`**（9 处）：hooks 加载/执行错误 + todo guard 调试，直写 stderr
3. **`fmt.Print*`**（~46 处）：chat REPL 的所有用户交互输出，直写 stdout/stderr

Reasonix 已验证 typed event + Sink 多前端架构的可行性，其 `event.Sink` 接口（单方法 `Emit(Event)`）统一承载 18 种事件。本项目引入精简版。

## Goals / Non-Goals

**Goals:**
- 定义 `internal/event` 包，提供 `Event`/`Kind`/`Level`/`Sink` 类型
- 用 `Sink.Emit` 完全取代 `StreamEmitter` 的 6 个 callback
- 新增 `Notice` 事件覆盖 hook warn/error、todo guard 等带外消息
- 提供 `TextSink`（ANSI 终端）和 TUI channel Sink 两个实现
- Chat REPL 和 once 模式通过 `TextSink` 输出
- hooks 包零 `log.Printf`，加载静默降级，执行 warn/error 走 `notify` → `Notice`

**Non-Goals:**
- 不引入 Controller 层
- 不引入 slog
- 不新增 Usage/Compaction/Phase/Retry/Steer 等高级 Event Kind
- 不改 permission.Asker 机制
- 不改工具内部逻辑

## Decisions

### D1: Event 类型定义

```go
// internal/event/event.go
type Kind int
const (
    Text             Kind = iota // 文本增量（流式 chunk）
    ToolDispatch                 // 工具即将执行
    ToolResult                   // 工具完成
    ApprovalRequest              // 等待用户审批
    TurnDone                     // 一轮结束
    Notice                       // 带外消息（警告、hook block 等）
)

type Level int
const (
    LevelInfo Level = iota
    LevelWarn
)

type Event struct {
    Kind  Kind
    Level Level  // 仅 Notice 使用

    Text string // Text/Notice: 内容

    // ToolDispatch / ToolResult
    ToolName   string
    ToolArgs   string
    ToolOutput string
    ToolIsErr  bool

    // ApprovalRequest
    ApprovalRespond func(bool)

    // TurnDone
    Err error
}
```

**理由**：flat struct 比 interface 联合体更简单，与 Reasonix 一致。6 种 Kind 覆盖现有 StreamEmitter 全部 + Notice。

### D2: Sink 接口

```go
type Sink interface {
    Emit(Event)
}

type FuncSink func(Event)
func (f FuncSink) Emit(e Event) { f(e) }

var Discard Sink = FuncSink(func(Event) {})
```

**理由**：单方法接口，扩展 Kind 不需改 Sink 签名。`Discard` 供测试和无 UI 场景。

### D3: TextSink（chat REPL + once）

```go
// internal/event/textsink.go
type TextSink struct {
    Out io.Writer // stdout
    Err io.Writer // stderr（Notice warn）
}
```

渲染规则：
- `Text` → 直接写 Out（无换行，流式 chunk）
- `ToolDispatch` → `Err` 写 `"  ⚡ <name>(<args>)"`
- `ToolResult` → `Err` 写 `"  ✓ <name>"` 或 `"  ✗ <name>: <err>"`
- `Notice` → `Err` 写 `"  ! <text>"`（warn）或 `"  · <text>"`（info）
- `ApprovalRequest` → 委托给注入的 Asker（TextSink 不直接处理审批 UI）
- `TurnDone` → 无输出（或写换行分隔）

### D4: Agent 集成

Agent 构造时注入 `event.Sink`（替代 `WithEmitter`）：

```go
// option.go
func WithSink(s event.Sink) Option

// agent.go
type Agent struct {
    sink event.Sink // 替代原 emitter context 注入
}
```

`loop.go` 中所有 `emitter.OnXxx()` 调用改为 `a.sink.Emit(event.Event{...})`。

### D5: Hook notify 注入

`hooks.NewRunner` 增加第 4 个参数 `notify func(string)`：

```go
func NewRunner(hooks []ResolvedHook, cwd string, spawner Spawner, notify func(string)) *Runner
```

CLI 装配时将 `notify` 桥接到 Sink：

```go
notify := func(msg string) {
    sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn, Text: msg})
}
runner := hooks.NewRunner(resolved, workdir, hooks.DefaultSpawner, notify)
```

`load.go` 中的 `log.Printf` 全部移除（静默降级）。`run.go` 中 spawn 失败/warn 通过 `FormatOutcome` → `notify`。

### D6: TUI Sink 实现

现有 `chanEmitter` 改为实现 `event.Sink`：

```go
type tuiSink struct {
    ch chan<- event.Event
}
func (s *tuiSink) Emit(e event.Event) { s.ch <- e }
```

TUI model 的 `Update` 根据 `Event.Kind` 分发渲染，替代原有的多个 message type。

### D7: Chat REPL 迁移

`cmd/cli/chat.go` 中 agent 运行时输出（调用 `agent.Run` / `agent.RunStream` 前后）改为通过 `TextSink`。Slash 命令反馈（`/tools`、`/history` 等）保持 `fmt.Print*` 直接输出（REPL 交互层，非 agent 事件）。

## Risks / Trade-offs

- **[BREAKING]** 删除 `StreamEmitter` 是破坏性变更，所有消费者必须同步迁移 → 本次一次性完成，项目无外部消费者
- **[复杂度]** chat REPL 的 ~46 处 `fmt` 迁移工作量较大 → slash 命令保持 `fmt` 减小范围（仅 agent 运行时输出迁移）
- **[性能]** flat struct Event 每次创建可能有额外分配 → 事件量不大（非热路径），可忽略
- **[扩展]** 6 种 Kind 后续可能不够 → Kind 是 int 枚举，新增不影响现有 Sink 实现
