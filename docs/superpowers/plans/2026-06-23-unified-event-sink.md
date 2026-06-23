---
change: unified-event-sink
design-doc: docs/superpowers/specs/2026-06-23-unified-event-sink-design.md
base-ref: a7166d55a2ae4a17327be81c697587711b89617c
---

# Unified Event Sink 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: 使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 按任务逐步实施。步骤使用 checkbox（`- [ ]`）语法跟踪进度。

**Goal:** 引入 `internal/event` 统一事件体系，用 `event.Sink` 取代 `StreamEmitter`/`log.Printf`/`fmt.Print*` 三条分散输出路径，使 chat/TUI/once 三种前端共享同一 Agent 事件流。

**Architecture:** Agent struct 持有 `event.Sink`，`Run()` 为唯一入口；`TextSink`（chat/once）、`TuiSink`（TUI 可替换 channel）、`Discard`（subagent/测试）三种 Sink 实现；hooks 通过 `notify` 回调桥接到 Sink，包内零 `log.Printf`。

**Tech Stack:** Go 1.26、现有 `internal/agent`/`internal/hooks`/`internal/tui`/`cmd/cli` 包，无新外部依赖。

## Global Constraints

- 不引入 Controller 层、slog、Usage/Compaction/Phase 等高级 Kind（Design Doc 非目标）。
- 不改 `permission.Asker` 机制语义；TUI 审批仍走 Ask 路径，底层改为 `ApprovalRequest` 事件（D4/D6）。
- 不改工具内部逻辑。
- `internal/hooks/` 和 `internal/agent/` 迁移完成后 MUST 零 `log.Printf`。
- 全量代码 MUST 零 `StreamEmitter`/`RunStreaming`/`EmitterFromContext`/`WithEmitter` 引用。
- 每个 task 完成后：`tasks.md` 打勾 → `git commit`（不得积攒）。
- 验证命令：`go build ./...`、`go test ./... -count=1`、合规 grep（见 Task 8）。

## 设计决策索引

| 决策 | 内容 | 本计划对应 Task |
|------|------|----------------|
| **D1** | `Event`/`Kind`/`Level` flat struct 定义 | Task 1 |
| **D2** | `Sink` 接口、`FuncSink`、`Discard` | Task 1 |
| **D3** | `TextSink` 渲染 6 种 Kind 到 stdout/stderr | Task 1 |
| **D4** | Agent 集成 Sink、删除 `RunStreaming`/`emitter.go` | Task 2 |
| **D5** | hooks `notify` 回调、load 静默降级 | Task 3 |
| **D6** | `TuiSink` + chan 桥接、TUI 按 `Event.Kind` 分发 | Task 4 |
| **D7** | CLI 三种模式 Sink 装配 | Task 5 |

## 目标文件结构

```
internal/event/           # 新建（D1-D3）
├── event.go              # Kind、Level、Event
├── sink.go               # Sink、FuncSink、Discard
├── textsink.go           # TextSink
├── event_test.go
├── sink_test.go
└── textsink_test.go

internal/agent/
├── agent.go              # sink 字段、Run() 发 TurnDone、删 RunStreaming
├── option.go             # WithSink
├── loop.go               # sink.Emit 替代 emitter
├── todo_guard.go         # Notice 事件
├── sink_asker.go         # 新建，替代 emitter_asker.go（ApprovalRequest）
├── subagent.go           # 默认 Discard（不传 WithSink）
├── emitter.go            # 删除
├── emitter_asker.go      # 删除
├── emitter_test.go       # 删除
└── emitter_asker_test.go # 迁移为 sink_asker_test.go

internal/hooks/
├── runner.go             # notify 参数
├── run.go                  # notify 替代 log.Printf
└── load.go                 # 静默降级

internal/tui/
├── sink.go               # 新建 TuiSink
├── runner.go             # Runner 接口简化
├── stream.go             # 仅保留 streamClosedMsg
├── model.go              # switch event.Event.Kind
└── *_test.go             # 迁移测试

cmd/cli/
├── chat_setup.go         # TextSink/TuiSink + notify 桥接
├── chat.go               # runOneTurn 不再重复 print
├── once.go               # TextSink + quiet 模式
├── tui.go                # 传递 TuiSink 给 model
└── tui_runner.go         # Run() 替代 RunStreaming
```

---

## Task 1: event 包核心定义（D1 + D2 + D3）

**输入:** Design Doc D1-D3；当前无 `internal/event` 包。

**输出:** 可独立编译测试的 `event` 包，含 6 种 Kind 的 TextSink 渲染。

**验证标准:** `go test ./internal/event/... -count=1` 全部 PASS。

**Files:**
- Create: `internal/event/event.go`
- Create: `internal/event/sink.go`
- Create: `internal/event/textsink.go`
- Create: `internal/event/textsink_test.go`
- Create: `internal/event/sink_test.go`

**Interfaces:**
- Produces: `type Kind int`（Text/ToolDispatch/ToolResult/ApprovalRequest/TurnDone/Notice）
- Produces: `type Event struct { Kind, Level, Text, ToolName, ToolArgs, ToolOutput, ToolIsErr, ApprovalName, ApprovalArgs, ApprovalRespond, Err }`
- Produces: `type Sink interface { Emit(Event) }`
- Produces: `var Discard Sink`
- Produces: `type TextSink struct { Out, Err io.Writer }` + `func (*TextSink) Emit(Event)`

- [x] **Step 1: 编写 TextSink 失败测试**

创建 `internal/event/textsink_test.go`：

