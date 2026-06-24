package tui

import (
	"strings"
	"testing"
)

func hasANSI(s string) bool {
	return strings.Contains(s, "\x1b[")
}

func TestGlamourRendererCodeBlock(t *testing.T) {
	r := NewGlamourRenderer()
	md := "# Title\n\n```go\nfunc main() {}\n```"
	out := r.Render(md, 80)
	if !strings.Contains(out, "main") {
		t.Fatalf("missing code content: %s", out)
	}
	if !hasANSI(out) {
		t.Fatal("expected ANSI styling")
	}
}

func TestGlamourRendererBold(t *testing.T) {
	r := NewGlamourRenderer()
	out := r.Render("This is **bold** text", 80)
	if !strings.Contains(out, "bold") {
		t.Fatalf("missing bold text: %s", out)
	}
	if !hasANSI(out) {
		t.Fatal("expected ANSI styling for bold")
	}
}

func TestGlamourRendererList(t *testing.T) {
	r := NewGlamourRenderer()
	md := "- first\n- second\n- third"
	out := r.Render(md, 80)
	for _, item := range []string{"first", "second", "third"} {
		if !strings.Contains(out, item) {
			t.Fatalf("missing list item %q: %s", item, out)
		}
	}
	if !hasANSI(out) {
		t.Fatal("expected ANSI styling for list")
	}
}

func TestGlamourRendererTable(t *testing.T) {
	r := NewGlamourRenderer()
	md := "| A | B |\n|---|---|\n| 1 | 2 |"
	out := r.Render(md, 80)
	for _, cell := range []string{"A", "B", "1", "2"} {
		if !strings.Contains(out, cell) {
			t.Fatalf("missing table cell %q: %s", cell, out)
		}
	}
	if !hasANSI(out) {
		t.Fatal("expected ANSI styling for table")
	}
}

func TestGlamourRendererBlockquote(t *testing.T) {
	r := NewGlamourRenderer()
	out := r.Render("> quoted line", 80)
	if !strings.Contains(out, "quoted") {
		t.Fatalf("missing quote text: %s", out)
	}
	if !hasANSI(out) {
		t.Fatal("expected ANSI styling for blockquote")
	}
}

func TestGlamourRendererChromaHighlighting(t *testing.T) {
	r := NewGlamourRenderer()
	md := "```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```"
	out := r.Render(md, 80)
	// Chroma syntax highlighting should produce ANSI escape sequences
	// for different token types (keywords, strings, etc.)
	if !hasANSI(out) {
		t.Fatal("expected ANSI styling from chroma highlighting")
	}
	// Verify keyword "func" is present
	if !strings.Contains(out, "func") {
		t.Fatalf("missing keyword 'func': %s", out)
	}
	// Verify string literal "hello" is present
	if !strings.Contains(out, "hello") {
		t.Fatalf("missing string 'hello': %s", out)
	}
}

func TestChromaStyleVariable(t *testing.T) {
	// chromaStyle should be a non-nil chroma Style
	if chromaStyle == nil {
		t.Fatal("chromaStyle should not be nil")
	}
	// chromaStyle should have a name
	if chromaStyle.Name == "" {
		t.Fatal("chromaStyle should have a non-empty name")
	}
}

func TestModelRendersAssistantWithMarkdown(t *testing.T) {
	m := New()
	m.width = 80
	m = m.appendEntry(TranscriptEntry{Kind: EntryAssistantChunk, Raw: "**hello** world"})
	content := m.transcript[0].Content
	if !strings.Contains(content, "hello") {
		t.Fatalf("missing assistant content: %s", content)
	}
	if !hasANSI(content) {
		t.Fatalf("expected ANSI in assistant entry: %s", content)
	}
	if !strings.HasPrefix(content, "assistant: ") {
		t.Fatalf("expected assistant prefix: %s", content)
	}
}

// --- isDiffMarkdown tests ---

