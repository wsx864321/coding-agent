package event

import "testing"

func TestNewKinds_ReasoningText_ToolProgress(t *testing.T) {
	// ReasoningText and ToolProgress must be distinct from existing kinds.
	if ReasoningText == Text {
		t.Fatal("ReasoningText must not equal Text")
	}
	if ToolProgress == Text {
		t.Fatal("ToolProgress must not equal Text")
	}
	if ReasoningText == ToolProgress {
		t.Fatal("ReasoningText must not equal ToolProgress")
	}
	if ReasoningText == Notice {
		t.Fatal("ReasoningText must not equal Notice")
	}
	if ToolProgress == Notice {
		t.Fatal("ToolProgress must not equal Notice")
	}
}

func TestEvent_NewFields_ReasoningChunk_ToolCallID_ToolChunk(t *testing.T) {
	ev := Event{
		Kind:           ReasoningText,
		ReasoningChunk: "thinking...",
	}
	if ev.ReasoningChunk != "thinking..." {
		t.Fatalf("ReasoningChunk = %q, want %q", ev.ReasoningChunk, "thinking...")
	}

	ev2 := Event{
		Kind:       ToolProgress,
		ToolCallID: "call_123",
		ToolChunk:  "partial output",
	}
	if ev2.ToolCallID != "call_123" {
		t.Fatalf("ToolCallID = %q, want %q", ev2.ToolCallID, "call_123")
	}
	if ev2.ToolChunk != "partial output" {
		t.Fatalf("ToolChunk = %q, want %q", ev2.ToolChunk, "partial output")
	}
}

func TestEvent_NewFields_ZeroValueSafe(t *testing.T) {
	// Existing event types should not access new fields; zero values must be safe.
	ev := Event{Kind: Text, Text: "hello"}
	if ev.ReasoningChunk != "" {
		t.Fatalf("ReasoningChunk zero value = %q, want empty", ev.ReasoningChunk)
	}
	if ev.ToolCallID != "" {
		t.Fatalf("ToolCallID zero value = %q, want empty", ev.ToolCallID)
	}
	if ev.ToolChunk != "" {
		t.Fatalf("ToolChunk zero value = %q, want empty", ev.ToolChunk)
	}
}