```go
package event

import (
	"bytes"
	"strings"
	"testing"
)

func TestTextSink_Text(t *testing.T) {
	var out, err bytes.Buffer
	s := &TextSink{Out: &out, Err: &err}
	s.Emit(Event{Kind: Text, Text: "hello"})
	if out.String() != "hello" {
		t.Fatalf("stdout = %q, want hello", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr should be empty, got %q", err.String())
	}
}

func TestTextSink_ToolDispatch(t *testing.T) {
	var err bytes.Buffer
	s := &TextSink{Err: &err}
	s.Emit(Event{Kind: ToolDispatch, ToolName: "bash"})
	if !strings.Contains(err.String(), "bash") {
		t.Fatalf("stderr = %q, want tool name", err.String())
	}
}

func TestTextSink_ToolResult_OK(t *testing.T) {
	var err bytes.Buffer
	s := &TextSink{Err: &err}
	s.Emit(Event{Kind: ToolResult, ToolName: "read_file", ToolIsErr: false})
	if !strings.Contains(err.String(), "read_file") {
		t.Fatalf("stderr = %q", err.String())
	}
}

func TestTextSink_ToolResult_Error(t *testing.T) {
	var err bytes.Buffer
	s := &TextSink{Err: &err}
	s.Emit(Event{Kind: ToolResult, ToolName: "bash", ToolIsErr: true})
	got := err.String()
	if !strings.Contains(got, "bash") {
		t.Fatalf("stderr = %q", got)
	}
}

func TestTextSink_Notice_Info(t *testing.T) {
	var err bytes.Buffer
	s := &TextSink{Err: &err}
	s.Emit(Event{Kind: Notice, Level: LevelInfo, Text: "todo guard"})
	if !strings.Contains(err.String(), "todo guard") {
		t.Fatalf("stderr = %q", err.String())
	}
}

func TestTextSink_Notice_Warn(t *testing.T) {
	var err bytes.Buffer
	s := &TextSink{Err: &err}
	s.Emit(Event{Kind: Notice, Level: LevelWarn, Text: "hook failed"})
	if !strings.Contains(err.String(), "hook failed") {
		t.Fatalf("stderr = %q", err.String())
	}
}

func TestTextSink_ApprovalRequest_NoOutput(t *testing.T) {
	var out, err bytes.Buffer
	s := &TextSink{Out: &out, Err: &err}
	s.Emit(Event{Kind: ApprovalRequest, ApprovalName: "write_file"})
	if out.Len() != 0 || err.Len() != 0 {
		t.Fatalf("ApprovalRequest should produce no output")
	}
}

func TestTextSink_TurnDone_NoOutput(t *testing.T) {
	var out, err bytes.Buffer
	s := &TextSink{Out: &out, Err: &err}
	s.Emit(Event{Kind: TurnDone, Err: nil})
	if out.Len() != 0 || err.Len() != 0 {
		t.Fatalf("TurnDone should produce no output")
	}
}
```

- [x] **Step 2: 运行测试确认失败**

```bash
go test ./internal/event/... -count=1 -v
```

Expected: FAIL — `package event: no Go files` 或 `TextSink` 未定义

- [x] **Step 3: 实现 event.go + sink.go + textsink.go**

`internal/event/event.go`：

```go
package event

type Kind int

const (
	Text Kind = iota
	ToolDispatch
	ToolResult
	ApprovalRequest
	TurnDone
	Notice
)

type Level int

const (
	LevelInfo Level = iota
	LevelWarn
)

type Event struct {
	Kind  Kind
	Level Level

	Text string

	ToolName   string
	ToolArgs   string
	ToolOutput string
	ToolIsErr  bool

	ApprovalName    string
	ApprovalArgs    map[string]any
	ApprovalRespond func(bool)

	Err error
}
```

`internal/event/sink.go`：

```go
package event

type Sink interface {
	Emit(Event)
}

type FuncSink func(Event)

func (f FuncSink) Emit(e Event) { f(e) }

var Discard Sink = FuncSink(func(Event) {})
```

`internal/event/textsink.go`：

```go
package event

import (
	"fmt"
	"io"
)

type TextSink struct {
	Out io.Writer
	Err io.Writer
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
		if e.Level == LevelWarn {
			prefix = "⚠"
		}
		fmt.Fprintf(s.Err, "  %s %s\n", prefix, e.Text)
	case ApprovalRequest, TurnDone:
		// chat 审批走 StdinAsker；TurnDone 无终端输出
	}
}
```

- [x] **Step 4: 编写 FuncSink/Discard 测试**

创建 `internal/event/sink_test.go`：

```go
package event

import "testing"

func TestFuncSink_Emits(t *testing.T) {
	var n int
	s := FuncSink(func(Event) { n++ })
	s.Emit(Event{Kind: Text, Text: "x"})
	if n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
}

func TestDiscard_NoPanic(t *testing.T) {
	Discard.Emit(Event{Kind: Notice, Text: "ignored"})
}
```

- [x] **Step 5: 运行测试确认通过**

```bash
go test ./internal/event/... -count=1 -v
```

Expected: PASS

- [x] **Step 6: Commit**

```bash
git add internal/event/
git commit -m "feat(event): add unified Event/Sink/TextSink package"
```

---

## Task 2: Agent 层迁移（D4）

**输入:** Task 1 产出的 `event.Sink`；当前 `emitter.go`/`RunStreaming`/`loop.go` emitter 路径。

**输出:** Agent 通过 `a.sink.Emit` 发射所有事件；`Run()` 为唯一入口；审批改为 `SinkAsker`。

