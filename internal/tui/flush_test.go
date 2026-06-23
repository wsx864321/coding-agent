package tui

import (
	"strings"
	"testing"
)

func TestFlushablePrefixHalfCodeBlock(t *testing.T) {
	open := "intro line\n\n```go\ncode"
	renderable, remaining := flushableMarkdownPrefix(open)
	if renderable != "intro line" {
		t.Fatalf("renderable=%q, want intro line", renderable)
	}
	if !strings.Contains(remaining, "```") {
		t.Fatal("code block should remain buffered")
	}
}

func TestFlushablePrefixCompleteBlock(t *testing.T) {
	closed := "```go\ncode\n```\n\nmore"
	renderable, _ := flushableMarkdownPrefix(closed)
	if !strings.Contains(renderable, "```") {
		t.Fatalf("complete block should be renderable: %q", renderable)
	}
}

func TestFlushablePrefixEmpty(t *testing.T) {
	renderable, remaining := flushableMarkdownPrefix("")
	if renderable != "" {
		t.Fatalf("renderable=%q, want empty", renderable)
	}
	if remaining != "" {
		t.Fatalf("remaining=%q, want empty", remaining)
	}
}

func TestFlushablePrefixCrossParagraph(t *testing.T) {
	buf := "first para\n\nsecond para\n\nthird"
	renderable, remaining := flushableMarkdownPrefix(buf)
	want := "first para\n\nsecond para"
	if renderable != want {
		t.Fatalf("renderable=%q, want %q", renderable, want)
	}
	if remaining != "\nthird" {
		t.Fatalf("remaining=%q, want \\nthird", remaining)
	}
}

func TestFlushablePrefixNoBoundary(t *testing.T) {
	renderable, remaining := flushableMarkdownPrefix("no boundary yet")
	if renderable != "" {
		t.Fatalf("renderable=%q, want empty", renderable)
	}
	if remaining != "no boundary yet" {
		t.Fatalf("remaining=%q, want full buffer", remaining)
	}
}
