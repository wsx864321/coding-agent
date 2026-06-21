package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

type stubRunner struct {
	chunks []string
	err    error
	prompt string
}

func (s *stubRunner) RunTurn(_ context.Context, prompt string, emit StreamEmitter) error {
	s.prompt = prompt
	for _, c := range s.chunks {
		emit.OnChunk(c)
	}
	if s.err != nil {
		emit.OnError(s.err)
		return s.err
	}
	emit.OnDone()
	return nil
}

func TestSubmitWritesUserMessageAndInvokesRunner(t *testing.T) {
	runner := &stubRunner{chunks: []string{"ok"}}
	m := NewWithRunner(runner)
	m.input = "hello agent"

	updated, cmd := m.submit()
	if cmd == nil {
		t.Fatal("submit should start async stream command")
	}

	if updated.input != "" {
		t.Fatalf("input = %q, want cleared after submit", updated.input)
	}
	if !updated.busy {
		t.Fatal("busy should be true after submit")
	}
	if len(updated.messages) != 2 {
		t.Fatalf("messages = %d, want 2 (user + assistant placeholder)", len(updated.messages))
	}
	if updated.messages[0].Role != RoleUser || updated.messages[0].Content != "hello agent" {
		t.Fatalf("messages[0] = %+v, want user/hello agent", updated.messages[0])
	}
	if updated.messages[1].Role != RoleAssistant {
		t.Fatalf("messages[1].Role = %v, want assistant placeholder", updated.messages[1].Role)
	}

	msg := cmd()
	if msg == nil {
		t.Fatal("stream command returned nil msg")
	}
}

func TestStreamChunksAppendAssistantContent(t *testing.T) {
	ch := make(chan any, 1)
	m := New()
	m = m.withMessage(RoleUser, "q")
	m = m.withMessage(RoleAssistant, "")
	m.busy = true
	m.streamCh = ch

	next, cmd := m.Update(StreamChunkMsg{Text: "hel"})
	updated := next.(Model)
	if got := updated.messages[1].Content; got != "hel" {
		t.Fatalf("assistant content = %q, want %q", got, "hel")
	}
	if cmd == nil {
		t.Fatal("chunk update should continue listening for stream")
	}

	ch <- StreamChunkMsg{Text: "lo"}
	next, cmd = updated.Update(cmd())
	updated = next.(Model)
	if got := updated.messages[1].Content; got != "hello" {
		t.Fatalf("assistant content = %q, want %q", got, "hello")
	}
	if cmd == nil {
		t.Fatal("chunk update should continue listening for stream")
	}
}

func TestSubmitSetsProcessing(t *testing.T) {
	runner := &stubRunner{chunks: []string{"ok"}}
	m := NewWithRunner(runner)
	m.input = "hello"
	m.width = 80
	m.height = 24

	updated, cmd := m.submit()
	if cmd == nil {
		t.Fatal("submit should start async stream command")
	}
	if !updated.busy {
		t.Fatal("busy should be true after submit")
	}
	if updated.statusMsg != processingStatusMsg {
		t.Fatalf("statusMsg = %q, want %q", updated.statusMsg, processingStatusMsg)
	}
	view := updated.View()
	if !strings.Contains(view, processingStatusMsg) {
		t.Fatalf("View should show processing status:\n%s", view)
	}
	if !strings.Contains(view, "处理中") {
		t.Fatalf("View should show processing hint in input pane:\n%s", view)
	}
}

func TestStreamDoneClearsProcessing(t *testing.T) {
	m := New()
	m.busy = true
	m.statusMsg = processingStatusMsg
	m.width = 80
	m.height = 24

	next, cmd := m.Update(StreamDoneMsg{})
	updated := next.(Model)
	if updated.busy {
		t.Fatal("busy should be false after StreamDoneMsg")
	}
	if updated.statusMsg != "" {
		t.Fatalf("statusMsg = %q, want empty after done", updated.statusMsg)
	}
	if cmd != nil {
		t.Fatal("StreamDoneMsg should not return follow-up command")
	}
	view := updated.View()
	if strings.Contains(view, processingStatusMsg) {
		t.Fatalf("View should not show processing after done:\n%s", view)
	}
}

func TestStreamClosedResets(t *testing.T) {
	m := New()
	m.busy = true
	m.statusMsg = processingStatusMsg
	m.streamCh = make(chan any)
	m.width = 80
	m.height = 24

	next, cmd := m.Update(streamClosedMsg{})
	updated := next.(Model)
	if updated.busy {
		t.Fatal("busy should be false after stream closed")
	}
	if updated.statusMsg != "" {
		t.Fatalf("statusMsg = %q, want empty after stream closed", updated.statusMsg)
	}
	if updated.streamCh != nil {
		t.Fatal("streamCh should be nil after stream closed")
	}
	if cmd != nil {
		t.Fatal("streamClosedMsg should not return follow-up command")
	}

	next, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("next")})
	typed := next.(Model)
	if typed.input != "next" {
		t.Fatalf("input = %q, want next after stream closed", typed.input)
	}
}

