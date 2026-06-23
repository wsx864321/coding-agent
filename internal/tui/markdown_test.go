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
