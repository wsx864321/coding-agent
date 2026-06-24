package event

// Sink 接收 Agent 事件用于展示或日志输出。
//
// 实现必须保证 Emit 可被多个 goroutine 并发调用。Agent 可能在主循环
// 以及并行工具执行的 goroutine 中同时调用 Emit。
type Sink interface {
	Emit(Event)
}

type FuncSink func(Event)

func (f FuncSink) Emit(e Event) { f(e) }

var Discard Sink = FuncSink(func(Event) {})
