package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/wsx864321/coding-agent/internal/event"
)

func TestScrollWithKAndJ(t *testing.T) {
	m := prepareScrollModel()
	m.viewport.GotoBottom()
	bottom := m.viewport.YOffset()

	next, _ := m.Update(tea.KeyPressMsg{Code: 'k'})
	scrolledUp := next.(Model)
	if scrolledUp.viewport.YOffset() >= bottom {
		t.Fatalf("YOffset = %d, want < %d after k", scrolledUp.viewport.YOffset(), bottom)
	}

	next, _ = scrolledUp.Update(tea.KeyPressMsg{Code: 'j'})
	scrolledDown := next.(Model)
	if scrolledDown.viewport.YOffset() <= scrolledUp.viewport.YOffset() {
		t.Fatalf("YOffset = %d, want > %d after j", scrolledDown.viewport.YOffset(), scrolledUp.viewport.YOffset())
	}
}

func TestEscInterruptsBusyTurn(t *testing.T) {
	m := NewWithRunner(&stubRunner{chunks: []string{"partial"}}, nil)
	m.busy = true
	m.streamCh = make(chan event.Event, 1)
	m.width = 80
	m.height = 24
	m = m.syncLayout()

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
	m := NewWithRunner(&stubRunner{}, nil)
	m.busy = true

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	updated := next.(Model)

	typed := typeText(updated, "next")
	if typed.textarea.Value() != "next" {
		t.Fatalf("textarea = %q, want next after interrupt", typed.textarea.Value())
	}
}

func TestViewShowsErrorBanner(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.lastError = "network down"
	m = m.syncLayout()

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

	next, cmd := m.Update(event.Event{Kind: event.TurnDone, Err: errors.New("model timeout")})
	if cmd != nil {
		t.Fatal("TurnDone with error should not return follow-up command")
	}
	updated := next.(Model)
	updated = updated.syncLayout()

	view := viewContent(updated)
	if !strings.Contains(view, "model timeout") {
		t.Fatalf("View should show stream error:\n%s", view)
	}
}

func TestJKTypeWhenInputNotEmpty(t *testing.T) {
	m := New()
	m.textarea.SetValue("a")

	updated := applyKey(m, tea.KeyPressMsg{Code: 'j', Text: "j"})
	if updated.textarea.Value() != "aj" {
		t.Fatalf("textarea = %q, want aj", updated.textarea.Value())
	}

	updated = applyKey(updated, tea.KeyPressMsg{Code: 'k', Text: "k"})
	if updated.textarea.Value() != "ajk" {
		t.Fatalf("textarea = %q, want ajk", updated.textarea.Value())
	}
}

func TestCtrlCInterruptsBusyBeforeQuit(t *testing.T) {
	m := NewWithRunner(&stubRunner{}, nil)
	m.busy = true
	m.streamCh = make(chan event.Event, 1)

	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("Ctrl+C should return quit command")
	}
	updated := next.(Model)
	if updated.busy {
		t.Fatal("busy should be false after Ctrl+C")
	}
}
