package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/wsx864321/coding-agent/internal/event"
)

func TestToolProgressCreatesStreamBlock(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	// First ToolProgress should create a stream block
	next, _ := m.Update(event.Event{
		Kind:       event.ToolProgress,
		ToolCallID: "call_abc",
		ToolChunk:  "line1\n",
	})
	updated := next.(Model)

	if len(updated.transcript) != 1 {
		t.Fatalf("transcript = %d, want 1 after first ToolProgress", len(updated.transcript))
	}
	if updated.transcript[0].Kind != EntryToolStream {
		t.Fatalf("transcript[0].Kind = %v, want EntryToolStream", updated.transcript[0].Kind)
	}
	if updated.toolStreamIdx != 0 {
		t.Fatalf("toolStreamIdx = %d, want 0", updated.toolStreamIdx)
	}
	if updated.toolStreamID != "call_abc" {
		t.Fatalf("toolStreamID = %q, want 'call_abc'", updated.toolStreamID)
	}
	if updated.toolLineCount != 1 {
		t.Fatalf("toolLineCount = %d, want 1", updated.toolLineCount)
	}
	if len(updated.toolTail) != 1 {
		t.Fatalf("toolTail = %v, want 1 line", updated.toolTail)
	}
	if updated.toolTail[0] != "line1" {
		t.Fatalf("toolTail[0] = %q, want 'line1'", updated.toolTail[0])
	}
}

func TestToolProgressAppendsChunks(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	// First chunk: partial line
	next, _ := m.Update(event.Event{
		Kind:       event.ToolProgress,
		ToolCallID: "call_abc",
		ToolChunk:  "hello ",
	})
	updated := next.(Model)

	if updated.toolPartial != "hello " {
		t.Fatalf("toolPartial = %q, want 'hello '", updated.toolPartial)
	}

	// Second chunk: completes the line
	next, _ = updated.Update(event.Event{
		Kind:       event.ToolProgress,
		ToolCallID: "call_abc",
		ToolChunk:  "world\n",
	})
	updated = next.(Model)

	if updated.toolPartial != "" {
		t.Fatalf("toolPartial = %q, want empty after newline", updated.toolPartial)
	}
	if updated.toolLineCount != 1 {
		t.Fatalf("toolLineCount = %d, want 1", updated.toolLineCount)
	}
	if len(updated.toolTail) != 1 {
		t.Fatalf("toolTail = %v, want 1 line", updated.toolTail)
	}
	if updated.toolTail[0] != "hello world" {
		t.Fatalf("toolTail[0] = %q, want 'hello world'", updated.toolTail[0])
	}
}

func TestToolProgressTailTruncation(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	// Send 25 lines (more than the 20-line limit)
	for i := 0; i < 25; i++ {
		next, _ := m.Update(event.Event{
			Kind:       event.ToolProgress,
			ToolCallID: "call_abc",
			ToolChunk:  "line\n",
		})
		m = next.(Model)
	}

	if m.toolLineCount != 25 {
		t.Fatalf("toolLineCount = %d, want 25", m.toolLineCount)
	}
	if len(m.toolTail) != 20 {
		t.Fatalf("toolTail len = %d, want 20 (truncated)", len(m.toolTail))
	}
}

func TestToolProgressLineCount(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	// Send chunks with multiple newlines
	next, _ := m.Update(event.Event{
		Kind:       event.ToolProgress,
		ToolCallID: "call_abc",
		ToolChunk:  "a\nb\nc\n",
	})
	m = next.(Model)

	if m.toolLineCount != 3 {
		t.Fatalf("toolLineCount = %d, want 3", m.toolLineCount)
	}
	if len(m.toolTail) != 3 {
		t.Fatalf("toolTail len = %d, want 3", len(m.toolTail))
	}
}

func TestToolProgressNoopWhenNotBusy(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = false

	next, _ := m.Update(event.Event{
		Kind:       event.ToolProgress,
		ToolCallID: "call_abc",
		ToolChunk:  "output\n",
	})
	updated := next.(Model)

	if len(updated.transcript) != 0 {
		t.Fatalf("transcript = %d, want 0 when not busy", len(updated.transcript))
	}
}

func TestToolProgressDifferentIDResetsStream(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	// First tool
	next, _ := m.Update(event.Event{
		Kind:       event.ToolProgress,
		ToolCallID: "call_1",
		ToolChunk:  "tool1 output\n",
	})
	updated := next.(Model)

	if updated.toolStreamID != "call_1" {
		t.Fatalf("toolStreamID = %q, want 'call_1'", updated.toolStreamID)
	}
	if updated.toolLineCount != 1 {
		t.Fatalf("toolLineCount = %d, want 1", updated.toolLineCount)
	}

	// Second tool with different ID should reset
	next, _ = updated.Update(event.Event{
		Kind:       event.ToolProgress,
		ToolCallID: "call_2",
		ToolChunk:  "tool2 output\n",
	})
	updated = next.(Model)

	if updated.toolStreamID != "call_2" {
		t.Fatalf("toolStreamID = %q, want 'call_2'", updated.toolStreamID)
	}
	if updated.toolLineCount != 1 {
		t.Fatalf("toolLineCount = %d, want 1 after reset", updated.toolLineCount)
	}
	if len(updated.toolTail) != 1 {
		t.Fatalf("toolTail len = %d, want 1 after reset", len(updated.toolTail))
	}
}

