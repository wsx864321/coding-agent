package tui

import (
	"strings"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/wsx864321/coding-agent/internal/event"
)

func TestToolEventFlowUpdatesTranscript(t *testing.T) {
	ch := make(chan event.Event, 8)
	m := New()
	m.busy = true
	m.width = 80
	m.height = 24
	m.streamCh = ch
	m = m.appendEntry(TranscriptEntry{Kind: EntryAssistantChunk})
	m = m.syncLayout()

	next, cmd := m.Update(event.Event{
		Kind:     event.ToolDispatch,
		ToolName: "Read",
		ToolArgs: `{"path":"main.go"}`,
	})
	updated := next.(Model)
	if updated.statusLabel != "running Read..." {
		t.Fatalf("statusLabel = %q, want %q", updated.statusLabel, "running Read...")
	}
	view := viewContent(updated.syncLayout())
	if !strings.Contains(view, "running Read") {
		t.Fatalf("View should show running tool status:\n%s", view)
	}
	if cmd == nil {
		t.Fatal("ToolDispatch should schedule stream listener when streamCh is set")
	}

	ch <- event.Event{
		Kind:       event.ToolResult,
		ToolName:   "Read",
		ToolOutput: "package main\n",
	}
	got := cmd()
	if got == nil {
		t.Fatal("stream command returned nil message")
	}
	next, _ = updated.Update(got)
	updated = next.(Model)

	foundCard, foundOutput := false, false
	for _, e := range updated.transcript {
		switch e.Kind {
		case EntryToolCard:
			foundCard = true
			if !strings.Contains(stripANSI(e.Content), "Read") {
				t.Fatalf("tool card missing name: %s", e.Content)
			}
		case EntryToolOutput:
			foundOutput = true
			if !strings.Contains(e.Content, "package main") {
				t.Fatalf("tool output missing result: %s", e.Content)
			}
		}
	}
	if !foundCard || !foundOutput {
		t.Fatalf("transcript missing tool entries: card=%v output=%v", foundCard, foundOutput)
	}
	if updated.statusLabel != "thinking" {
		t.Fatalf("statusLabel = %q, want thinking after tool end", updated.statusLabel)
	}
}

func TestApprovalFlowEndToEnd(t *testing.T) {
	t.Run("approve with y", func(t *testing.T) {
		var calls atomic.Int32
		m := New()
		m.busy = true
		m.width = 80
		m.height = 24
		m = m.syncLayout()

		next, cmd := m.Update(event.Event{
			Kind:            event.ApprovalRequest,
			ApprovalName:    "bash",
			ApprovalArgs:    map[string]any{"command": "rm -rf /tmp/test"},
			ApprovalRespond: func(ok bool) { if ok { calls.Add(1) } },
		})
		if cmd != nil {
			t.Fatal("ApprovalRequest without streamCh should not return a command")
		}
		updated := next.(Model)
		if updated.approval == nil {
			t.Fatal("approval modal should be active")
		}

		view := viewContent(updated)
		for _, want := range []string{"Allow", "[y]es", "[n]o"} {
			if !strings.Contains(view, want) {
				t.Fatalf("View missing approval banner %q:\n%s", want, view)
			}
		}

		next, _ = updated.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
		updated = next.(Model)
		if updated.approval != nil {
			t.Fatal("approval should be cleared after y")
		}
		if calls.Load() != 1 {
			t.Fatalf("respond(true) calls = %d, want 1", calls.Load())
		}
	})

	t.Run("reject with n", func(t *testing.T) {
		var rejected atomic.Bool
		m := New()
		m.busy = true
		m.width = 80
		m.height = 24

		next, _ := m.Update(event.Event{
			Kind:            event.ApprovalRequest,
			ApprovalName:    "bash",
			ApprovalArgs:    map[string]any{"command": "rm -rf /tmp/test"},
			ApprovalRespond: func(ok bool) { if !ok { rejected.Store(true) } },
		})
		updated := next.(Model)

		next, _ = updated.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
		updated = next.(Model)
		if updated.approval != nil {
			t.Fatal("approval should be cleared after n")
		}
		if !rejected.Load() {
			t.Fatal("respond(false) was not called")
		}
	})
}

func TestCJKMarkdownIntegrationInView(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	md := "**你好**，这是 *Markdown* 渲染测试"
	m = m.appendEntry(TranscriptEntry{Kind: EntryAssistantChunk, Raw: md})
	m = m.syncLayout()
	m = m.syncViewportContent()

	view := viewContent(m)
	for _, want := range []string{"你好", "Markdown", "> "} {
		if !strings.Contains(stripANSI(view), want) {
			t.Errorf("View missing %q:\n%s", want, view)
		}
	}
	if !hasANSI(m.transcript[0].Content) {
		t.Fatalf("expected ANSI styling in assistant entry: %s", m.transcript[0].Content)
	}
}

func TestStreamFlushIntegration(t *testing.T) {
	ch := make(chan event.Event, 8)
	m := New()
	m.width = 80
	m.height = 24
	m.busy = true
	m.streamCh = ch
	m = m.appendEntry(TranscriptEntry{Kind: EntryAssistantChunk})
	m = m.syncLayout()

	next, cmd := m.Update(event.Event{Kind: event.Text, Text: "第一段内容"})
	updated := next.(Model)
	if updated.pending.String() != "第一段内容" {
		t.Fatalf("pending = %q, want 第一段内容", updated.pending.String())
	}
	if len(updated.transcript) == 0 || updated.transcript[0].Raw != "" {
		t.Fatalf("assistant raw = %q, want empty before paragraph boundary", updated.transcript[0].Raw)
	}

	var nextCmd tea.Cmd
	ch <- event.Event{Kind: event.Text, Text: "。\n\n"}
	got := cmd()
	if got == nil {
		t.Fatal("stream command returned nil message")
	}
	next, nextCmd = updated.Update(got)
	updated = next.(Model)
	if !strings.Contains(updated.transcript[0].Raw, "第一段内容") {
		t.Fatalf("assistant raw = %q, want first paragraph flushed", updated.transcript[0].Raw)
	}

	ch <- event.Event{Kind: event.Text, Text: "**第二段** bold\n\n"}
	got = nextCmd()
	if got == nil {
		t.Fatal("stream command returned nil message")
	}
	next, nextCmd = updated.Update(got)
	updated = next.(Model)
	if !strings.Contains(updated.transcript[0].Raw, "第二段") {
		t.Fatalf("assistant raw = %q, want second paragraph", updated.transcript[0].Raw)
	}

	ch <- event.Event{Kind: event.TurnDone}
	got = nextCmd()
	if got == nil {
		t.Fatal("stream command returned nil message")
	}
	next, nextCmd = updated.Update(got)
	updated = next.(Model)
	if nextCmd != nil {
		t.Fatal("TurnDone should not return follow-up command")
	}
	if updated.busy {
		t.Fatal("busy should be false after TurnDone")
	}
	if updated.pending.Len() != 0 {
		t.Fatalf("pending should be empty after done, got %q", updated.pending.String())
	}

	view := viewContent(updated.syncLayout())
	for _, want := range []string{"第一段", "第二段", "> "} {
		if !strings.Contains(stripANSI(view), want) {
			t.Errorf("View missing %q:\n%s", want, view)
		}
	}
}
