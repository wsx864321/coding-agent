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

#### Scenario: chat 模式使用 TextSink
- **WHEN** 启动 chat REPL
- **THEN** 创建 TextSink(stdout, stderr) 传入 Agent

#### Scenario: once 模式使用 TextSink
- **WHEN** 启动 once 模式
- **THEN** 创建 TextSink 传入 Agent；quiet 模式下仅输出最终 Text

#### Scenario: tui 模式使用 channel Sink
- **WHEN** 启动 tui 模式
- **THEN** 创建 channel Sink 传入 Agent