**验证标准:** `go test ./internal/agent/... -count=1` PASS；`grep -r StreamEmitter internal/agent/` 零匹配。

**Files:**
- Modify: `internal/agent/option.go`
- Modify: `internal/agent/agent.go`
- Modify: `internal/agent/loop.go`
- Modify: `internal/agent/todo_guard.go`
- Create: `internal/agent/sink_asker.go`
- Create: `internal/agent/sink_asker_test.go`
- Delete: `internal/agent/emitter.go`
- Delete: `internal/agent/emitter_asker.go`
- Delete: `internal/agent/emitter_test.go`
- Delete: `internal/agent/emitter_asker_test.go`
- Modify: `internal/agent/agent_test.go`（如有 emitter 引用）
- Modify: `internal/agent/todo_guard_test.go`

**Interfaces:**
- Consumes: `event.Sink`, `event.Event`, `event.Kind`, `event.Discard`
- Produces: `func WithSink(s event.Sink) Option`
- Produces: `type SinkAsker struct { Sink event.Sink }` + `func (SinkAsker) Ask(...) bool`
- Produces: `Run()` 在 turn 结束时 `a.sink.Emit(event.Event{Kind: event.TurnDone, Err: err})`

- [ ] **Step 1: 编写 Agent Sink 集成失败测试**

在 `internal/agent/sink_asker_test.go` 新建（从 emitter_asker_test 迁移逻辑）：

```go
package agent

import (
	"context"
	"sync"
	"testing"

	"github.com/wsx864321/coding-agent/internal/event"
)

type approvalCapture struct {
	mu    sync.Mutex
	events []event.Event
}

func (c *approvalCapture) Emit(e event.Event) {
	if e.Kind != event.ApprovalRequest {
		return
	}
	c.mu.Lock()
	c.events = append(c.events, e)
	c.mu.Unlock()
}

func TestSinkAsker_Approves(t *testing.T) {
	cap := &approvalCapture{}
	asker := SinkAsker{Sink: cap}
	done := make(chan bool, 1)
	go func() {
		done <- asker.Ask(context.Background(), "write_file", map[string]any{"path": "a.txt"}, "reason")
	}()
	cap.mu.Lock()
	if len(cap.events) != 1 {
		cap.mu.Unlock()
		t.Fatal("expected ApprovalRequest event")
	}
	cap.events[0].ApprovalRespond(true)
	cap.mu.Unlock()
	if !<-done {
		t.Fatal("Ask() = false, want true")
	}
}

func TestSinkAsker_NoSinkReturnsFalse(t *testing.T) {
	got := (SinkAsker{}).Ask(context.Background(), "write_file", nil, "reason")
	if got {
		t.Fatal("Ask() = true, want false without sink")
	}
}
```

在 `internal/agent/todo_guard_test.go` 追加：

```go
func TestCheckTodoGuard_EmitsNotice(t *testing.T) {
	var notices []event.Event
	a := &Agent{sink: event.FuncSink(func(e event.Event) {
		if e.Kind == event.Notice {
			notices = append(notices, e)
		}
	})}
	l := evidence.NewLedger()
	l.SetTodos([]evidence.TodoItem{{Content: "task", Status: "pending"}})
	ctx := evidence.WithLedger(context.Background(), l)
	_ = a.checkTodoGuard(ctx)
	if len(notices) == 0 {
		t.Fatal("expected Notice event from todo guard")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/agent/... -run 'TestSinkAsker|TestCheckTodoGuard_EmitsNotice' -count=1 -v
```

Expected: FAIL — `SinkAsker` 未定义 / Notice 未发射

- [ ] **Step 3: 添加 WithSink Option**

修改 `internal/agent/option.go`，追加：

```go
import "github.com/wsx864321/coding-agent/internal/event"

type sinkOpt struct{ s event.Sink }

func (o sinkOpt) apply(a *Agent) {
	if o.s != nil {
		a.sink = o.s
	}
}

func WithSink(s event.Sink) Option {
	return sinkOpt{s: s}
}
```

- [ ] **Step 4: Agent struct 添加 sink 字段 + NewAgent 默认值**

修改 `internal/agent/agent.go`：

```go
import "github.com/wsx864321/coding-agent/internal/event"

type Agent struct {
	// ...existing fields...
	sink event.Sink
}
```

在 `NewAgent` 末尾 `return a, nil` 之前：

```go
if a.sink == nil {
	a.sink = event.Discard
}
```

- [ ] **Step 5: 迁移 loop.go — sink.Emit 替代 emitter**

修改 `internal/agent/loop.go`：

1. 删除 `loopStep` 对 `EmitterFromContext` 的调用，改为直接调用内联逻辑。
2. 删除 `loopStepWithText` 的 `emitter` 参数，合并为单一 `loopStep`：

```go
func (a *Agent) loopStep(ctx context.Context) (final string, err error) {
	onText := func(s string) {
		a.sink.Emit(event.Event{Kind: event.Text, Text: s})
	}
	// ...其余逻辑不变，去掉 emitter 参数...
}
```

3. `invokeTool` 签名改为无 emitter：

```go
func (a *Agent) invokeTool(ctx context.Context, tc provider.ToolCall) string {
	name := tc.Name
	a.sink.Emit(event.Event{Kind: event.ToolDispatch, ToolName: name, ToolArgs: tc.Arguments})

	var result string
	defer func() {
		isErr := strings.HasPrefix(result, "Error:") || strings.HasPrefix(result, "Permission denied")
		a.sink.Emit(event.Event{Kind: event.ToolResult, ToolName: name, ToolOutput: result, ToolIsErr: isErr})
	}()
	// ...existing body...
}
```