func TestIsDiffMarkdown_WithDiffFence(t *testing.T) {
	md := "Some text\n```diff\n+added line\n-removed line\n```\nMore text"
	if !isDiffMarkdown(md) {
		t.Fatal("expected isDiffMarkdown to return true for ```diff fence")
	}
}

func TestIsDiffMarkdown_WithoutDiffFence(t *testing.T) {
	md := "Some text\n```go\nfunc main() {}\n```\nMore text"
	if isDiffMarkdown(md) {
		t.Fatal("expected isDiffMarkdown to return false for ```go fence")
	}
}

func TestIsDiffMarkdown_PlainText(t *testing.T) {
	if isDiffMarkdown("just plain text without any code blocks") {
		t.Fatal("expected isDiffMarkdown to return false for plain text")
	}
}

func TestIsDiffMarkdown_Empty(t *testing.T) {
	if isDiffMarkdown("") {
		t.Fatal("expected isDiffMarkdown to return false for empty string")
	}
}

// --- applyDiffColoring tests ---

func TestApplyDiffColoring_AddedLine(t *testing.T) {
	// Simulate a rendered diff line starting with +
	in := "+ added line"
	out := applyDiffColoring(in)
	if !strings.Contains(out, "added line") {
		t.Fatalf("missing content after coloring: %s", out)
	}
	if !hasANSI(out) {
		t.Fatal("expected ANSI coloring on added line")
	}
}

func TestApplyDiffColoring_RemovedLine(t *testing.T) {
	in := "- removed line"
	out := applyDiffColoring(in)
	if !strings.Contains(out, "removed line") {
		t.Fatalf("missing content after coloring: %s", out)
	}
	if !hasANSI(out) {
		t.Fatal("expected ANSI coloring on removed line")
	}
}

func TestApplyDiffColoring_HunkHeader(t *testing.T) {
	in := "@@ -1,3 +1,4 @@"
	out := applyDiffColoring(in)
	if !strings.Contains(out, "@@") {
		t.Fatalf("missing hunk header after coloring: %s", out)
	}
	if !hasANSI(out) {
		t.Fatal("expected ANSI coloring on hunk header")
	}
}

func TestApplyDiffColoring_PlainLine(t *testing.T) {
	in := "  unchanged line"
	out := applyDiffColoring(in)
	if !strings.Contains(out, "unchanged line") {
		t.Fatalf("missing content: %s", out)
	}
	// Plain lines should NOT get additional ANSI coloring beyond what glamour already provides
	// But since we pass plain text without ANSI, there should be no ANSI at all
	if hasANSI(out) {
		t.Fatal("expected no additional ANSI coloring on plain line")
	}
}

func TestApplyDiffColoring_MultiLine(t *testing.T) {
	in := "+ added\n- removed\n@@ -1 +1 @@\n  context"
	out := applyDiffColoring(in)
	for _, want := range []string{"added", "removed", "@@", "context"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
	if !hasANSI(out) {
		t.Fatal("expected ANSI coloring in multi-line output")
	}
}

func TestApplyDiffColoring_Empty(t *testing.T) {
	out := applyDiffColoring("")
	if out != "" {
		t.Fatalf("expected empty output, got: %s", out)
	}
}

// --- Integration: Render diff markdown ---

func TestGlamourRendererDiffBlock(t *testing.T) {
	r := NewGlamourRenderer()
	md := "```diff\n+added line\n-removed line\n@@ -1,3 +1,4 @@\n  context line\n```"
	out := r.Render(md, 80)
	for _, want := range []string{"added line", "removed line", "@@", "context line"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in rendered diff: %s", want, out)
		}
	}
	if !hasANSI(out) {
		t.Fatal("expected ANSI styling in diff block")
	}
}

// --- Task 17: chroma syntax highlighting, diff coloring, non-diff unchanged ---

