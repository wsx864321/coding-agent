package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

func TestTextareaShiftEnterInsertsNewline(t *testing.T) {
	m := New()
	m.textarea.SetValue("line1")

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	updated := next.(Model)
	if !strings.Contains(updated.textarea.Value(), "\n") {
		t.Fatalf("expected newline after shift+enter, got %q", updated.textarea.Value())
	}
}

func TestViewportPageDownScrolls(t *testing.T) {
	m := New()
	m.width = 40
	m.height = 12
	m.transcript = longTranscriptHistory()
	m = m.rerenderTranscript()
	m = m.syncLayout()
	m = m.syncViewportContent()
	m.viewport.GotoTop()
	before := m.viewport.YOffset()

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	updated := next.(Model)
	if updated.viewport.YOffset() <= before {
		t.Fatalf("YOffset = %d, want > %d after pgdown", updated.viewport.YOffset(), before)
	}
}

func TestViewportMouseWheelScrolls(t *testing.T) {
	m := New()
	m.width = 40
	m.height = 12
	m.transcript = longTranscriptHistory()
	m = m.rerenderTranscript()
	m = m.syncLayout()
	m = m.syncViewportContent()
	m.viewport.GotoTop()
	before := m.viewport.YOffset()

	next, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	updated := next.(Model)
	if updated.viewport.YOffset() <= before {
		t.Fatalf("YOffset = %d, want > %d after wheel down", updated.viewport.YOffset(), before)
	}
}

func TestSpinnerTickAnimatesWhenBusy(t *testing.T) {
	m := New()
	m.busy = true
	frame0 := m.spinner.View()

	next, cmd := m.Update(m.spinner.Tick())
	if cmd == nil {
		t.Fatal("spinner tick should schedule next frame when busy")
	}
	updated := next.(Model)
	if updated.spinner.View() == frame0 {
		t.Fatalf("spinner frame unchanged after tick: %q", frame0)
	}
}

func TestSpinnerStopsWhenNotBusy(t *testing.T) {
	m := New()
	m.busy = false

	next, cmd := m.Update(spinner.TickMsg{ID: m.spinner.ID()})
	if cmd != nil {
		t.Fatal("spinner tick should not schedule next frame when idle")
	}
	_ = next.(Model)
}

func TestTailFollowKeepsBottomWhenAtBottom(t *testing.T) {
	m := New()
	m.width = 40
	m.height = 12
	m.transcript = longTranscriptHistory()
	m = m.rerenderTranscript()
	m = m.syncLayout()
	m = m.syncViewportContent()
	m.viewport.GotoBottom()

	m = m.appendUserMessage("new tail message")
	m = m.syncViewportContent()

	if !m.viewport.AtBottom() {
		t.Fatalf("expected viewport at bottom after tail-follow, YOffset=%d", m.viewport.YOffset())
	}
}