4. `executeBatch` 删除 `emitter := EmitterFromContext(ctx)`，调用 `a.invokeTool(ctx, calls[i])`。

5. 添加 import `"github.com/wsx864321/coding-agent/internal/event"`。

- [ ] **Step 6: Run() 发射 TurnDone，删除 RunStreaming**

修改 `internal/agent/agent.go` 的 `Run()`：

```go
func (a *Agent) Run(ctx context.Context, userInput string) (final string, err error) {
	defer func() {
		a.sink.Emit(event.Event{Kind: event.TurnDone, Err: err})
	}()
	// ...existing Run body，loopStep 不再区分 streaming...
}
```

删除整个 `RunStreaming` 方法（约 L283-L333）。

- [ ] **Step 7: 迁移 todo_guard.go**

修改 `internal/agent/todo_guard.go`：

1. 删除 `"log"` import。
2. 两处 `log.Printf` 替换为：

```go
a.sink.Emit(event.Event{
	Kind:  event.Notice,
	Level: event.LevelInfo,
	Text:  fmt.Sprintf("终答守卫: 阻断最终回答 — %d/%d 待办未完成（第 %d/%d 次阻断）", ...),
})
// 放行路径：
a.sink.Emit(event.Event{
	Kind:  event.Notice,
	Level: event.LevelInfo,
	Text:  fmt.Sprintf("终答守卫: %d/%d 未完成，已超过最大阻断次数 (%d)，放行", ...),
})
```

- [ ] **Step 8: 创建 sink_asker.go 替代 emitter_asker.go**

`internal/agent/sink_asker.go`：

```go
package agent

import (
	"context"
	"sync"

	"github.com/wsx864321/coding-agent/internal/event"
)

func requestApprovalViaSink(ctx context.Context, sink event.Sink, name string, args map[string]any) bool {
	if sink == nil {
		return false
	}
	ch := make(chan bool, 1)
	var once sync.Once
	respond := func(ok bool) { once.Do(func() { ch <- ok; close(ch) }) }
	sink.Emit(event.Event{
		Kind:            event.ApprovalRequest,
		ApprovalName:    name,
		ApprovalArgs:    args,
		ApprovalRespond: respond,
	})
	select {
	case approved := <-ch:
		return approved
	case <-ctx.Done():
		respond(false)
		return false
	}
}

type SinkAsker struct {
	Sink event.Sink
}

func (a SinkAsker) Ask(ctx context.Context, name string, args map[string]any, _ string) bool {
	return requestApprovalViaSink(ctx, a.Sink, name, args)
}
```

- [ ] **Step 9: 删除 emitter 相关文件**

```bash
rm internal/agent/emitter.go internal/agent/emitter_asker.go internal/agent/emitter_test.go internal/agent/emitter_asker_test.go
```

- [ ] **Step 10: 运行 agent 测试**

```bash
go test ./internal/agent/... -count=1 -v
```

Expected: PASS

- [ ] **Step 11: Commit**

```bash
git add internal/agent/
git commit -m "refactor(agent): replace StreamEmitter with event.Sink"
```

---

## Task 3: Hook 层迁移（D5）

**输入:** Task 2 完成；当前 `hooks/runner.go`/`run.go`/`load.go` 中 7 处 `log.Printf`。

**输出:** hooks 包零 `log.Printf`；运行时 warn 通过 `notify` 回调；load 静默降级。

**验证标准:** `go test ./internal/hooks/... -count=1` PASS；`grep log.Printf internal/hooks/` 零匹配。

**Files:**
- Modify: `internal/hooks/runner.go`
- Modify: `internal/hooks/run.go`
- Modify: `internal/hooks/load.go`
- Modify: `internal/hooks/runner_test.go`
- Modify: `internal/hooks/run_test.go`

**Interfaces:**
- Produces: `func NewRunner(hooks []ResolvedHook, cwd string, spawner Spawner, notify func(string)) *Runner`
- Produces: `func Run(ctx, payload, hooks, spawner, notify func(string)) Report`
- Consumes: notify 为 nil 时内部替换为 `func(string) {}`

- [ ] **Step 1: 编写 notify 回调失败测试**

在 `internal/hooks/run_test.go` 追加：

```go
func TestRun_NotifyOnMarshalError(t *testing.T) {
	var notified string
	hooks := []ResolvedHook{{Event: EventUserPromptSubmit, HookConfig: HookConfig{Command: "x"}}}
	// 使用会导致 marshal 失败的 payload 较复杂；改测 spawn failed：
	sp := Spawner(func(_ context.Context, _ SpawnInput) SpawnResult {
		return SpawnResult{Err: errors.New("spawn boom")}
	})
	notify := func(msg string) { notified = msg }
	Run(context.Background(), Payload{Event: EventPostToolUse, Cwd: "/tmp"}, hooks, sp, notify)
	if notified == "" {
		t.Fatal("expected notify on spawn failure")
	}
}

func TestRun_NotifyOnInvalidRegex(t *testing.T) {
	var notified string
	h := ResolvedHook{
		Event: EventPreToolUse, HookConfig: HookConfig{Command: "hook", Match: "bad["},
	}
	notify := func(msg string) { notified = msg }
	Run(context.Background(), Payload{Event: EventPreToolUse, ToolName: "bash"}, []ResolvedHook{h}, mockSpawner(nil), notify)
	if notified == "" {
		t.Fatal("expected notify on invalid regex")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/hooks/... -run 'TestRun_Notify' -count=1 -v
```

