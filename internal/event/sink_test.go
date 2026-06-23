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
