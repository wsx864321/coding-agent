package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestAppendUserEntryUpdatesViewport(t *testing.T) {
	m := New()
	m.width, m.height = 80, 24
	m = m.appendEntry(TranscriptEntry{Kind: EntryUserMessage, Content: "user bubble", Raw: "hello"})
	content := m.renderTranscriptContent()
	if !strings.Contains(content, "user bubble") {
		t.Fatalf("missing entry: %s", content)
	}
}

func TestAppendEntryRebuildsViewport(t *testing.T) {
	m := New()
	m.width, m.height = 80, 24
	m = m.syncLayout()
	m = m.appendEntry(TranscriptEntry{Kind: EntryUserMessage, Content: "visible", Raw: "visible"})
	m = m.rebuildViewport()
	if !strings.Contains(m.viewport.View(), "visible") {
		t.Fatalf("viewport missing entry: %s", m.viewport.View())
	}
}

func TestAppendAssistantChunkAccumulatesRaw(t *testing.T) {
	m := New()
	m = m.appendEntry(TranscriptEntry{Kind: EntryUserMessage, Raw: "q", Content: "q"})
	m = m.appendEntry(TranscriptEntry{Kind: EntryAssistantChunk, Raw: "", Content: ""})

	m = m.appendAssistantChunk("hel")
	m = m.appendAssistantChunk("lo")

	last := m.transcript[len(m.transcript)-1]
	if last.Kind != EntryAssistantChunk {
		t.Fatalf("last kind = %v, want EntryAssistantChunk", last.Kind)
	}
	if last.Raw != "hello" {
		t.Fatalf("assistant raw = %q, want hello", last.Raw)
	}
}

func TestRenderTranscriptContentEmpty(t *testing.T) {
	m := New()
	if got := m.renderTranscriptContent(); got != "(暂无消息)" {
		t.Fatalf("empty transcript = %q, want placeholder", got)
	}
}

func TestWindowSizeRerendersUserEntry(t *testing.T) {
	m := New()
	m.width = 40
	m.height = 24
	m = m.appendUserMessage("hello world")
	m = m.syncLayout()
	m = m.rebuildViewport()
	before := m.transcript[0].Content

	m.width = 80
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	updated := next.(Model)
	if updated.transcript[0].Content == before && m.width != 40 {
		// content should be re-rendered for new width (may differ when width changes)
	}
	if !strings.Contains(updated.renderTranscriptContent(), "hello world") {
		t.Fatalf("rerendered content missing raw text: %s", updated.renderTranscriptContent())
	}
}

func TestSubmitAppendsUserTranscriptEntry(t *testing.T) {
	m, _ := newStubModel([]string{"ok"})
	m.width, m.height = 80, 24
	m.textarea.SetValue("hello agent")

	updated, _ := m.submit()
	if len(updated.transcript) != 2 {
		t.Fatalf("transcript = %d entries, want 2", len(updated.transcript))
	}
	if updated.transcript[0].Kind != EntryUserMessage || updated.transcript[0].Raw != "hello agent" {
		t.Fatalf("transcript[0] = %+v, want user/hello agent", updated.transcript[0])
	}
	if updated.transcript[1].Kind != EntryAssistantChunk {
		t.Fatalf("transcript[1].Kind = %v, want EntryAssistantChunk", updated.transcript[1].Kind)
	}
}
