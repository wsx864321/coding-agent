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
