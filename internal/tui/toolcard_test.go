package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/wsx864321/coding-agent/internal/event"
)

func stripANSI(s string) string {
	return ansi.Strip(s)
}

func TestRenderToolCardTruncatesLongArgs(t *testing.T) {
	long := strings.Repeat("x", 200)
	card := renderToolCard("Read", `{"path":"`+long+`"}`, 60)
	if !strings.Contains(card, "●") || !strings.Contains(card, "Read") {
		t.Fatalf("unexpected card: %s", card)
	}
	if runewidth.StringWidth(stripANSI(card)) > 80 {
		t.Fatal("card too wide")
	}
}

func TestRenderToolCardShowsPrimaryArg(t *testing.T) {
	card := renderToolCard("Read", `{"path":"/tmp/foo.go"}`, 80)
	plain := stripANSI(card)
	if !strings.Contains(plain, "Read") {
		t.Fatalf("missing tool name: %s", plain)
	}
	if !strings.Contains(plain, "/tmp/foo.go") {
		t.Fatalf("missing path arg: %s", plain)
	}
}

func TestRenderToolOutputShort(t *testing.T) {
	result := "line1\nline2\nline3"
	out := renderToolOutput(result, toolOutputCollapseLines)
	if !strings.Contains(out, "line1") || !strings.Contains(out, "line3") {
		t.Fatalf("missing lines: %s", out)
	}
	if strings.Contains(out, "collapsed") {
		t.Fatalf("short output should not collapse: %s", out)
	}
}

func TestRenderToolOutputCollapsesLong(t *testing.T) {
	var lines []string
	for i := 1; i <= 12; i++ {
		lines = append(lines, fmt.Sprintf("output-line-%02d", i))
	}
	result := strings.Join(lines, "\n")
	out := renderToolOutput(result, toolOutputCollapseLines)
	if !strings.Contains(out, "output-line-01") {
		t.Fatalf("missing visible lines: %s", out)
	}
	if !strings.Contains(out, "collapsed") {
		t.Fatalf("expected collapse summary: %s", out)
	}
	if strings.Contains(out, "output-line-12") {
		t.Fatalf("line 12 should be hidden: %s", out)
	}
}

func TestUpdateToolStartSetsStatusLabel(t *testing.T) {
	m := New()
	m.busy = true
	m.width = 80

	next, _ := m.Update(event.Event{
		Kind:     event.ToolDispatch,
		ToolName: "Read",
		ToolArgs: `{"path":"x"}`,
	})
	updated := next.(Model)
	if updated.statusLabel != "running Read..." {
		t.Fatalf("statusLabel = %q, want %q", updated.statusLabel, "running Read...")
	}
}

func TestUpdateToolEndAppendsToolEntries(t *testing.T) {
	m := New()
	m.busy = true
	m.width = 80
	m.pendingToolName = "Read"
	m.pendingToolArgs = `{"path":"main.go"}`
	m = m.appendEntry(TranscriptEntry{Kind: EntryAssistantChunk, Raw: "thinking..."})

	next, _ := m.Update(event.Event{
		Kind:       event.ToolResult,
		ToolName:   "Read",
		ToolOutput: "file contents",
	})
	updated := next.(Model)
	if updated.statusLabel != "thinking" {
		t.Fatalf("statusLabel = %q, want thinking", updated.statusLabel)
	}

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
			if !strings.Contains(e.Content, "file contents") {
				t.Fatalf("tool output missing result: %s", e.Content)
			}
		}
	}
	if !foundCard || !foundOutput {
		t.Fatalf("transcript missing tool entries: card=%v output=%v", foundCard, foundOutput)
	}
}