func TestInterruptDoesNotAppend(t *testing.T) {
	m := NewWithRunner(&stubRunner{})
	m.busy = true
	m = m.withMessage(RoleUser, "q")
	m = m.withMessage(RoleAssistant, "partial")
	m.width = 80
	m.height = 24

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	interrupted := next.(Model)
	bodyBefore := interrupted.messages[len(interrupted.messages)-1].Content

	next, _ = interrupted.Update(StreamErrorMsg{Err: errors.New("context canceled")})
	updated := next.(Model)
	bodyAfter := updated.messages[len(updated.messages)-1].Content
	if bodyAfter != bodyBefore {
		t.Fatalf("message body changed after interrupted error: before=%q after=%q", bodyBefore, bodyAfter)
	}
	if updated.lastError != "" {
		t.Fatalf("lastError = %q, want empty for interrupted stream error", updated.lastError)
	}
	if updated.statusMsg != interruptedStatusMsg {
		t.Fatalf("statusMsg = %q, want %q", updated.statusMsg, interruptedStatusMsg)
	}
	view := updated.View()
	if strings.Contains(view, "context canceled") {
		t.Fatalf("interrupted error must not appear in view:\n%s", view)
	}
}

func TestStreamDoneClearsBusy(t *testing.T) {
	m := New()
	m.busy = true
	m.statusMsg = processingStatusMsg
	m = m.withMessage(RoleAssistant, "done")

	next, cmd := m.Update(StreamDoneMsg{})
	updated := next.(Model)
	if updated.busy {
		t.Fatal("busy should be false after StreamDoneMsg")
	}
	if updated.statusMsg != "" {
		t.Fatalf("statusMsg = %q, want empty after done", updated.statusMsg)
	}
	if cmd != nil {
		t.Fatal("StreamDoneMsg should not return follow-up command")
	}
}

func TestStreamErrorClearsBusy(t *testing.T) {
	m := New()
	m.busy = true
	m.statusMsg = processingStatusMsg

	next, cmd := m.Update(StreamErrorMsg{Err: errors.New("network down")})
	updated := next.(Model)
	if updated.busy {
		t.Fatal("busy should be false after StreamErrorMsg")
	}
	if updated.statusMsg != "" {
		t.Fatalf("statusMsg = %q, want empty after error", updated.statusMsg)
	}
	if updated.lastError != "network down" {
		t.Fatalf("lastError = %q, want network down", updated.lastError)
	}
	if cmd != nil {
		t.Fatal("StreamErrorMsg should not return follow-up command")
	}
}

func TestSubmitAfterRecoverableErrorStartsNewTurn(t *testing.T) {
	runner := &stubRunner{chunks: []string{"retry-ok"}}
	m := NewWithRunner(runner)
	m.busy = true
	m.lastError = "network down"

	next, _ := m.Update(StreamErrorMsg{Err: errors.New("network down")})
	afterErr := next.(Model)
	if afterErr.busy {
		t.Fatal("should be idle after recoverable error")
	}

	afterErr.input = "try again"
	next, cmd := afterErr.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter after error should start new turn")
	}
	updated := next.(Model)
	if updated.lastError != "" {
		t.Fatalf("lastError=%q, want cleared on new submit", updated.lastError)
	}
	if msg := cmd(); msg == nil {
		t.Fatal("stream command returned nil after error recovery submit")
	}
	if runner.prompt != "try again" {
		t.Fatalf("runner prompt=%q, want try again", runner.prompt)
	}
}

func TestEnterSubmitEndToEndWithStubRunner(t *testing.T) {
	runner := &stubRunner{chunks: []string{"A", "B"}}
	m := NewWithRunner(runner)
	m.input = "ping"

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter should return stream command")
	}
	updated := next.(Model)

	msg := cmd()
	if msg == nil {
		t.Fatal("stream command returned nil")
	}
	if runner.prompt != "ping" {
		t.Fatalf("runner prompt = %q, want ping", runner.prompt)
	}

	for {
		next, cmd = updated.Update(msg)
		updated = next.(Model)
		if _, ok := msg.(StreamDoneMsg); ok {
			break
		}
		if _, ok := msg.(StreamErrorMsg); ok {
			t.Fatal("unexpected stream error")
		}
		if cmd == nil {
			t.Fatal("expected follow-up command before done")
		}
		msg = cmd()
		if msg == nil {
			t.Fatal("stream command returned nil")
		}
	}

	if got := updated.messages[len(updated.messages)-1].Content; got != "AB" {
		t.Fatalf("assistant content = %q, want AB", got)
	}
	if updated.busy {
		t.Fatal("busy should be false after stream completes")
	}
}
