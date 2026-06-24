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

// --- Task 23: diffMaxLines and diff block collapse ---

func TestIsDiffOutputDetectsUnifiedDiff(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "full unified diff",
			output: "diff --git a/file.go b/file.go\n--- a/file.go\n+++ b/file.go\n@@ -1,3 +1,4 @@\n context\n+added\n-removed\n context",
			want:   true,
		},
		{
			name:   "minimal diff with hunk header and changes",
			output: "@@ -10,5 +10,6 @@\n unchanged\n+new line\n-old line\n unchanged",
			want:   true,
		},
		{
			name:   "plain text no diff markers",
			output: "hello world\nthis is output\nno diff here",
			want:   false,
		},
		{
			name:   "empty string",
			output: "",
			want:   false,
		},
		{
			name:   "code with plus signs but no hunk header",
			output: "x := 1\ny := x + 2\nz := 3",
			want:   false,
		},
		{
			name:   "only hunk header no change lines",
			output: "@@ -1,1 +1,1 @@\njust context\nmore context",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDiffOutput(tt.output)
			if got != tt.want {
				t.Fatalf("isDiffOutput() = %v, want %v for input:\n%s", got, tt.want, tt.output)
			}
		})
	}
}

func TestRenderDiffOutputCollapsesAtMaxLines(t *testing.T) {
	// Build a diff with 20 lines.
	var lines []string
	lines = append(lines, "--- a/file.go", "+++ b/file.go", "@@ -1,10 +1,12 @@")
	for i := 1; i <= 17; i++ {
		if i%3 == 0 {
			lines = append(lines, fmt.Sprintf("+added line %02d", i))
		} else if i%3 == 1 {
			lines = append(lines, fmt.Sprintf("-removed line %02d", i))
		} else {
			lines = append(lines, fmt.Sprintf("  context line %02d", i))
		}
	}
	diffOutput := strings.Join(lines, "\n")

	// With diffMaxLines = 5, should collapse and show summary.
	out := renderDiffOutput(diffOutput, 5)

	if !strings.Contains(out, "--- a/file.go") {
		t.Fatal("diff output should show first lines")
	}
	if !strings.Contains(out, "collapsed") {
		t.Fatal("diff output exceeding maxLines should show 'collapsed' summary")
	}
	// Line 17 should NOT be visible (beyond first 5 lines).
	if strings.Contains(out, "added line 17") || strings.Contains(out, "removed line 17") {
		t.Fatal("diff output should hide lines beyond maxLines")
	}
}

func TestRenderDiffOutputNoCollapseWhenUnderLimit(t *testing.T) {
	diffOutput := "--- a/file.go\n+++ b/file.go\n@@ -1,2 +1,3 @@\n unchanged\n+added\n unchanged"

	// With diffMaxLines = 10 (more than output), should NOT collapse.
	out := renderDiffOutput(diffOutput, 10)

	if !strings.Contains(out, "+added") {
		t.Fatal("short diff should show all lines")
	}
	if strings.Contains(out, "collapsed") {
		t.Fatal("short diff should NOT show 'collapsed'")
	}
}

func TestRenderDiffOutputZeroMaxLinesNoCollapse(t *testing.T) {
	// Build a diff with 30 lines.
	var lines []string
	lines = append(lines, "--- a/file.go", "+++ b/file.go", "@@ -1,15 +1,15 @@")
	for i := 1; i <= 27; i++ {
		lines = append(lines, fmt.Sprintf("  context line %02d", i))
	}
	diffOutput := strings.Join(lines, "\n")

	// With diffMaxLines = 0 (no limit), should show everything.
	out := renderDiffOutput(diffOutput, 0)

	if !strings.Contains(out, "context line 27") {
		t.Fatal("diffMaxLines=0 should show ALL lines without collapse")
	}
	if strings.Contains(out, "collapsed") {
		t.Fatal("diffMaxLines=0 should NOT collapse")
	}
}

func TestToolResultUsesDiffMaxLinesForDiffOutput(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true
	m.diffMaxLines = 3

	// Build a diff with 15 lines.
	var lines []string
	lines = append(lines, "--- a/file.go", "+++ b/file.go", "@@ -1,8 +1,8 @@")
	for i := 1; i <= 12; i++ {
		if i%2 == 0 {
			lines = append(lines, fmt.Sprintf("+added-%02d", i))
		} else {
			lines = append(lines, fmt.Sprintf("-removed-%02d", i))
		}
	}
	diffOutput := strings.Join(lines, "\n")

	next, _ := m.Update(event.Event{
		Kind:       event.ToolResult,
		ToolName:   "bash",
		ToolOutput: diffOutput,
		ToolCallID: "call_diff_001",
	})
	updated := next.(Model)

	// Find the EntryToolOutput entry.
	var toolOutputEntry *TranscriptEntry
	for i := range updated.transcript {
		if updated.transcript[i].Kind == EntryToolOutput {
			toolOutputEntry = &updated.transcript[i]
			break
		}
	}
	if toolOutputEntry == nil {
		t.Fatal("should have EntryToolOutput in transcript")
	}

	content := toolOutputEntry.Content
	// Should show first lines (diff header).
	if !strings.Contains(content, "--- a/file.go") {
		t.Fatal("diff output should show header lines")
	}
	// Should show collapsed summary because diffMaxLines=3 < 15 lines.
	if !strings.Contains(content, "collapsed") {
		t.Fatal("diff output exceeding diffMaxLines should show 'collapsed'")
	}
	// Line 12 should NOT be visible.
	if strings.Contains(content, "added-12") {
		t.Fatal("diff output should hide lines beyond diffMaxLines")
	}
}

func TestToolResultFallsBackToDefaultForNonDiffOutput(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true
	m.diffMaxLines = 2 // Set diffMaxLines low, but output is NOT a diff.

	// Non-diff output with 12 lines.
	var lines []string
	for i := 1; i <= 12; i++ {
		lines = append(lines, fmt.Sprintf("plain-line-%02d", i))
	}
	plainOutput := strings.Join(lines, "\n")

	next, _ := m.Update(event.Event{
		Kind:       event.ToolResult,
		ToolName:   "bash",
		ToolOutput: plainOutput,
		ToolCallID: "call_plain_001",
	})
	updated := next.(Model)

	// Find the EntryToolOutput entry.
	var toolOutputEntry *TranscriptEntry
	for i := range updated.transcript {
		if updated.transcript[i].Kind == EntryToolOutput {
			toolOutputEntry = &updated.transcript[i]
			break
		}
	}
	if toolOutputEntry == nil {
		t.Fatal("should have EntryToolOutput in transcript")
	}

	content := toolOutputEntry.Content
	// Should show first visible lines (from toolOutputCollapseLines = 8).
	if !strings.Contains(content, "plain-line-01") {
		t.Fatal("plain output should show first line")
	}
	// Should show collapsed summary (12 > 8).
	if !strings.Contains(content, "collapsed") {
		t.Fatal("plain output exceeding 8 lines should show 'collapsed'")
	}
	// Should show line 8 (within first 8).
	if !strings.Contains(content, "plain-line-08") {
		t.Fatal("plain output should show line 8 (within default collapse limit)")
	}
	// Should NOT show line 12.
	if strings.Contains(content, "plain-line-12") {
		t.Fatal("plain output should hide lines beyond default collapse")
	}
}