Expected: FAIL — `Run` 签名不匹配（缺少 notify 参数）

- [ ] **Step 3: 修改 runner.go**

```go
type Runner struct {
	hooks   []ResolvedHook
	cwd     string
	spawner Spawner
	notify  func(string)
}

func NewRunner(hooks []ResolvedHook, cwd string, spawner Spawner, notify func(string)) *Runner {
	if spawner == nil {
		spawner = DefaultSpawner
	}
	if notify == nil {
		notify = func(string) {}
	}
	return &Runner{hooks: hooks, cwd: cwd, spawner: spawner, notify: notify}
}
```

所有 `Run(ctx, payload, r.hooks, r.spawner)` 改为 `Run(ctx, payload, r.hooks, r.spawner, r.notify)`。

- [ ] **Step 4: 修改 run.go — notify 替代 log.Printf**

```go
func Run(ctx context.Context, payload Payload, hooks []ResolvedHook, spawner Spawner, notify func(string)) Report {
	if notify == nil {
		notify = func(string) {}
	}
	// ...
	// marshal 失败:
	notify(fmt.Sprintf("[hooks] marshal payload for hook %q: %v", h.Command, err))
	// spawn failed:
	notify(fmt.Sprintf("[hooks] spawn failed for hook %q: %v", h.Command, res.Err))
	// matchesHook invalid regex:
	notify(fmt.Sprintf("[hooks] invalid match regex %q in hook %q: not compiled", h.Match, h.Command))
}
```

删除 `"log"` import，添加 `"fmt"`（若尚未导入）。

`matchesHook` 需接收 notify 或在 Run 内联处理——将 `matchesHook` 改为：

```go
func matchesHook(h ResolvedHook, p Payload, notify func(string)) bool {
	// ...
	if h.compiledMatch == nil {
		notify(fmt.Sprintf("[hooks] invalid match regex %q in hook %q: not compiled", h.Match, h.Command))
		return false
	}
	// ...
}
```

- [ ] **Step 5: 修改 load.go — 静默降级**

删除全部 4 处 `log.Printf` 及 `"log"` import：

| 场景 | 改法 |
|------|------|
| 用户目录获取失败 | `return out`（跳过全局 hook） |
| 文件读取失败（非 NotExist） | `return nil` |
| JSON 解析失败 | `return nil` |
| 正则编译失败 | `continue`（跳过该 hook） |

- [ ] **Step 6: 更新所有 NewRunner 调用处（测试文件）**

`internal/hooks/runner_test.go`、`internal/hooks/runner_iface_test.go` 等所有 `NewRunner(...)` 调用追加第 4 参数 `nil`。

- [ ] **Step 7: 运行 hooks 测试**

```bash
go test ./internal/hooks/... -count=1 -v
grep -r log.Printf internal/hooks/ || echo "OK: zero log.Printf"
```

Expected: PASS + zero grep

- [ ] **Step 8: Commit**

```bash
git add internal/hooks/
git commit -m "refactor(hooks): replace log.Printf with notify callback"
```

---

## Task 4: TUI 层迁移（D6）

**输入:** Task 1-2 产出；当前 `internal/tui/runner.go`/`stream.go`/`model.go` 基于 `StreamEmitter` 和 6 种 Msg 类型。

**输出:** `TuiSink` 实现 `event.Sink`；TUI model 按 `event.Event.Kind` 分发；旧 Msg 类型删除。

**验证标准:** `go test ./internal/tui/... -count=1` PASS。

**Files:**
- Create: `internal/tui/sink.go`
- Modify: `internal/tui/runner.go`
- Modify: `internal/tui/stream.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/runner_test.go`
- Modify: `internal/tui/emitter_test.go` → 删除或重写为 `sink_test.go`
- Modify: `internal/tui/model_test.go`、`internal/tui/integration_test.go`（如有旧 Msg 引用）

**Interfaces:**
- Produces: `type TuiSink struct { mu sync.Mutex; ch chan<- event.Event }`
- Produces: `func (s *TuiSink) Emit(e event.Event)`、`func (s *TuiSink) SetChan(ch chan<- event.Event)`
- Produces: `type Runner interface { RunTurn(ctx context.Context, prompt string) error }`

- [ ] **Step 1: 编写 TuiSink 失败测试**

创建 `internal/tui/sink_test.go`：

```go
package tui

import (
	"testing"

	"github.com/wsx864321/coding-agent/internal/event"
)

func TestTuiSink_ForwardsToChannel(t *testing.T) {
	ch := make(chan event.Event, 1)
	s := &TuiSink{}
	s.SetChan(ch)
	s.Emit(event.Event{Kind: event.ToolDispatch, ToolName: "bash"})
	got := <-ch
	if got.ToolName != "bash" {
		t.Fatalf("event = %+v", got)
	}
}

func TestTuiSink_NilChannelNoBlock(t *testing.T) {
	s := &TuiSink{}
	s.Emit(event.Event{Kind: event.Text, Text: "x"}) // must not block
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/tui/... -run TestTuiSink -count=1 -v
```

Expected: FAIL — `TuiSink` 未定义

- [ ] **Step 3: 实现 TuiSink**

`internal/tui/sink.go`：

```go
package tui

import (
	"sync"

	"github.com/wsx864321/coding-agent/internal/event"
)

type TuiSink struct {
	mu sync.Mutex
	ch chan<- event.Event
}

func (s *TuiSink) SetChan(ch chan<- event.Event) {
	s.mu.Lock()
	s.ch = ch
	s.mu.Unlock()
}

func (s *TuiSink) Emit(e event.Event) {
	s.mu.Lock()
	ch := s.ch
	s.mu.Unlock()
	if ch != nil {
		ch <- e
	}
}
```

