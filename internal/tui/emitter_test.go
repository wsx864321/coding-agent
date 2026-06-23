package tui

import (
	"testing"
)

func TestChanEmitterForwardsToolStart(t *testing.T) {
	ch := make(chan any, 4)
	emit := chanEmitter{ch: ch}
	emit.OnToolStart("bash", `{"command":"ls"}`)

	select {
	case msg := <-ch:
		start, ok := msg.(ToolStartMsg)
		if !ok {
			t.Fatalf("msg type = %T, want ToolStartMsg", msg)
		}
		if start.Name != "bash" || start.Args != `{"command":"ls"}` {
			t.Fatalf("ToolStartMsg = %+v", start)
		}
	default:
		t.Fatal("expected ToolStartMsg on channel")
	}
}

func TestChanEmitterForwardsToolEnd(t *testing.T) {
	ch := make(chan any, 4)
	emit := chanEmitter{ch: ch}
	emit.OnToolEnd("bash", "ok", false)

	msg := <-ch
	end, ok := msg.(ToolEndMsg)
	if !ok {
		t.Fatalf("msg type = %T, want ToolEndMsg", msg)
	}
	if end.Name != "bash" || end.Result != "ok" || end.IsError {
		t.Fatalf("ToolEndMsg = %+v", end)
	}
}
