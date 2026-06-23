package tui

import (
	"context"
	"errors"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
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
	m.textarea.SetValue("hello agent")

	updated, cmd := m.submit()
	if cmd == nil {
		t.Fatal("submit should start async stream command")
	}

	if updated.textarea.Value() != "" {
		t.Fatalf("textarea = %q, want cleared after submit", updated.textarea.Value())
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

func TestStreamDoneClearsBusy(t *testing.T) {
	m := New()
	m.busy = true
	m = m.withMessage(RoleAssistant, "done")

	next, cmd := m.Update(StreamDoneMsg{})
	updated := next.(Model)
	if updated.busy {
		t.Fatal("busy should be false after StreamDoneMsg")
	}
	if cmd != nil {
		t.Fatal("StreamDoneMsg should not return follow-up command")
	}
}

func TestStreamErrorClearsBusyAndStoresError(t *testing.T) {
	m := New()
	m.busy = true

	next, cmd := m.Update(StreamErrorMsg{Err: errors.New("network down")})
	updated := next.(Model)
	if updated.busy {
		t.Fatal("busy should be false after StreamErrorMsg")
	}
	if updated.lastError != "network down" {
		t.Fatalf("lastError = %q, want network down", updated.lastError)
	}
	if cmd != nil {
		t.Fatal("StreamErrorMsg should not return follow-up command")
	}
}

func TestEnterSubmitEndToEndWithStubRunner(t *testing.T) {
	runner := &stubRunner{chunks: []string{"A", "B"}}
	m := NewWithRunner(runner)
	m.textarea.SetValue("ping")

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter should return stream command")
	}
	updated := next.(Model)
	if len(updated.messages) != 2 || updated.messages[0].Content != "ping" {
		t.Fatalf("messages = %+v, want user ping committed", updated.messages)
	}

	msg := cmd()
	if msg == nil {
		t.Fatal("stream command returned nil")
	}

	for {
		if tick, ok := msg.(spinner.TickMsg); ok {
			next, cmd = updated.Update(tick)
			updated = next.(Model)
			if cmd == nil {
				t.Fatal("spinner tick should schedule next frame while busy")
			}
			msg = cmd()
			continue
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				if sub := c(); sub != nil {
					msg = sub
					goto dispatch
				}
			}
			t.Fatal("batch command returned no messages")
		}
	dispatch:
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