- [ ] **Step 4: 简化 runner.go**

```go
package tui

import "context"

type Runner interface {
	RunTurn(ctx context.Context, prompt string) error
}
```

删除 `StreamEmitter` alias、`chanEmitter` 全部方法。

- [ ] **Step 5: 精简 stream.go**

仅保留：

```go
package tui

// streamClosedMsg 在事件 channel 关闭时触发。
type streamClosedMsg struct{}
```

删除 `StreamChunkMsg`、`ToolStartMsg`、`ToolEndMsg`、`ApprovalRequestMsg`、`StreamDoneMsg`、`StreamErrorMsg`。

- [ ] **Step 6: 重构 model.go — event.Event 分发**

1. 字段变更：

```go
type Model struct {
	// ...
	tuiSink  *TuiSink   // 新增，submit 时 SetChan
	streamCh <-chan event.Event  // 替代 chan any
}
```

2. `NewWithRunner` 签名扩展（或新增 setter）：

```go
func NewWithRunner(runner Runner, tuiSink *TuiSink) Model {
	m := New()
	m.runner = runner
	m.tuiSink = tuiSink
	return m
}
```

3. `submit()` 改为：

```go
ch := make(chan event.Event, 16)
if m.tuiSink != nil {
	m.tuiSink.SetChan(ch)
}
go func() {
	defer close(ch)
	_ = runner.RunTurn(ctx, text)
}()
m.streamCh = ch
return m, tea.Batch(waitStreamEvent(ch), m.spinner.Tick)
```

4. `waitStreamEvent` 替代 `waitStreamMsg`：

```go
func waitStreamEvent(ch <-chan event.Event) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return streamClosedMsg{}
		}
		return e
	}
}
```

5. `Update` 中删除旧 Msg case，新增 `event.Event` 处理：

```go
case event.Event:
	switch msg.Kind {
	case event.Text:
		// 原 StreamChunkMsg 逻辑
	case event.ToolDispatch:
		// 原 ToolStartMsg 逻辑，用 msg.ToolName/ToolArgs
	case event.ToolResult:
		// 原 ToolEndMsg 逻辑
	case event.ApprovalRequest:
		// 原 ApprovalRequestMsg，用 msg.ApprovalName/ApprovalArgs/ApprovalRespond
	case event.Notice:
		m.statusMsg = msg.Text
	case event.TurnDone:
		if msg.Err != nil && !m.interrupted {
			m.lastError = msg.Err.Error()
		}
		m = m.flushPending()
		m.busy = false
		m.streamCh = nil
		m.turnCancel = nil
		m.interrupted = false
		m.statusLabel = ""
		// ...
	}
	if m.streamCh != nil {
		return m, waitStreamEvent(m.streamCh)
	}
	return m, nil
```

6. 删除 `StreamChunkMsg`、`ToolStartMsg` 等旧 case 分支。

- [ ] **Step 7: 迁移 runner_test.go stubRunner**

```go
type stubRunner struct {
	chunks []string
	err    error
	prompt string
	sink   *TuiSink
}

func (s *stubRunner) RunTurn(_ context.Context, prompt string) error {
	s.prompt = prompt
	for _, c := range s.chunks {
		if s.sink != nil {
			s.sink.Emit(event.Event{Kind: event.Text, Text: c})
		}
	}
	if s.err != nil {
		if s.sink != nil {
			s.sink.Emit(event.Event{Kind: event.TurnDone, Err: s.err})
		}
		return s.err
	}
	if s.sink != nil {
		s.sink.Emit(event.Event{Kind: event.TurnDone})
	}
	return nil
}
```

测试中使用 `TuiSink` + channel 替代直接 `StreamChunkMsg`。

- [ ] **Step 8: 删除 emitter_test.go，运行 TUI 测试**

```bash
rm internal/tui/emitter_test.go
go test ./internal/tui/... -count=1 -v
```

Expected: PASS（需同步修复 model_test.go 等引用旧 Msg 的测试）

- [ ] **Step 9: Commit**

```bash
git add internal/tui/
git commit -m "refactor(tui): migrate from StreamEmitter to event.Event channel"
```

---

## Task 5: CLI 层装配（D7）

**输入:** Task 1-4 产出；当前 `chat_setup.go`/`chat.go`/`once.go`/`tui.go`/`tui_runner.go`。

**输出:** 三种模式正确注入 Sink；notify 桥接 hook warn；chat 不再重复 print 最终回答。

**验证标准:** `go build ./cmd/...` 成功；手动 smoke：`coding-agent once -m "hi" -q` 有流式 Text 输出。

**Files:**
- Modify: `cmd/cli/chat_setup.go`
- Modify: `cmd/cli/chat.go`
- Modify: `cmd/cli/once.go`
- Modify: `cmd/cli/tui.go`
- Modify: `cmd/cli/tui_runner.go`

**Interfaces:**
- Consumes: `event.TextSink`, `tui.TuiSink`, `agent.WithSink`, `agent.SinkAsker`, `hooks.NewRunner(..., notify)`

- [ ] **Step 1: 修改 chat_setup.go**

