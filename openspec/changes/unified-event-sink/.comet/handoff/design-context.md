# Comet Design Handoff

- Change: unified-event-sink
- Phase: design
- Mode: compact
- Context hash: 0a93cc47b8ad57ae2c4fecbc37d8d107355f58141bcaa49647a3d79051d4be96

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/unified-event-sink/proposal.md

- Source: openspec/changes/unified-event-sink/proposal.md
- Lines: 1-35
- SHA256: c884c72225260cde57d9e40c8acd9fd5d3b9bdd6f86e6a589f91cb9ce346711e

```md
## Why

当前项目的运行时输出分散在三种机制中：`log.Printf`（9 处，写 stderr，TUI 用户不可见）、`fmt.Print*`（~46 处，chat REPL 直写控制台）、`StreamEmitter`（6 个 callback 方法，仅 TUI 使用）。这导致 hook warn/error、todo guard 阻断、compact 状态等关键信息在 TUI 模式下对用户不可见，且无法统一适配多前端（CLI/TUI/未来 serve）。参考 Reasonix 的 `event.Sink` 架构，引入统一事件体系解决这一问题。

## What Changes

- **新增** `internal/event/` 包：定义 `Event`（typed struct）、`Kind` 枚举（6 种）、`Sink` 接口（单方法 `Emit`）
- **删除** `StreamEmitter` 接口及 context 注入机制，用 `event.Sink` 完全取代
- **新增** `TextSink` 实现：ANSI 终端输出，供 chat REPL 和 once 模式使用
- **改造** TUI 的 `chanEmitter` 为 `Sink` 实现
- **移除** `internal/hooks/` 和 `internal/agent/` 中全部 9 处 `log.Printf`
  - hook 加载错误改为静默降级
  - hook 执行 warn/error 通过 `notify` 回调走 `Notice` 事件
  - todo guard 日志改为 `Notice` 事件
- **改造** chat REPL（`cmd/cli/chat.go`）的 agent 运行时输出走 `TextSink`
- **改造** once 模式（`cmd/cli/once.go`）的输出走 `Sink`

## Capabilities

### New Capabilities

- `event-sink`: 统一事件流体系，定义 Event/Kind/Sink 接口和 TextSink 参考实现

### Modified Capabilities

- `shell-hook-engine`: hook 执行结果输出从 `log.Printf` 迁移到 `notify` 回调 → `Notice` 事件

## Impact

- `internal/event/`（新包）
- `internal/agent/`：删除 `emitter.go`，改造 `loop.go`、`agent.go`、`option.go`
- `internal/hooks/`：改造 `load.go`、`run.go`、`runner.go`
- `internal/tui/`：改造 `runner.go`（chanEmitter → Sink）
- `cmd/cli/`：改造 `chat.go`、`once.go`、`chat_setup.go`、`tui.go`
- **BREAKING**：删除 `StreamEmitter` 接口，所有依赖方需迁移到 `event.Sink`
```

## openspec/changes/unified-event-sink/design.md

- Source: openspec/changes/unified-event-sink/design.md
- Lines: 1-162
- SHA256: 23f6afe43e1c61c6dc81b1dff36f8a24222802a2dc8bec676b97a27cdf8f2b06

[TRUNCATED]

```md
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
```

Full source: openspec/changes/unified-event-sink/design.md

## openspec/changes/unified-event-sink/tasks.md

- Source: openspec/changes/unified-event-sink/tasks.md
- Lines: 1-41
- SHA256: 0a92a19de21598d589b8c1ae05f6a995d2c62a70fc43366267149225123c1ded

