package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestScrollWithKAndJ(t *testing.T) {
	m := New()
	m.width = 40
	m.height = 12
	m.messages = longMessageHistory()
	m = m.clampScrollToBottom()
	bottom := m.scrollOffset

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k'})
	scrolledUp := next.(Model)
	if scrolledUp.scrollOffset >= bottom {
		t.Fatalf("scrollOffset = %d, want < %d after k", scrolledUp.scrollOffset, bottom)
	}

	next, _ = scrolledUp.Update(tea.KeyPressMsg{Code: 'j'})
	scrolledDown := next.(Model)
	if scrolledDown.scrollOffset <= scrolledUp.scrollOffset {
		t.Fatalf("scrollOffset = %d, want > %d after j", scrolledDown.scrollOffset, scrolledUp.scrollOffset)
	}
}

func TestEscInterruptsBusyTurn(t *testing.T) {
	m := NewWithRunner(&stubRunner{chunks: []string{"partial"}})
	m.busy = true
	m.streamCh = make(chan any, 1)
	m.width = 80
	m.height = 24

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Fatal("Esc interrupt should not return a command")
	}
	updated := next.(Model)
	if updated.busy {
		t.Fatal("busy should be false after Esc interrupt")
	}
	if !strings.Contains(viewContent(updated), "已中断") {
		t.Fatalf("View should show interrupted feedback:\n%s", viewContent(updated))
	}
}

func TestEscInterruptAllowsInputAfter(t *testing.T) {
	m := NewWithRunner(&stubRunner{})
	m.busy = true

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	updated := next.(Model)

	typed := typeText(updated, "next")
	if typed.input != "next" {
		t.Fatalf("input = %q, want next after interrupt", typed.input)
	}
}

func TestViewShowsErrorBanner(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.lastError = "network down"

	view := viewContent(m)
	if !strings.Contains(view, "network down") {
		t.Fatalf("View should show error text:\n%s", view)
	}
	if !strings.Contains(view, "错误") {
		t.Fatalf("View should label error area:\n%s", view)
	}
}

func TestStreamErrorVisibleInView(t *testing.T) {
	m := New()
	m.busy = true
	m.width = 80
	m.height = 24

	next, cmd := m.Update(StreamErrorMsg{Err: errors.New("model timeout")})
	if cmd != nil {
		t.Fatal("StreamErrorMsg should not return follow-up command")
	}
	updated := next.(Model)

	view := viewContent(updated)
	if !strings.Contains(view, "model timeout") {
		t.Fatalf("View should show stream error:\n%s", view)
	}
}

func TestJKTypeWhenInputNotEmpty(t *testing.T) {
	m := New()
	m.input = "a"

	updated := applyKey(m, tea.KeyPressMsg{Code: 'j'})
	if updated.input != "aj" {
		t.Fatalf("input = %q, want aj", updated.input)
	}

	updated = applyKey(updated, tea.KeyPressMsg{Code: 'k'})
	if updated.input != "ajk" {
		t.Fatalf("input = %q, want ajk", updated.input)
	}
}

func TestCtrlCInterruptsBusyBeforeQuit(t *testing.T) {
	m := NewWithRunner(&stubRunner{})
	m.busy = true
	m.streamCh = make(chan any, 1)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("Ctrl+C should return quit command")
	}
	updated := next.(Model)
	if updated.busy {
		t.Fatal("busy should be false after Ctrl+C")
	}
}