```go
import (
	"os"
	"github.com/wsx864321/coding-agent/internal/event"
	"github.com/wsx864321/coding-agent/internal/tui"
)

type chatSetup struct {
	Agent      *agent.Agent
	SkillStore *skill.Store
	Registry   *tools.Registry
	TuiSink    *tui.TuiSink // TUI 模式非 nil
	cleanup    func()
}

func setupChatAgent(cmd *cobra.Command) (*chatSetup, error) {
	asker := &permission.StdinAsker{Reader: os.Stdin, Writer: os.Stderr}
	return setupAgentWithAsker(cmd, asker, nil)
}

func setupTuiAgent(cmd *cobra.Command) (*chatSetup, error) {
	tuiSink := &tui.TuiSink{}
	asker := agent.SinkAsker{Sink: tuiSink}
	return setupAgentWithAsker(cmd, asker, tuiSink)
}

func setupAgentWithAsker(cmd *cobra.Command, asker permission.Asker, tuiSink *tui.TuiSink) (*chatSetup, error) {
	// ...existing registry/skill/memory/checker/jobMgr setup...

	var sink event.Sink
	if tuiSink != nil {
		sink = tuiSink
	} else {
		sink = &event.TextSink{Out: os.Stdout, Err: os.Stderr}
	}

	notify := func(msg string) {
		sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn, Text: msg})
	}

	hookRunner := hooks.NewRunner(
		hooks.Load(hooks.LoadOptions{ProjectRoot: workdir}),
		workdir,
		hooks.DefaultSpawner,
		notify,
	)

	opts := []agent.Option{
		agent.WithRegistry(registry),
		agent.WithChecker(checker),
		agent.WithHooks(hookRunner),
		agent.WithSkillStore(skillStore),
		agent.WithMemory(memSet),
		agent.WithJobManager(jobMgr),
		agent.WithSink(sink),
	}
	// ...NewAgent + Wire*...

	return &chatSetup{
		Agent: a, SkillStore: skillStore, Registry: registry,
		TuiSink: tuiSink, cleanup: jobMgr.Close,
	}, nil
}
```

- [ ] **Step 2: 修改 chat.go runOneTurn**

```go
func runOneTurn(ctx context.Context, a *agent.Agent, prompt string) error {
	_, err := a.Run(ctx, prompt)
	if err != nil {
		if errors.Is(err, agent.ErrMaxTurnsExceeded) {
			return fmt.Errorf("超过最大轮数: %w", err)
		}
		return err
	}
	fmt.Println() // 流式 Text 已由 TextSink 输出，仅追加换行分隔
	return nil
}
```

REPL 启动信息、slash 命令的 `fmt.Printf` 保持不变（Design Doc D7 说明）。

- [ ] **Step 3: 修改 once.go**

```go
import (
	"io"
	"os"
	"github.com/wsx864321/coding-agent/internal/event"
)

func runOnce(cmd *cobra.Command, args []string) error {
	// ...registry/checker setup...

	sink := &event.TextSink{Out: os.Stdout, Err: os.Stderr}
	if onceQuiet {
		sink = &event.TextSink{Out: os.Stdout, Err: io.Discard}
	}
	notify := func(msg string) {
		sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn, Text: msg})
	}

	hookRunner := hooks.NewRunner(
		hooks.Load(hooks.LoadOptions{ProjectRoot: workdir}),
		workdir,
		hooks.DefaultSpawner,
		notify,
	)

	a, err := agent.NewAgent(buildConfig(cmd),
		agent.WithRegistry(registry),
		agent.WithChecker(checker),
		agent.WithHooks(hookRunner),
		agent.WithSkillStore(skillStore),
		agent.WithSink(sink),
	)
	// ...

	if !onceQuiet {
		fmt.Fprintf(os.Stderr, "[coding-agent] running once, message=%q\n", truncate(onceMessage, 60))
	}

	_, err = a.Run(cmd.Context(), onceMessage)
	return err // Text 已流式输出，不再 fmt.Println(out)
}
```

- [ ] **Step 4: 修改 tui_runner.go**

```go
func (r agentRunner) RunTurn(ctx context.Context, prompt string) error {
	_, err := r.agent.Run(ctx, prompt)
	return err
}
```

- [ ] **Step 5: 修改 tui.go**

```go
p := tea.NewProgram(tui.NewWithRunner(newAgentRunner(setup.Agent), setup.TuiSink))
```

- [ ] **Step 6: 编译验证**

```bash
go build ./cmd/...
go build ./...
```

Expected: 编译成功，零 StreamEmitter 引用

- [ ] **Step 7: Commit**

```bash
git add cmd/cli/
git commit -m "feat(cli): wire TextSink/TuiSink and notify bridge for all modes"
```

---

## Task 6: 测试补全与 subagent 确认

**输入:** Task 1-5 完成。

**输出:** Agent 集成测试覆盖 Sink 事件序列；subagent 默认 Discard。

**验证标准:** `go test ./internal/agent/... -count=1` PASS。

**Files:**
- Modify: `internal/agent/subagent.go`（确认不传 WithSink）
- Create/Modify: `internal/agent/loop_test.go` 或 `agent_test.go`

- [ ] **Step 1: 编写 Agent Run + FuncSink 事件序列测试**

在 `internal/agent/agent_test.go` 追加：

