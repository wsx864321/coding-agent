package tui

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestApprovalRequestMsgEntersModal(t *testing.T) {
	m := New()
	m.busy = true
	msg := ApprovalRequestMsg{
		Name:    "write_file",
		Args:    map[string]any{"path": "config.yaml"},
		Respond: func(bool) {},
	}

	next, cmd := m.Update(msg)
	if cmd != nil {
		t.Fatal("ApprovalRequestMsg should not return a command")
	}
	updated := next.(Model)
	if updated.approval == nil {
		t.Fatal("approval should be set after ApprovalRequestMsg")
	}
	if updated.approval.toolName != "write_file" {
		t.Fatalf("toolName = %q, want write_file", updated.approval.toolName)
	}
}

func TestApprovalYRespondsTrueOnce(t *testing.T) {
	var calls atomic.Int32
	m := New()
	m.busy = true
	m.approval = &pendingApproval{
		toolName: "write_file",
		args:     map[string]any{"path": "config.yaml"},
		respond: func(ok bool) {
			if ok {
				calls.Add(1)
			}
		},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	updated := next.(Model)
	if updated.approval != nil {
		t.Fatal("approval should be cleared after y")
	}
	if calls.Load() != 1 {
		t.Fatalf("respond(true) calls = %d, want 1", calls.Load())
	}

	next, _ = updated.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if calls.Load() != 1 {
		t.Fatalf("respond should not be called again, got %d calls", calls.Load())
	}
}

func TestApprovalNRespondsFalseOnce(t *testing.T) {
	var gotFalse atomic.Bool
	m := New()
	m.busy = true
	m.approval = &pendingApproval{
		toolName: "bash",
		respond: func(ok bool) {
			if !ok {
				gotFalse.Store(true)
			}
		},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	updated := next.(Model)
	if updated.approval != nil {
		t.Fatal("approval should be cleared after n")
	}
	if !gotFalse.Load() {
		t.Fatal("respond(false) was not called")
	}
}

func TestApprovalKeysDoNotReachTextarea(t *testing.T) {
	m := New()
	m.textarea.SetValue("")
	m.approval = &pendingApproval{
		toolName: "write_file",
		respond:  func(bool) {},
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	updated := next.(Model)
	if updated.textarea.Value() != "" {
		t.Fatalf("textarea = %q, want empty during approval", updated.textarea.Value())
	}
}

func TestRenderApprovalBanner(t *testing.T) {
	banner := renderApprovalBanner(pendingApproval{
		toolName: "write_file",
		args:     map[string]any{"path": "config.yaml"},
	}, 80)
	for _, want := range []string{"Allow", "Write", "config.yaml", "[y]es", "[n]o"} {
		if !strings.Contains(banner, want) {
			t.Errorf("banner missing %q: %s", want, banner)
		}
	}
}

func TestEscDuringApprovalCancelsTurn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := NewWithRunner(&stubRunner{})
	m.busy = true
	m.turnCancel = cancel
	m.approval = &pendingApproval{
		toolName: "write_file",
		respond:  func(bool) {},
	}
	m.width = 80
	m.height = 24
	m = m.syncLayout()

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Fatal("Esc during approval should not return a command")
	}
	updated := next.(Model)
	if updated.approval != nil {
		t.Fatal("approval should be cleared after Esc")
	}
	if updated.busy {
		t.Fatal("busy should be false after Esc interrupt")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("turn context should be cancelled on Esc")
	}
}