func TestRenderToolStreamBlock(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true
	m.toolStreamStart = time.Now()
	m.toolStreamID = "call_abc"
	m.toolTail = []string{"line1", "line2", "line3"}
	m.toolLineCount = 3

	content := m.renderToolStreamBlock()

	if content == "" {
		t.Fatal("renderToolStreamBlock should return non-empty content")
	}
	if !strings.Contains(content, "working") {
		t.Fatalf("stream block should contain 'working', got: %q", content)
	}
	if !strings.Contains(content, "line1") {
		t.Fatalf("stream block should contain tail lines, got: %q", content)
	}
	if !strings.Contains(content, "line2") {
		t.Fatalf("stream block should contain tail lines, got: %q", content)
	}
	if !strings.Contains(content, "line3") {
		t.Fatalf("stream block should contain tail lines, got: %q", content)
	}
	if !strings.Contains(content, "3 lines") {
		t.Fatalf("stream block should contain line count summary, got: %q", content)
	}
}

func TestRenderToolStreamBlockEmpty(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true
	m.toolStreamStart = time.Now()
	m.toolStreamID = "call_abc"
	m.toolTail = nil
	m.toolLineCount = 0

	content := m.renderToolStreamBlock()

	if content == "" {
		t.Fatal("renderToolStreamBlock should return non-empty content even with no lines")
	}
	if !strings.Contains(content, "working") {
		t.Fatalf("stream block should contain 'working', got: %q", content)
	}
	if !strings.Contains(content, "0 lines") {
		t.Fatalf("stream block should contain '0 lines', got: %q", content)
	}
}

func TestToolProgressStreamStartTime(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	if !m.toolStreamStart.IsZero() {
		t.Fatal("toolStreamStart should be zero initially")
	}

	next, _ := m.Update(event.Event{
		Kind:       event.ToolProgress,
		ToolCallID: "call_abc",
		ToolChunk:  "output\n",
	})
	updated := next.(Model)

	if updated.toolStreamStart.IsZero() {
		t.Fatal("toolStreamStart should be set on first ToolProgress")
	}
}

func TestToolResultCollapsesStream(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	// Create stream block
	next, _ := m.Update(event.Event{
		Kind:       event.ToolProgress,
		ToolCallID: "call_abc",
		ToolChunk:  "line1\nline2\nline3\n",
	})
	updated := next.(Model)

	if updated.toolStreamIdx < 0 {
		t.Fatal("toolStreamIdx should be >= 0 after ToolProgress")
	}

	// ToolResult should reset stream state
	next, _ = updated.Update(event.Event{
		Kind:       event.ToolResult,
		ToolName:   "bash",
		ToolOutput: "line1\nline2\nline3\n",
	})
	updated = next.(Model)

	if updated.toolStreamIdx != -1 {
		t.Fatalf("toolStreamIdx = %d, want -1 after ToolResult", updated.toolStreamIdx)
	}
	if updated.toolStreamID != "" {
		t.Fatalf("toolStreamID = %q, want empty after ToolResult", updated.toolStreamID)
	}
	if updated.toolTail != nil {
		t.Fatalf("toolTail = %v, want nil after ToolResult", updated.toolTail)
	}
	if updated.toolLineCount != 0 {
		t.Fatalf("toolLineCount = %d, want 0 after ToolResult", updated.toolLineCount)
	}
}

func TestToolStreamFieldsInitializedInNew(t *testing.T) {
	m := New()
	if m.toolStreamIdx != -1 {
		t.Fatalf("toolStreamIdx = %d, want -1", m.toolStreamIdx)
	}
	if m.toolStreamID != "" {
		t.Fatalf("toolStreamID = %q, want empty", m.toolStreamID)
	}
	if m.toolTail != nil {
		t.Fatalf("toolTail = %v, want nil", m.toolTail)
	}
	if m.toolPartial != "" {
		t.Fatalf("toolPartial = %q, want empty", m.toolPartial)
	}
	if m.toolLineCount != 0 {
		t.Fatalf("toolLineCount = %d, want 0", m.toolLineCount)
	}
	if !m.toolStreamStart.IsZero() {
		t.Fatal("toolStreamStart should be zero time initially")
	}
}

func TestEntryToolStreamKindExists(t *testing.T) {
	// EntryToolStream must be a distinct EntryKind with value 6.
	if EntryToolStream != 6 {
		t.Fatalf("EntryToolStream = %d, want 6", EntryToolStream)
	}
}
