package tui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

func TestWrapTextCJKDoubleWidth(t *testing.T) {
	// "你好世界" = 4 runes × 2 width = 8; width=6 should break after 3 chars
	lines := WrapText("你好世界", 6)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %v", len(lines), lines)
	}
	if runewidth.StringWidth(lines[0]) > 6 {
		t.Fatalf("first line %q exceeds width 6", lines[0])
	}
}

func TestWrapTextJapanese(t *testing.T) {
	lines := WrapText("こんにちは世界", 8)
	for i, ln := range lines {
		if runewidth.StringWidth(ln) > 8 {
			t.Fatalf("line %d %q exceeds width 8", i, ln)
		}
	}
}

func TestWrapTextEmoji(t *testing.T) {
	lines := WrapText("Hi 👋 there", 8)
	for _, ln := range lines {
		if runewidth.StringWidth(ln) > 8 {
			t.Fatalf("line %q exceeds width 8", ln)
		}
	}
}

func TestWrapTextANSI(t *testing.T) {
	text := "\x1b[31m你好\x1b[0m世界"
	lines := WrapText(text, 6)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %v", len(lines), lines)
	}
	for _, ln := range lines {
		if ansi.StringWidth(ln) > 6 {
			t.Fatalf("line %q exceeds width 6 (ANSI-aware)", ln)
		}
	}
}