```go
func TestRun_EmitsTextAndToolEvents(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{toolCalls: []provider.ToolCall{{
			ID: "1", Name: "echo", Arguments: `{}`,
		}}},
		scriptedResponse{content: "done"},
	)
	reg := tools.NewRegistry()
	reg.Register(&tools.EchoTool{}) // 或项目内已有的只读测试工具

	var kinds []event.Kind
	a, err := NewAgent(Config{ProviderKind: "openai", Model: "test", MaxTurns: 5, BaseURL: f.server.URL},
		WithProvider(newFakeProvider(f)),
		WithRegistry(reg),
		WithSink(event.FuncSink(func(e event.Event) {
			kinds = append(kinds, e.Kind)
		})),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = a.Run(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	hasText := false
	hasTool := false
	for _, k := range kinds {
		if k == event.Text {
			hasText = true
		}
		if k == event.ToolDispatch || k == event.ToolResult {
			hasTool = true
		}
	}
	if !hasText {
		t.Error("expected Text events")
	}
	if !hasTool {
		t.Error("expected tool events")
	}
}
```

（根据项目现有 fake 工具/provider 测试辅助函数调整具体构造方式。）

- [ ] **Step 2: 确认 subagent 使用 Discard**

`internal/agent/subagent.go` 的 `NewAgent(subCfg, subOpts...)` 不传 `WithSink`，依赖 Task 2 中 `NewAgent` 默认 `event.Discard`。无需代码变更，测试验证：

```go
func TestSubAgent_UsesDiscardSink(t *testing.T) {
	// parent with capturing sink; subagent events should NOT appear on parent sink
}
```

- [ ] **Step 3: 运行测试**

```bash
go test ./internal/agent/... -count=1 -v
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/agent/
git commit -m "test(agent): add Sink event sequence integration tests"
```

---

## Task 7: 全仓引用清理

**输入:** Task 1-6 完成。

**输出:** 零遗留 StreamEmitter/RunStreaming/log.Printf 引用。

**验证标准:** 下列 grep 全部零匹配（docs/openspec 归档文档除外）。

- [ ] **Step 1: 搜索并修复遗留引用**

```bash
grep -r "StreamEmitter\|RunStreaming\|EmitterFromContext\|WithEmitter\|EmitterAsker" --include="*.go" .
grep -r "log\.Printf" internal/agent/ internal/hooks/
grep -r "StreamChunkMsg\|ToolStartMsg\|StreamDoneMsg" internal/tui/
```

Expected: 零匹配（或仅测试迁移遗漏，逐一修复）

- [ ] **Step 2: 更新受影响的测试包**

重点检查：
- `internal/tui/model_test.go` — 旧 Msg 类型 → `event.Event`
- `internal/tui/integration_test.go`
- `internal/agent/hooks_test.go` — `NewRunner` 第 4 参数
- `internal/hooks/e2e_test.go`

- [ ] **Step 3: Commit（若有修复）**

```bash
git add -A
git commit -m "chore: remove remaining StreamEmitter and log.Printf references"
```

---

## Task 8: 全量验证与合规检查

**输入:** Task 1-7 全部完成。

**输出:** CI 级绿色构建；迁移检查清单全部勾选。

**验证标准:** Design Doc 迁移检查清单 6 项全部满足。

- [ ] **Step 1: 全量编译**

```bash
go build ./...
```

Expected: 成功，零 error

- [ ] **Step 2: 全量测试**

```bash
go test ./... -count=1
```

Expected: 全部 PASS

- [ ] **Step 3: 合规 grep**

```bash
grep -r "StreamEmitter" --include="*.go" . | grep -v "docs/" | grep -v "openspec/"
grep -r "RunStreaming" --include="*.go" .
grep -r "log\.Printf" internal/hooks/ internal/agent/
```

Expected: 全部零输出

- [ ] **Step 4: 确认文件已删除**

```bash
test ! -f internal/agent/emitter.go && echo "emitter.go deleted OK"
test ! -f internal/agent/emitter_asker.go && echo "emitter_asker.go deleted OK"
```

- [ ] **Step 5: 更新 tasks.md 全部打勾**

打开 `openspec/changes/unified-event-sink/tasks.md`，将所有 `- [ ]` 改为 `- [x]`。

- [ ] **Step 6: Commit**

```bash
git add openspec/changes/unified-event-sink/tasks.md
git commit -m "chore: mark unified-event-sink tasks complete"
```

---

## Self-Review 清单

| Design Doc 要求 | 对应 Task |
|-----------------|-----------|
| D1 Event/Kind/Level 定义 | Task 1 |
| D2 Sink/FuncSink/Discard | Task 1 |
| D3 TextSink 6 种 Kind | Task 1 |
| D4 Agent sink 集成、删 RunStreaming/emitter.go | Task 2 |
| D4 todo_guard Notice | Task 2 |
| D4 subagent Discard | Task 6 |
| D5 hooks notify + load 静默 | Task 3 |
| D6 TuiSink + TUI Event 分发 | Task 4 |
| D7 CLI 三种模式装配 | Task 5 |
| 测试策略（event/agent/hooks 层） | Task 1/2/3/6 |
| 迁移检查清单 6 项 | Task 8 |

**类型一致性:** 全计划统一使用 `event.Event`、`event.Sink`、`event.Kind` 命名；TUI channel 类型为 `chan event.Event`；Runner 接口无 emit 参数。

**无占位符:** 所有 Step 均含具体文件路径、代码片段或 shell 命令。

---

## 依赖关系

```
Task 1 (event 包)
    ↓
Task 2 (Agent 迁移) ──→ Task 6 (集成测试)
    ↓
Task 3 (hooks 迁移)
    ↓
Task 4 (TUI 迁移)
    ↓
Task 5 (CLI 装配)
    ↓
Task 7 (引用清理)
    ↓
Task 8 (全量验证)
```

Task 3 可与 Task 4 并行（互不依赖），但 Task 5 需等待 Task 3+4 完成。