```md
## 1. event 包核心定义

- [ ] 1.1 创建 `internal/event/event.go`：定义 Kind 枚举（6 种）、Level 枚举、Event struct
- [ ] 1.2 创建 `internal/event/sink.go`：定义 Sink 接口、FuncSink 适配器、Discard 实例
- [ ] 1.3 创建 `internal/event/textsink.go`：实现 TextSink（Out/Err io.Writer），渲染 6 种事件到 ANSI 终端
- [ ] 1.4 编写 event 包单元测试：TextSink 各 Kind 输出格式、FuncSink、Discard

## 2. Agent 层迁移（StreamEmitter → Sink）

- [ ] 2.1 修改 `internal/agent/option.go`：新增 `WithSink(event.Sink)` Option，移除 `WithEmitter` 相关
- [ ] 2.2 修改 `internal/agent/agent.go`：Agent struct 新增 `sink event.Sink` 字段（nil 时用 Discard）
- [ ] 2.3 修改 `internal/agent/loop.go`：所有 emitter.OnXxx 调用替换为 sink.Emit
- [ ] 2.4 修改 `internal/agent/subagent.go`：subagent 的 Sink 传递
- [ ] 2.5 删除 `internal/agent/emitter.go`：移除 StreamEmitter 接口及 context 注入
- [ ] 2.6 更新 `internal/agent/agent_test.go`：适配 Sink，移除 StreamEmitter 依赖

## 3. Hook 层迁移（log.Printf → notify）

- [ ] 3.1 修改 `internal/hooks/runner.go`：NewRunner 增加 notify 参数，Runner 持有 notify 字段
- [ ] 3.2 修改 `internal/hooks/run.go`：移除 3 处 log.Printf，非 pass outcome 通过 notify 输出
- [ ] 3.3 修改 `internal/hooks/load.go`：移除 4 处 log.Printf，错误静默降级
- [ ] 3.4 修改 `internal/agent/todo_guard.go`：移除 2 处 log.Printf，改为通过 Agent 的 sink 发 Notice
- [ ] 3.5 更新 hook 相关测试：验证 notify 回调被正确调用，验证零 log.Printf

## 4. TUI 迁移（chanEmitter → Sink）

- [ ] 4.1 修改 `internal/tui/runner.go`：chanEmitter 改为实现 event.Sink
- [ ] 4.2 修改 TUI model Update：根据 Event.Kind 分发渲染，替代原有多 message type
- [ ] 4.3 更新 `cmd/cli/tui.go` / `cmd/cli/tui_runner.go`：装配 channel Sink

## 5. CLI 层装配

- [ ] 5.1 修改 `cmd/cli/chat_setup.go`：创建 TextSink 传入 Agent，notify 桥接到 Sink
- [ ] 5.2 修改 `cmd/cli/chat.go`：agent 运行时输出迁移到 TextSink（REPL 启动信息和 slash 命令保持 fmt）
- [ ] 5.3 修改 `cmd/cli/once.go`：创建 TextSink（quiet 模式用精简 Sink），notify 桥接

## 6. 清理与验证

- [ ] 6.1 全量编译 `go build ./...`
- [ ] 6.2 全量测试 `go test ./...`
- [ ] 6.3 验证零 log.Printf：grep 确认 internal/hooks/ 和 internal/agent/ 无 log.Printf 调用
```

## openspec/changes/unified-event-sink/specs/event-sink/spec.md

- Source: openspec/changes/unified-event-sink/specs/event-sink/spec.md
- Lines: 1-91
- SHA256: 16f7a63988c78b9deca5a1a473deb87d1eae52fb325eb80c3525d485c92256f6

[TRUNCATED]

```md
## ADDED Requirements

### Requirement: 统一事件类型定义
系统 MUST 定义 `internal/event` 包，包含 `Event` struct、`Kind` 枚举和 `Level` 枚举。Kind 包含 6 种事件：`Text`、`ToolDispatch`、`ToolResult`、`ApprovalRequest`、`TurnDone`、`Notice`。Level 包含 `LevelInfo` 和 `LevelWarn`。

#### Scenario: Event struct 包含所有必要字段
- **WHEN** 创建任何类型的 Event
- **THEN** Event struct 包含 Kind、Level、Text、ToolName、ToolArgs、ToolOutput、ToolIsErr、ApprovalRespond、Err 字段

#### Scenario: Kind 枚举完整
- **WHEN** 枚举所有 Kind 常量
- **THEN** 共 6 种：Text、ToolDispatch、ToolResult、ApprovalRequest、TurnDone、Notice

### Requirement: Sink 接口
系统 MUST 定义 `Sink` 接口，仅包含单方法 `Emit(Event)`。系统 MUST 提供 `FuncSink` 函数适配器和 `Discard` 丢弃实现。

#### Scenario: Sink 接口单方法
- **WHEN** 实现 Sink 接口
- **THEN** 只需实现 `Emit(Event)` 一个方法

#### Scenario: Discard Sink 丢弃所有事件
- **WHEN** 使用 Discard Sink
- **THEN** 所有 Emit 调用被静默忽略

#### Scenario: FuncSink 适配函数为 Sink
- **WHEN** 将 `func(Event)` 转为 FuncSink
- **THEN** 该函数可作为 Sink 使用

### Requirement: TextSink 终端输出实现
系统 MUST 提供 `TextSink` struct，实现 `Sink` 接口，将事件渲染为 ANSI 终端文本。TextSink 接受 `Out`（stdout）和 `Err`（stderr）两个 `io.Writer`。

#### Scenario: Text 事件直接写 Out
- **WHEN** TextSink 收到 Kind=Text 的事件
- **THEN** 事件 Text 字段直接写入 Out（无额外换行）

#### Scenario: ToolDispatch 写工具摘要到 Err
- **WHEN** TextSink 收到 Kind=ToolDispatch 的事件
- **THEN** 格式化为工具名和参数摘要写入 Err

#### Scenario: ToolResult 写结果到 Err
- **WHEN** TextSink 收到 Kind=ToolResult 且 ToolIsErr=false
- **THEN** 格式化为成功摘要写入 Err

#### Scenario: ToolResult 错误写到 Err
- **WHEN** TextSink 收到 Kind=ToolResult 且 ToolIsErr=true
- **THEN** 格式化为错误摘要写入 Err

#### Scenario: Notice warn 写到 Err
- **WHEN** TextSink 收到 Kind=Notice 且 Level=LevelWarn
- **THEN** 以警告格式写入 Err

#### Scenario: Notice info 写到 Err
- **WHEN** TextSink 收到 Kind=Notice 且 Level=LevelInfo
- **THEN** 以信息格式写入 Err

### Requirement: Agent 通过 Sink 输出
Agent 核心循环 MUST 通过注入的 `event.Sink` 发射所有运行时事件。Sink 为 nil 时使用 `Discard`。删除 `StreamEmitter` 接口和 context 注入机制。

#### Scenario: Agent 注入 Sink
- **WHEN** Agent 创建时传入 Sink
- **THEN** loop.go 中文本流、工具执行、审批、完成事件均通过 Sink.Emit 发射

#### Scenario: Agent 无 Sink 时静默
- **WHEN** Agent 创建时未注入 Sink
- **THEN** 使用 Discard，所有事件被丢弃，agent 正常运行

#### Scenario: StreamEmitter 已删除
- **WHEN** 搜索 StreamEmitter 引用
- **THEN** 整个代码库无 StreamEmitter 类型定义或使用

### Requirement: TUI Sink 实现
TUI 模式 MUST 通过 channel-based Sink 实现将事件转为 Bubble Tea 消息。

#### Scenario: TUI 事件通过 channel 传递
- **WHEN** TUI 模式 agent 发射事件
- **THEN** 事件通过 channel 送入 Bubble Tea Update 循环渲染

### Requirement: CLI 装配 Sink
`cmd/cli/` MUST 为每种运行模式装配合适的 Sink：chat REPL 用 TextSink，TUI 用 channel Sink，once 用 TextSink 或 Discard（quiet 模式）。

```

