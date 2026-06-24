package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/wsx864321/coding-agent/internal/event"
)

func TestReasoningTextCreatesSummaryLine(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	next, _ := m.Update(event.Event{Kind: event.ReasoningText, ReasoningChunk: "analyzing..."})
	updated := next.(Model)

	if len(updated.transcript) != 1 {
		t.Fatalf("transcript = %d, want 1 after ReasoningText", len(updated.transcript))
	}
	if updated.transcript[0].Kind != EntryReasoning {
		t.Fatalf("transcript[0].Kind = %v, want EntryReasoning", updated.transcript[0].Kind)
	}
	if updated.transcript[0].Raw != "analyzing..." {
		t.Fatalf("transcript[0].Raw = %q, want 'analyzing...'", updated.transcript[0].Raw)
	}
	if updated.reasoningLineIdx != 0 {
		t.Fatalf("reasoningLineIdx = %d, want 0", updated.reasoningLineIdx)
	}
}

func TestReasoningCompletionOnTextEvent(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	// First, create a reasoning entry via ingestReasoningChunk.
	m = m.ingestReasoningChunk("thinking step by step")
	if m.reasoningLineIdx < 0 {
		t.Fatal("reasoningLineIdx should be >= 0 after ingestReasoningChunk")
	}

	// Send Text event to signal reasoning completion.
	next, _ := m.Update(event.Event{Kind: event.Text, Text: "answer"})
	updated := next.(Model)

	if updated.reasoningLineIdx != -1 {
		t.Fatalf("reasoningLineIdx = %d, want -1 after Text event", updated.reasoningLineIdx)
	}
	// The transcript entry should still exist.
	if len(updated.transcript) < 1 {
		t.Fatal("transcript should have at least one entry")
	}
}

func TestReasoningCtrlOToggle(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true
	m = m.ingestReasoningChunk("my analysis")
	if m.reasoningLineIdx < 0 {
		t.Fatal("reasoningLineIdx should be >= 0 after ingestReasoningChunk")
	}

	// Initially showReasoning is false.
	if m.showReasoning {
		t.Fatal("showReasoning should initially be false")
	}

	// Press Ctrl+O to toggle showReasoning to true.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	updated := next.(Model)

	if !updated.showReasoning {
		t.Fatal("showReasoning should be true after first Ctrl+O")
	}

	// Press Ctrl+O again to toggle back to false.
	next, _ = updated.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	updated = next.(Model)

	if updated.showReasoning {
		t.Fatal("showReasoning should be false after second Ctrl+O")
	}
}

// TestReasoningResetOnSubmit is defined in model_test.go (line 521).
// It is not duplicated here to avoid redeclaration.
