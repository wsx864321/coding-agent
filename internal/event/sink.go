package event

type Sink interface {
	Emit(Event)
}

type FuncSink func(Event)

func (f FuncSink) Emit(e Event) { f(e) }

var Discard Sink = FuncSink(func(Event) {})