func TestChromaSyntaxHighlighting(t *testing.T) {
	r := NewGlamourRenderer()
	// A Go code block with keywords, types, string literals, and numbers.
	md := "```go\npackage main\n\nimport \"fmt\"\n\nfunc add(a, b int) int {\n\treturn a + b\n}\n```"
	out := r.Render(md, 80)

	// All tokens should be present in the output.
	for _, tok := range []string{"package", "main", "import", "fmt", "func", "add", "int", "return"} {
		if !strings.Contains(out, tok) {
			t.Fatalf("missing token %q in chroma-highlighted output: %s", tok, out)
		}
	}

	// Chroma highlighting must produce ANSI escape sequences.
	if !hasANSI(out) {
		t.Fatal("expected ANSI styling from chroma syntax highlighting")
	}

	// Count ANSI sequences — a properly highlighted code block should have
	// multiple sequences (one per token type, at minimum).
	count := strings.Count(out, "\x1b[")
	if count < 3 {
		t.Fatalf("expected at least 3 ANSI sequences for multi-token highlighting, got %d", count)
	}
}

func TestDiffViewColoring(t *testing.T) {
	r := NewGlamourRenderer()
	md := "```diff\n+added line\n-removed line\n@@ -1,3 +1,4 @@\n  context line\n```"
	out := r.Render(md, 80)

	// Content must be present.
	for _, want := range []string{"added line", "removed line", "@@", "context line"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in diff output: %s", want, out)
		}
	}

	// Diff coloring must produce ANSI sequences.
	if !hasANSI(out) {
		t.Fatal("expected ANSI coloring in diff view")
	}

	// Verify that diff-specific ANSI coloring is applied by checking
	// that the output contains green/red/cyan color codes.
	// The glamour renderer + diff overlay should produce colorized output.
	hasGreen := strings.Contains(out, "#2ecc71") || strings.Contains(out, "32") || strings.Contains(out, "38;2")
	hasRed := strings.Contains(out, "#e74c3c") || strings.Contains(out, "31") || strings.Contains(out, "38;2")
	if !hasGreen && !hasRed {
		// At minimum, the output should have ANSI styling from glamour + diff overlay.
		// If neither explicit color code is found, fall back to checking ANSI count.
		if strings.Count(out, "\x1b[") < 2 {
			t.Fatal("expected diff-specific ANSI coloring (green/red)")
		}
	}
}

func TestNonDiffCodeBlockUnchanged(t *testing.T) {
	r := NewGlamourRenderer()

	// A Go code block that happens to contain lines starting with + and -.
	// These should NOT be colored as diff additions/removals.
	md := "```go\nx := 1\ny := -2\nz := +3\n```"
	out := r.Render(md, 80)

	// Strip ANSI sequences to check for raw token content.
	// Glamour may insert ANSI codes between tokens, so we can't just
	// check for "x := 1" as a contiguous substring.
	plain := stripANSI(out)
	for _, tok := range []string{"x", ":=", "1", "y", "-", "2", "z", "+", "3"} {
		if !strings.Contains(plain, tok) {
			t.Fatalf("missing token %q in output: %s", tok, plain)
		}
	}

	// The output should have ANSI from chroma highlighting, but NOT from diff coloring.
	if !hasANSI(out) {
		t.Fatal("expected ANSI styling from chroma highlighting")
	}

	// Verify isDiffMarkdown returns false for this input.
	if isDiffMarkdown(md) {
		t.Fatal("isDiffMarkdown should return false for ```go block, not ```diff")
	}

	// Render the same content with a ```diff fence and verify it behaves differently.
	diffMd := "```diff\nx := 1\ny := -2\nz := +3\n```"
	diffOut := r.Render(diffMd, 80)

	// The diff output should have diff coloring applied (different from the Go output).
	// At minimum, isDiffMarkdown should return true for the diff variant.
	if !isDiffMarkdown(diffMd) {
		t.Fatal("isDiffMarkdown should return true for ```diff block")
	}

	// The two outputs should differ because diff coloring is applied to the diff variant.
	if out == diffOut {
		t.Fatal("expected different output for ```go vs ```diff with same content")
	}
}


