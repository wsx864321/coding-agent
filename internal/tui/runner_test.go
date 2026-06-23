package tui

import (
	"context"
	"errors"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/wsx864321/coding-agent/internal/event"
)

type stubRunner struct {
	chunks []string
	err    error
	prompt string
	sink   *TuiSink
}

func (s *stubRunner) RunTurn(_ context.Context, prompt string) error {
	s.prompt = prompt
	for _, c := range s.chunks {
		if s.sink != nil {
			s.sink.Emit(event.Event{Kind: event.Text, Text: c})
		}
	}
	if s.err != nil {
		if s.sink != nil {
			s.sink.Emit(event.Event{Kind: event.TurnDone, Err: s.err})
		}
		return s.err
	}
	if s.sink != nil {
		s.sink.Emit(event.Event{Kind: event.TurnDone})
	}
	return nil
}

func newStubModel(chunks []string) (Model, *stubRunner) {
	sink := &TuiSink{}
	runner := &stubRunner{chunks: chunks, sink: sink}
	return NewWithRunner(runner, sink), runner
}

func TestSubmitWritesUserMessageAndInvokesRunner(t *testing.T) {
	m, _ := newStubModel([]string{"ok"})
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
	if len(updated.transcript) != 2 {
		t.Fatalf("transcript = %d, want 2 (user + assistant placeholder)", len(updated.transcript))
	}
	if updated.transcript[0].Kind != EntryUserMessage || updated.transcript[0].Raw != "hello agent" {
		t.Fatalf("transcript[0] = %+v, want user/hello agent", updated.transcript[0])
	}
	if updated.transcript[1].Kind != EntryAssistantChunk {
		t.Fatalf("transcript[1].Kind = %v, want assistant placeholder", updated.transcript[1].Kind)
	}

	msg := cmd()
	if msg == nil {
		t.Fatal("stream command returned nil msg")
	}
}

func TestStreamChunksAppendAssistantContent(t *testing.T) {
	ch := make(chan event.Event, 1)
	m := New()
	m.width = 80
	m = m.appendUserMessage("q")
	m = m.appendEntry(TranscriptEntry{Kind: EntryAssistantChunk})
	m.busy = true
	m.streamCh = ch

	// No paragraph boundary yet — content stays in pending buffer.
	next, cmd := m.Update(event.Event{Kind: event.Text, Text: "hel"})
	updated := next.(Model)
	if got := updated.transcript[1].Raw; got != "" {
		t.Fatalf("assistant raw = %q, want empty before boundary", got)
	}
	if updated.pending.String() != "hel" {
		t.Fatalf("pending = %q, want hel", updated.pending.String())
	}
	if cmd == nil {
		t.Fatal("chunk update should continue listening for stream")
	}

	ch <- event.Event{Kind: event.Text, Text: "lo\n\n"}
	next, cmd = updated.Update(cmd())
	updated = next.(Model)
	if got := updated.transcript[1].Raw; got != "hello\n" {
		t.Fatalf("assistant raw = %q, want hello\\n after paragraph flush", got)
	}
	if cmd == nil {
		t.Fatal("chunk update should continue listening for stream")
	}
}

func TestStreamDoneClearsBusy(t *testing.T) {
	m := New()
	m.busy = true
	m = m.appendEntry(TranscriptEntry{Kind: EntryAssistantChunk, Raw: "done"})

	next, cmd := m.Update(event.Event{Kind: event.TurnDone})
	updated := next.(Model)
	if updated.busy {
		t.Fatal("busy should be false after TurnDone")
	}
	if cmd != nil {
		t.Fatal("TurnDone should not return follow-up command")
	}
}

func TestStreamErrorClearsBusyAndStoresError(t *testing.T) {
	m := New()
	m.busy = true

	next, cmd := m.Update(event.Event{Kind: event.TurnDone, Err: errors.New("network down")})
	updated := next.(Model)
	if updated.busy {
		t.Fatal("busy should be false after TurnDone with error")
	}
	if updated.lastError != "network down" {
		t.Fatalf("lastError = %q, want network down", updated.lastError)
	}
	if cmd != nil {
		t.Fatal("TurnDone should not return follow-up command")
	}
}

func TestEnterSubmitEndToEndWithStubRunner(t *testing.T) {
	m, _ := newStubModel([]string{"A", "B"})
	m.textarea.SetValue("ping")

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter should return stream command")
	}
	updated := next.(Model)
	if len(updated.transcript) != 2 || updated.transcript[0].Raw != "ping" {
		t.Fatalf("transcript = %+v, want user ping committed", updated.transcript)
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
		if ev, ok := msg.(event.Event); ok && ev.Kind == event.TurnDone {
			if ev.Err != nil {
				t.Fatal("unexpected stream error")
			}
			break
		}
		if cmd == nil {
			t.Fatal("expected follow-up command before done")
		}
		msg = cmd()
		if msg == nil {
			t.Fatal("stream command returned nil")
		}
	}

	if got := updated.transcript[len(updated.transcript)-1].Raw; got != "AB" {
		t.Fatalf("assistant raw = %q, want AB", got)
	}
	if updated.busy {
		t.Fatal("busy should be false after stream completes")
	}
}
