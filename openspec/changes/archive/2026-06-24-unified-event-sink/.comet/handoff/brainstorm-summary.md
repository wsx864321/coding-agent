# Brainstorm Summary

- Change: unified-event-sink
- Date: 2026-06-23

## 确认的技术方案

**方案 A：Agent 持有 Sink，完全内聚**

### event 包
- `internal/event/` 新包：Event (flat struct) + Kind (6 种) + Level (2 种) + Sink 接口 (单方法 Emit) + FuncSink + Discard + TextSink
- Kind: Text, ToolDispatch, ToolResult, ApprovalRequest, TurnDone, Notice
- Event 携带 ApprovalRespond func(bool) 回调（ApprovalRequest 事件）
- TextSink 接受 Out/Err io.Writer，按 Kind 分发渲染

### Agent 层
- Agent struct 新增 `sink event.Sink`，nil 时用 Discard
- 统一为单入口 Run()，删除 RunStreaming()
- 删除 StreamEmitter 接口 + emitter.go + context 注入机制
- loop.go 内部统一用 a.sink.Emit()
- invokeTool/executeBatch 不再传 emitter 参数
- Subagent 默认用 Discard（不传 Sink）
- todo_guard 的 2 处 log.Printf → sink.Emit(Notice{})

### Hook 层
- NewRunner 增加 notify func(string) 第 4 参数
- run.go 3 处 log.Printf → notify 回调
- load.go 4 处 log.Printf → 直接删除（静默降级）
- hook 包不依赖 event 包，通过 notify 解耦

### TUI 层
- 删除 chanEmitter + 6 个消息 struct
- Runner 接口改为 RunTurn(ctx, prompt) error（无 emit 参数）
- chanSink{ch chan<- event.Event} 实现 Sink
- TUI model Update 按 event.Event.Kind switch

### CLI 装配
- chat/once: TextSink(os.Stdout, os.Stderr) + notify 桥接到 Sink
- tui: TUI model 内部创建 chanSink
- once quiet: 精简 Sink（只输出最终 Text）

## 关键取舍与风险

- BREAKING: 删除 StreamEmitter，所有消费者同步迁移（项目无外部消费者）
- TUI Runner 接口变更：移除 emit 参数，影响 model.go goroutine 创建方式
- chat REPL 获得实时工具进度（UX 提升）
- ApprovalRequest 在 chat 模式：TextSink 不处理审批，仍走 StdinAsker
- Subagent hook notify 为 nil → 静默丢弃

## 测试策略

1. event 包：TextSink 各 Kind 输出格式、FuncSink、Discard
2. Agent 层：mock Sink 验证 Emit 调用序列
3. Hook 层：mock notify 验证 warn/error 消息、零 log.Printf
4. 集成：go build ./... + go test ./...

## Spec Patch

无（现有 delta spec 已覆盖所有需求）
