---
comet_change: unified-event-sink
role: technical-design
canonical_spec: openspec
archived-with: 2026-06-24-unified-event-sink
status: final
---

# Unified Event Sink — 统一事件流体系

## 背景

当前 agent 运行时输出散布在三种机制中：

| 机制 | 调用点 | 可见性 |
|------|--------|--------|
| `StreamEmitter` (6 callback) | 仅 TUI 模式 `RunStreaming()` | TUI 可见 |
| `log.Printf` (9 处) | hooks load/run + todo guard | stderr，TUI 不可见 |
| `fmt.Print*` (~46 处) | chat REPL 直写控制台 | 仅 chat 可见 |

三条路径互不相通：hook warn 在 TUI 不可见，chat REPL 无工具进度，新增事件类型需改 interface + 所有实现。

## 目标

- 引入 `internal/event` 包，定义 `Event`/`Kind`/`Sink` 统一事件体系
- Agent struct 直接持有 `event.Sink`，所有运行时产出通过 `Emit` 发射
- 统一为单入口 `Run()`，删除 `RunStreaming()` 和 `StreamEmitter`
- 新增 `Notice` 事件覆盖 hook warn/error、todo guard 等带外消息
- 三种前端 Sink 实现：TextSink（chat/once）、chanSink（TUI）、Discard（subagent/测试）
- hooks 包零 `log.Printf`，通过 `notify` 回调解耦

## 非目标

- 不引入 Controller 层
- 不引入 slog
- 不新增 Usage/Compaction/Phase 等高级 Kind（后续按需扩展）
- 不改 permission.Asker 机制（审批交互路径不变）
- 不改工具内部逻辑

## 架构

```
┌─────────────┐     ┌──────────┐     ┌──────────────┐
│ Agent.Run() │────▶│ Sink     │────▶│ TextSink     │ chat/once → stdout/stderr
│  loopStep   │     │ .Emit()  │     │ chanSink     │ TUI → chan → Bubble Tea
│  invokeTool │     │          │     │ Discard      │ subagent/test
└─────────────┘     └──────────┘     └──────────────┘
                         ▲
                         │ notify bridge
                    ┌────┴────┐
                    │ hooks   │
                    │ Runner  │
                    └─────────┘
```

## D1: Event 类型定义

```go
// internal/event/event.go
package event

type Kind int

const (
    Text            Kind = iota // 流式文本 chunk
    ToolDispatch                // 工具即将执行
    ToolResult                  // 工具完成
    ApprovalRequest             // 等待用户审批
    TurnDone                    // 一轮结束
    Notice                      // 带外消息（hook warn、todo guard 等）
)

type Level int

const (
    LevelInfo Level = iota
    LevelWarn
)

type Event struct {
    Kind  Kind
    Level Level // 仅 Notice 使用

    Text string // Text: chunk 内容; Notice: 消息文本

    // ToolDispatch / ToolResult
    ToolName   string
    ToolArgs   string
    ToolOutput string
    ToolIsErr  bool

    // ApprovalRequest
    ApprovalName    string
    ApprovalArgs    map[string]any
    ApprovalRespond func(bool)

    // TurnDone
    Err error
}
```

选用 flat struct（与 Reasonix 一致）而非 interface 联合体。每个 Event 只使用与其 Kind 对应的字段子集，其余为零值。后续扩展只需新增 Kind 常量和字段。

## D2: Sink 接口

```go
// internal/event/sink.go
package event

type Sink interface {
    Emit(Event)
}

type FuncSink func(Event)

func (f FuncSink) Emit(e Event) { f(e) }

var Discard Sink = FuncSink(func(Event) {})
```

单方法接口，新增 Kind 不改签名。`Discard` 供 subagent 和测试使用。

## D3: TextSink

