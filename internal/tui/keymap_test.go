package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestScrollWithKAndJ(t *testing.T) {
	m := New()
	m.width = 40
	m.height = 12
	m.messages = longMessageHistory()
	m = m.clampScrollToBottom()
	bottom := m.scrollOffset

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	scrolledUp := next.(Model)
	if scrolledUp.scrollOffset >= bottom {
		t.Fatalf("scrollOffset = %d, want < %d after k", scrolledUp.scrollOffset, bottom)
	}

	next, _ = scrolledUp.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	scrolledDown := next.(Model)
	if scrolledDown.scrollOffset <= scrolledUp.scrollOffset {
		t.Fatalf("scrollOffset = %d, want > %d after j", scrolledDown.scrollOffset, scrolledUp.scrollOffset)
	}
}

func TestEscInterrupt(t *testing.T) {
	m := NewWithRunner(&stubRunner{chunks: []string{"partial"}})
	m.busy = true
	m.statusMsg = processingStatusMsg
	m.streamCh = make(chan any, 1)
	m.width = 80
	m.height = 24

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatal("Esc interrupt should not return a command")
	}
	updated := next.(Model)
	if updated.busy {
		t.Fatal("busy should be false after Esc interrupt")
	}
	if updated.statusMsg != interruptedStatusMsg {
		t.Fatalf("statusMsg = %q, want %q", updated.statusMsg, interruptedStatusMsg)
	}
	if strings.Contains(updated.View(), processingStatusMsg) {
		t.Fatalf("View should not keep processing status after interrupt:\n%s", updated.View())
	}
	if !strings.Contains(updated.View(), interruptedStatusMsg) {
		t.Fatalf("View should show interrupted feedback:\n%s", updated.View())
	}
}

func TestEscInterruptsBusyTurn(t *testing.T) {
	m := NewWithRunner(&stubRunner{chunks: []string{"partial"}})
	m.busy = true
	m.streamCh = make(chan any, 1)
	m.width = 80
	m.height = 24

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		t.Fatal("Esc interrupt should not return a command")
	}
	updated := next.(Model)
	if updated.busy {
		t.Fatal("busy should be false after Esc interrupt")
	}
	if !strings.Contains(updated.View(), "已中断") {
		t.Fatalf("View should show interrupted feedback:\n%s", updated.View())
	}
}

func TestBusyBlocksInputEditing(t *testing.T) {
	m := New()
	m.busy = true
	m.input = ""

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	updated := next.(Model)
	if updated.input != "" {
		t.Fatalf("input=%q, want empty while busy", updated.input)
	}

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	updated = next.(Model)
	if updated.input != "" {
		t.Fatalf("backspace while busy should not change input")
	}
}

func TestRecoverableErrorHelpAffordance(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.lastError = "timeout"

	view := m.View()
	if !strings.Contains(view, "可继续输入") {
		t.Fatalf("error state should hint next action:\n%s", view)
	}
}

func TestEscInterruptAllowsInputAfter(t *testing.T) {
	m := NewWithRunner(&stubRunner{})
	m.busy = true

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated := next.(Model)

	next, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("next")})
	if cmd != nil {
		t.Fatal("typing after interrupt should not return a command")
	}
	typed := next.(Model)
	if typed.input != "next" {
		t.Fatalf("input = %q, want next after interrupt", typed.input)
	}
}

func TestViewShowsErrorBanner(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.lastError = "network down"

	view := m.View()
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

	view := updated.View()
	if !strings.Contains(view, "model timeout") {
		t.Fatalf("View should show stream error:\n%s", view)
	}
}

func TestJKTypeWhenInputNotEmpty(t *testing.T) {
	m := New()
	m.input = "a"

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	updated := next.(Model)
	if updated.input != "aj" {
		t.Fatalf("input = %q, want aj", updated.input)
	}

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	updated = next.(Model)
	if updated.input != "ajk" {
		t.Fatalf("input = %q, want ajk", updated.input)
	}
}

func TestCtrlCInterruptsBusyBeforeQuit(t *testing.T) {
	m := NewWithRunner(&stubRunner{})
	m.busy = true
	m.streamCh = make(chan any, 1)

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("Ctrl+C should return quit command")
	}
	updated := next.(Model)
	if updated.busy {
		t.Fatal("busy should be false after Ctrl+C")
	}
}
