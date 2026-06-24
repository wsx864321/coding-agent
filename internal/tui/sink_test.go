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