```go
// internal/event/textsink.go
type TextSink struct {
    Out io.Writer // stdout — Text chunk
    Err io.Writer // stderr — 工具摘要、Notice
}

func (s *TextSink) Emit(e Event) {
    switch e.Kind {
    case Text:
        io.WriteString(s.Out, e.Text)
    case ToolDispatch:
        fmt.Fprintf(s.Err, "  ⚡ %s\n", e.ToolName)
    case ToolResult:
        if e.ToolIsErr {
            fmt.Fprintf(s.Err, "  ✗ %s\n", e.ToolName)
        } else {
            fmt.Fprintf(s.Err, "  ✓ %s\n", e.ToolName)
        }
    case Notice:
        prefix := "·"
        if e.Level == LevelWarn { prefix = "⚠" }
        fmt.Fprintf(s.Err, "  %s %s\n", prefix, e.Text)
    case ApprovalRequest:
        // chat 模式审批走 StdinAsker，TextSink 不处理
    case TurnDone:
        // 无输出
    }
}
```

chat REPL 和 once 模式使用 `TextSink{Out: os.Stdout, Err: os.Stderr}`。相比现有方案，chat REPL 会获得实时工具进度（UX 提升）。

## D4: Agent 集成

### Agent struct

```go
type Agent struct {
    // ...existing fields...
    sink event.Sink  // 替代原 StreamEmitter context 注入
}
```

### Option

```go
func WithSink(s event.Sink) Option  // 新增
// 删除: WithEmitter 相关（不存在独立定义，但 emitter.go 的 context 机制全删）
```

`NewAgent` 末尾：`if a.sink == nil { a.sink = event.Discard }`

### 统一为单入口 Run()

删除 `RunStreaming()`。现有 `Run()` 签名不变，内部 loop 直接用 `a.sink`：

```go
func (a *Agent) loopStep(ctx context.Context) (string, error) {
    onText := func(s string) {
        a.sink.Emit(Event{Kind: event.Text, Text: s})
    }
    // ...stream + collect...
}
```

`invokeTool` 不再接收 emitter 参数：

```go
func (a *Agent) invokeTool(ctx context.Context, tc provider.ToolCall) string {
    a.sink.Emit(Event{Kind: event.ToolDispatch, ToolName: name, ToolArgs: tc.Arguments})
    // ...execute...
    a.sink.Emit(Event{Kind: event.ToolResult, ToolName: name, ToolOutput: result, ToolIsErr: isErr})
}
```

`executeBatch` 不再 `EmitterFromContext`。

### emitter.go 删除

删除 `StreamEmitter` 接口、`WithEmitter`、`EmitterFromContext`、`emitterContextKey`。

### todo_guard.go

```go
// 原: log.Printf("[agent] 终答守卫: ...")
// 改:
a.sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelInfo, Text: "..."})
```

### Subagent

Subagent 默认使用 `Discard`（不传 WithSink），其输出通过 tool result 回传父 agent。

## D5: Hook 层

### Runner 签名

```go
func NewRunner(hooks []ResolvedHook, cwd string, spawner Spawner, notify func(string)) *Runner
```

`notify` 为 nil 时 Runner 内部替换为 `func(string) {}`。Hook 包不依赖 `event` 包。

### run.go 迁移

| 原 log.Printf | 改为 |
|---|---|
| `marshal payload` 失败 | `r.notify(msg)` |
| `spawn failed` | `r.notify(msg)` |
| `invalid match regex` | `r.notify(msg)` |

### load.go 迁移

4 处 `log.Printf` 全部删除，静默降级：
- 用户目录获取失败 → 静默跳过全局 hook
- 文件读取失败 → 静默跳过
- JSON 解析失败 → 静默返回 nil
- 正则编译失败 → 静默跳过该 hook

## D6: TUI 层

### chanSink

```go
// internal/tui/sink.go（或 runner.go 内）
type chanSink struct { ch chan<- event.Event }
func (s chanSink) Emit(e event.Event) { s.ch <- e }
```

### Runner 接口变更

```go
type Runner interface {
    RunTurn(ctx context.Context, prompt string) error
}
```

移除 `emit StreamEmitter` 参数。TUI model 在 `submit()` 时：

```go
ch := make(chan event.Event, 16)
go func() {
    defer close(ch)
    // Agent 已在构造时注入 chanSink
    _ = runner.RunTurn(ctx, text)
}()
```

但这里有一个问题：Agent 的 Sink 是在构造时固定的（方案 A），而 TUI 每轮 submit 要创建新 channel。

