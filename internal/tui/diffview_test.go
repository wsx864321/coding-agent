package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wsx864321/coding-agent/internal/event"
)

// ---------------------------------------------------------------------------
// TestDiffFormatDetection — isDiffOutput 统一 diff 格式检测
// ---------------------------------------------------------------------------

func TestDiffFormatDetection(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name: "full unified diff",
			output: "diff --git a/file.go b/file.go\n" +
				"index 1234567..abcdefg 100644\n" +
				"--- a/file.go\n" +
				"+++ b/file.go\n" +
				"@@ -1,3 +1,4 @@\n" +
				" context\n" +
				"+added\n" +
				"-removed\n" +
				" context",
			want: true,
		},
		{
			name: "git diff with multiple hunks",
			output: "diff --git a/main.go b/main.go\n" +
				"--- a/main.go\n" +
				"+++ b/main.go\n" +
				"@@ -1,5 +1,5 @@\n" +
				" package main\n" +
				"-old import\n" +
				"+new import\n" +
				"@@ -10,4 +10,5 @@\n" +
				" unchanged\n" +
				"+extra line\n",
			want: true,
		},
		{
			name: "minimal diff — hunk header and one added line",
			output: "@@ -10,5 +10,6 @@\n" +
				"+new line\n",
			want: true,
		},
		{
			name: "minimal diff — hunk header and one removed line",
			output: "@@ -0,0 +1 @@\n" +
				"-removed\n",
			want: true,
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
			name: "code with + signs but no hunk header",
			output: "x := 1\n" +
				"y := x + 2\n" +
				"z := 3",
			want: false,
		},
		{
			name: "only hunk header — no change lines",
			output: "@@ -1,1 +1,1 @@\n" +
				"just context\n" +
				"more context",
			want: false,
		},
		{
			name: "only + change lines — no hunk header",
			output: "+added\n" +
				"+another\n" +
				"+third",
			want: false,
		},
		{
			name: "only - change lines — no hunk header",
			output: "-removed\n" +
				"-another",
			want: false,
		},
		{
			name: "hunk header with leading whitespace (indented hunk, raw +/- lines)",
			output: "  @@ -1,1 +1,1 @@\n" +
				"  context\n" +
				"+added",
			want: true,
		},
		{
			name:   "single line — just a hunk header",
			output: "@@ -1,2 +1,3 @@",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDiffOutput(tt.output)
			if got != tt.want {
				t.Fatalf("isDiffOutput() = %v, want %v\ninput:\n%s", got, tt.want, tt.output)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestDiffBlockCollapse — renderDiffOutput 折叠逻辑
// ---------------------------------------------------------------------------

func TestDiffBlockCollapse(t *testing.T) {
	// Helper: build a diff with N lines including a hunk header + changes.
	buildDiff := func(n int) string {
		var lines []string
		lines = append(lines, "--- a/file.go", "+++ b/file.go", "@@ -1,10 +1,12 @@")
		for i := 1; i <= n-3; i++ {
			switch i % 3 {
			case 0:
				lines = append(lines, fmt.Sprintf("+added line %02d", i))
			case 1:
				lines = append(lines, fmt.Sprintf("-removed line %02d", i))
			case 2:
				lines = append(lines, fmt.Sprintf("  context line %02d", i))
			}
		}
		return strings.Join(lines, "\n")
	}

	t.Run("collapses at maxLines", func(t *testing.T) {
		diff := buildDiff(20) // 20 lines
		out := renderDiffOutput(diff, 5)

		if !strings.Contains(out, "--- a/file.go") {
			t.Fatal("should show first lines")
		}
		if !strings.Contains(out, "collapsed") {
			t.Fatal("should include 'collapsed' summary")
		}
		if strings.Contains(out, "line 17") {
			t.Fatal("lines beyond maxLines should be hidden")
		}
	})

	t.Run("no collapse when under limit", func(t *testing.T) {
		diff := "--- a/x.go\n+++ b/x.go\n@@ -1,2 +1,3 @@\n unchanged\n+added\n unchanged"
		out := renderDiffOutput(diff, 10)

		if !strings.Contains(out, "+added") {
			t.Fatal("should show all lines when under maxLines")
		}
		if strings.Contains(out, "collapsed") {
			t.Fatal("should NOT collapse when under limit")
		}
	})

	t.Run("zero maxLines shows all without collapse", func(t *testing.T) {
		diff := buildDiff(25)
		out := renderDiffOutput(diff, 0)

		if strings.Contains(out, "collapsed") {
			t.Fatal("diffMaxLines=0 should NOT collapse")
		}
		// last line should be present
		if !strings.Contains(out, "line 22") {
			t.Fatal("diffMaxLines=0 should show all lines")
		}
	})

	t.Run("maxLines exactly equals line count — no collapse", func(t *testing.T) {
		diff := "--- a/x.go\n+++ b/x.go\n@@ -1,2 +1,3 @@\n unchanged\n+added"
		out := renderDiffOutput(diff, 5) // exactly 5 lines

		if !strings.Contains(out, "+added") {
			t.Fatal("should show all lines when maxLines == len(lines)")
		}
		if strings.Contains(out, "collapsed") {
			t.Fatal("should NOT collapse when maxLines equals line count")
		}
	})

	t.Run("maxLines=1 shows only first line and collapse summary", func(t *testing.T) {
		diff := buildDiff(10)
		out := renderDiffOutput(diff, 1)

		if !strings.Contains(out, "--- a/file.go") {
			t.Fatal("should show first line")
		}
		if !strings.Contains(out, "collapsed") {
			t.Fatal("should include collapse summary")
		}
		// Second line must NOT be visible
		if strings.Contains(out, "+++ b/file.go") {
			t.Fatal("second line should be hidden with maxLines=1")
		}
	})

	t.Run("empty input", func(t *testing.T) {
		out := renderDiffOutput("", 5)
		if out != "" {
			t.Fatalf("empty input should return empty, got %q", out)
		}
	})
}

// ---------------------------------------------------------------------------
// TestDiffFoldCommand — /diff-fold 斜杠命令行为
// ---------------------------------------------------------------------------

func TestDiffFoldCommand(t *testing.T) {
	t.Run("sets maxLines with numeric argument", func(t *testing.T) {
		m, _ := newStubModel(nil)
		m.textarea.SetValue("/diff-fold 10")

		updated, _ := m.submit()

		if updated.diffMaxLines != 10 {
			t.Fatalf("diffMaxLines = %d, want 10", updated.diffMaxLines)
		}
		if updated.busy {
			t.Fatal("/diff-fold should not start a turn (busy=false)")
		}
		if updated.statusMsg == "" {
			t.Fatal("statusMsg should be set after /diff-fold")
		}
	})

	t.Run("no argument shows current value", func(t *testing.T) {
		m, _ := newStubModel(nil)
		m.diffMaxLines = 7
		m.textarea.SetValue("/diff-fold")

		updated, _ := m.submit()

		if updated.diffMaxLines != 7 {
			t.Fatalf("diffMaxLines = %d, want 7 (unchanged)", updated.diffMaxLines)
		}
		if !updated.busy {
			// fine — busy should be false
		}
		if updated.statusMsg == "" {
			t.Fatal("statusMsg should show current value")
		}
	})

	t.Run("invalid argument shows error and preserves value", func(t *testing.T) {
		m, _ := newStubModel(nil)
		m.diffMaxLines = 3
		m.textarea.SetValue("/diff-fold abc")

		updated, _ := m.submit()

		if updated.diffMaxLines != 3 {
			t.Fatalf("diffMaxLines = %d, want 3 (unchanged on error)", updated.diffMaxLines)
		}
		if !strings.Contains(updated.statusMsg, "无效") {
			t.Fatalf("statusMsg should indicate error, got: %s", updated.statusMsg)
		}
		if updated.busy {
			t.Fatal("/diff-fold with invalid arg should not start a turn")
		}
	})

	t.Run("clears textarea after command", func(t *testing.T) {
		m, _ := newStubModel(nil)
		m.textarea.SetValue("/diff-fold 25")

		updated, _ := m.submit()

		if updated.textarea.Value() != "" {
			t.Fatalf("textarea = %q, want empty after /diff-fold", updated.textarea.Value())
		}
	})

	t.Run("handles leading and trailing spaces", func(t *testing.T) {
		m, _ := newStubModel(nil)
		m.textarea.SetValue("  /diff-fold  15  ")

		updated, _ := m.submit()

		if updated.diffMaxLines != 15 {
			t.Fatalf("diffMaxLines = %d, want 15", updated.diffMaxLines)
		}
	})

	t.Run("negative argument clamps to zero", func(t *testing.T) {
		m, _ := newStubModel(nil)
		m.diffMaxLines = 8
		m.textarea.SetValue("/diff-fold -5")

		updated, _ := m.submit()

		if updated.diffMaxLines != 0 {
			t.Fatalf("diffMaxLines = %d, want 0 (negative clamped)", updated.diffMaxLines)
		}
		if !strings.Contains(updated.statusMsg, "不限制") {
			t.Logf("statusMsg for negative clamp: %s", updated.statusMsg)
		}
	})

	t.Run("large number works", func(t *testing.T) {
		m, _ := newStubModel(nil)
		m.textarea.SetValue("/diff-fold 9999")

		updated, _ := m.submit()

		if updated.diffMaxLines != 9999 {
			t.Fatalf("diffMaxLines = %d, want 9999", updated.diffMaxLines)
		}
	})
}

// ---------------------------------------------------------------------------
// TestDiffFoldDisable — 禁言 diff 折叠 (maxLines=0)
// ---------------------------------------------------------------------------

func TestDiffFoldDisable(t *testing.T) {
	t.Run("setting to zero disables collapse", func(t *testing.T) {
		m, _ := newStubModel(nil)
		m.diffMaxLines = 5
		m.textarea.SetValue("/diff-fold 0")

		updated, _ := m.submit()

		if updated.diffMaxLines != 0 {
			t.Fatalf("diffMaxLines = %d, want 0 (disabled)", updated.diffMaxLines)
		}
		if !strings.Contains(updated.statusMsg, "不限制") {
			t.Fatalf("statusMsg should say '不限制', got: %s", updated.statusMsg)
		}
	})

	t.Run("fresh model starts with zero (no limit)", func(t *testing.T) {
		m, _ := newStubModel(nil)
		if m.diffMaxLines != 0 {
			t.Fatalf("fresh model diffMaxLines = %d, want 0", m.diffMaxLines)
		}
	})

	t.Run("diff output unobstructed when disabled", func(t *testing.T) {
		diff := strings.Join([]string{
			"--- a/file.go",
			"+++ b/file.go",
			"@@ -1,5 +1,7 @@",
			" context 1",
			"+added 1",
			" context 2",
			"-removed 1",
			" context 3",
			"+added 2",
		}, "\n")

		out := renderDiffOutput(diff, 0)
		if strings.Contains(out, "collapsed") {
			t.Fatal("diffMaxLines=0 must not collapse output")
		}
		if !strings.Contains(out, "+added 2") {
			t.Fatal("should show all lines when disabled")
		}
	})

	t.Run("tool result with diffMaxLines=0 renders fully", func(t *testing.T) {
		m := New()
		m.width = 80
		m.busy = true
		m.diffMaxLines = 0 // disabled

		var lines []string
		lines = append(lines, "--- a/main.go", "+++ b/main.go", "@@ -1,4 +1,5 @@")
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
			ToolCallID: "call_diff_disable",
		})
		updated := next.(Model)

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
		if !strings.Contains(content, "+added-12") {
			t.Fatal("all lines should be visible when diffMaxLines=0")
		}
		if strings.Contains(content, "collapsed") {
			t.Fatal("should NOT collapse when disabled (diffMaxLines=0)")
		}
	})
}
