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

func TestSinkForwardsReasoningText(t *testing.T) {
	ch := make(chan event.Event, 1)
	s := &TuiSink{}
	s.SetChan(ch)
	s.Emit(event.Event{Kind: event.ReasoningText, ReasoningChunk: "let me think..."})
	got := <-ch
	if got.Kind != event.ReasoningText {
		t.Fatalf("expected Kind=ReasoningText, got %v", got.Kind)
	}
	if got.ReasoningChunk != "let me think..." {
		t.Fatalf("expected ReasoningChunk='let me think...', got %q", got.ReasoningChunk)
	}
}

func TestSinkForwardsToolProgress(t *testing.T) {
	ch := make(chan event.Event, 1)
	s := &TuiSink{}
	s.SetChan(ch)
	s.Emit(event.Event{Kind: event.ToolProgress, ToolCallID: "call_abc", ToolChunk: "partial output"})
	got := <-ch
	if got.Kind != event.ToolProgress {
		t.Fatalf("expected Kind=ToolProgress, got %v", got.Kind)
	}
	if got.ToolCallID != "call_abc" {
		t.Fatalf("expected ToolCallID='call_abc', got %q", got.ToolCallID)
	}
	if got.ToolChunk != "partial output" {
		t.Fatalf("expected ToolChunk='partial output', got %q", got.ToolChunk)
	}
}