Full source: openspec/changes/unified-event-sink/specs/event-sink/spec.md

## openspec/changes/unified-event-sink/specs/shell-hook-engine/spec.md

- Source: openspec/changes/unified-event-sink/specs/shell-hook-engine/spec.md
- Lines: 1-39
- SHA256: fbb3c517531fbd81e61946b939e1a514a0bf14805ca5fd1dea563e2e1c47d335

```md
## MODIFIED Requirements

### Requirement: 外部 hook 通过 stdin JSON payload 和 exit code 通信
系统 MUST 通过 stdin 向外部命令传入 JSON payload，并根据 exit code 决定行为。payload 包含 `event`、`cwd` 及事件相关字段（`toolName`、`toolArgs`、`toolResult`、`prompt`、`messages`）。Hook 执行的 warn/error/block 结果 MUST 通过注入的 `notify` 回调传递给调用方，而非直接写 stderr 日志。配置加载错误（文件读取失败、JSON 解析失败、正则非法）MUST 静默降级，不写日志。

#### Scenario: PreToolUse hook 接收 JSON payload
- **WHEN** PreToolUse 事件触发，匹配到已注册的外部 hook
- **THEN** 系统 spawn 外部命令，通过 stdin 传入 `{"event":"PreToolUse","cwd":"...","toolName":"bash","toolArgs":{...}}` 格式的 JSON

#### Scenario: exit 0 表示 pass
- **WHEN** 外部命令返回 exit code 0
- **THEN** 系统视为 pass，继续执行后续 hook 和操作

#### Scenario: exit 2 在阻塞型事件中表示 block
- **WHEN** PreToolUse 事件中外部命令返回 exit code 2
- **THEN** 系统阻止该操作，将 stderr 或 stdout 作为阻止原因传回 agent，并通过 notify 回调通知

#### Scenario: exit 2 在非阻塞型事件中表示 warn
- **WHEN** PostToolUse 或 Stop 事件中外部命令返回 exit code 2
- **THEN** 系统通过 notify 回调发送警告消息，不阻止操作

#### Scenario: hook 执行 spawn 失败
- **WHEN** hook 外部命令 spawn 失败
- **THEN** 系统通过 notify 回调发送错误消息，不写 log.Printf

#### Scenario: hook 配置加载失败静默降级
- **WHEN** hooks.json 文件不存在、JSON 解析失败或 match 正则非法
- **THEN** 系统静默跳过该配置，不写 log.Printf，不影响其他 hook 加载

### Requirement: Spawner 可注入以支持测试
系统 MUST 支持通过注入自定义 Spawner 函数替代默认的 shell 进程 spawn，以便单元测试无需启动真实子进程。Runner 构造 MUST 额外接受 `notify func(string)` 参数，nil 时丢弃所有通知。

#### Scenario: 使用自定义 Spawner 进行测试
- **WHEN** 创建 Runner 时传入 mock Spawner
- **THEN** hook 执行时调用 mock Spawner 而非 DefaultSpawner

#### Scenario: notify 为 nil 时静默
- **WHEN** 创建 Runner 时 notify 参数为 nil
- **THEN** 所有 warn/error 消息被丢弃
```