**解决方案**：Agent 构造时注入一个 `*TuiSink`，它内部持有可替换的 channel。每轮 submit 时更新 channel：

```go
type TuiSink struct {
    mu sync.Mutex
    ch chan<- event.Event
}

func (s *TuiSink) Emit(e event.Event) {
    s.mu.Lock()
    ch := s.ch
    s.mu.Unlock()
    if ch != nil { ch <- e }
}

func (s *TuiSink) SetChan(ch chan<- event.Event) {
    s.mu.Lock()
    s.ch = ch
    s.mu.Unlock()
}
```

### 消息类型

删除 `StreamChunkMsg`、`ToolStartMsg`、`ToolEndMsg`、`ApprovalRequestMsg`、`StreamDoneMsg`、`StreamErrorMsg`。

TUI model Update 直接 switch `event.Event.Kind`。保留 `streamClosedMsg`（channel 关闭信号）。

### tui_runner.go

```go
func (r agentRunner) RunTurn(ctx context.Context, prompt string) error {
    _, err := r.agent.Run(ctx, prompt)
    return err
}
```

## D7: CLI 装配

### chat_setup.go

```go
func setupAgentWithAsker(cmd *cobra.Command, asker permission.Asker) (*chatSetup, error) {
    // ...existing setup...
    sink := &event.TextSink{Out: os.Stdout, Err: os.Stderr}
    notify := func(msg string) {
        sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn, Text: msg})
    }
    runner := hooks.NewRunner(loaded, workdir, hooks.DefaultSpawner, notify)
    // ...
    opts = append(opts, agent.WithSink(sink))
}
```

### setupTuiAgent

```go
func setupTuiAgent(cmd *cobra.Command) (*chatSetup, error) {
    tuiSink := &tui.TuiSink{}
    notify := func(msg string) {
        tuiSink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn, Text: msg})
    }
    // ...
    opts = append(opts, agent.WithSink(tuiSink))
}
```

### once.go

```go
sink := &event.TextSink{Out: os.Stdout, Err: os.Stderr}
// quiet 模式: 只输出最终 Text
if onceQuiet {
    sink = &event.TextSink{Out: os.Stdout, Err: io.Discard}
}
```

### chat.go

`runOneTurn` 不再 `fmt.Println(out)` 最终回答——因为 TextSink 已在流式过程中输出了 Text chunk。`Run()` 返回的 final string 仅用于确认完成。

**但需注意**：当前 `Run()` 返回完整的 `msg.Content`。如果 TextSink 已经在流式过程中通过 `onText` 输出了所有 chunk，那最终不需要再 print 一次。

```go
func runOneTurn(ctx context.Context, a *agent.Agent, prompt string) error {
    _, err := a.Run(ctx, prompt)
    if err != nil {
        if errors.Is(err, agent.ErrMaxTurnsExceeded) {
            return fmt.Errorf("超过最大轮数: %w", err)
        }
        return err
    }
    fmt.Println() // 最终换行分隔
    return nil
}
```

## 测试策略

| 层 | 测试内容 | 方法 |
|---|---|---|
| `internal/event` | TextSink 6 种 Kind 输出格式 | bytes.Buffer 捕获 |
| `internal/event` | FuncSink/Discard | 回调计数 / 不 panic |
| `internal/agent` | Agent.Run + mock Sink | FuncSink 收集事件序列 |
| `internal/agent` | todo_guard Notice 事件 | mock Sink 验证 |
| `internal/hooks` | Runner notify 回调 | mock notify + mock spawner |
| `internal/hooks` | load 静默降级 | 无 log.Printf 输出 |
| 集成 | `go build ./...` + `go test ./...` | CI |
| 合规 | `grep -r log.Printf internal/hooks/ internal/agent/` | 零匹配 |

## 迁移检查清单

- [ ] 全量代码零 `StreamEmitter` 引用
- [ ] `internal/hooks/` 和 `internal/agent/` 零 `log.Printf`
- [ ] `internal/agent/emitter.go` 已删除
- [ ] `RunStreaming` 方法已删除
- [ ] TUI 旧消息类型已删除
- [ ] 三种模式（chat/tui/once）均正常工作
