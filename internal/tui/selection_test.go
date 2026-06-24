package tui

import (
	"testing"
)

func TestSelectionEmpty(t *testing.T) {
	tests := []struct {
		name string
		sel  selection
		want bool
	}{
		{
			name: "zero value is empty",
			sel:  selection{},
			want: true,
		},
		{
			name: "active with no range is empty",
			sel:  selection{active: true},
			want: true,
		},
		{
			name: "single line selection is not empty",
			sel:  selection{startLine: 0, startCol: 0, endLine: 0, endCol: 5, active: true},
			want: false,
		},
		{
			name: "multi line selection is not empty",
			sel:  selection{startLine: 1, startCol: 3, endLine: 5, endCol: 10, active: true},
			want: false,
		},
		{
			name: "same start and end is empty",
			sel:  selection{startLine: 2, startCol: 4, endLine: 2, endCol: 4, active: true},
			want: true,
		},
		{
			name: "not active is empty even with range",
			sel:  selection{startLine: 0, startCol: 0, endLine: 0, endCol: 5, active: false},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.sel.empty()
			if got != tt.want {
				t.Errorf("empty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSelectionFieldsExist(t *testing.T) {
	s := selection{
		startLine: 1,
		startCol:  2,
		endLine:   3,
		endCol:    4,
		active:    true,
	}

	if s.startLine != 1 {
		t.Errorf("startLine = %d, want 1", s.startLine)
	}
	if s.startCol != 2 {
		t.Errorf("startCol = %d, want 2", s.startCol)
	}
	if s.endLine != 3 {
		t.Errorf("endLine = %d, want 3", s.endLine)
	}
	if s.endCol != 4 {
		t.Errorf("endCol = %d, want 4", s.endCol)
	}
	if !s.active {
		t.Error("active should be true")
	}
}

func TestSelectionContainsLine(t *testing.T) {
	tests := []struct {
		name string
		sel  selection
		line int
		want bool
	}{
		{
			name: "empty selection contains nothing",
			sel:  selection{},
			line: 0,
			want: false,
		},
		{
			name: "inactive selection contains nothing",
			sel:  selection{startLine: 1, endLine: 3, active: false},
			line: 2,
			want: false,
		},
		{
			name: "line before range not contained",
			sel:  selection{startLine: 2, endLine: 5, active: true},
			line: 1,
			want: false,
		},
		{
			name: "line after range not contained",
			sel:  selection{startLine: 2, endLine: 5, active: true},
			line: 6,
			want: false,
		},
		{
			name: "line at start of range contained",
			sel:  selection{startLine: 2, endLine: 5, active: true},
			line: 2,
			want: true,
		},
		{
			name: "line at end of range contained",
			sel:  selection{startLine: 2, endLine: 5, active: true},
			line: 5,
			want: true,
		},
		{
			name: "line in middle of range contained",
			sel:  selection{startLine: 2, endLine: 5, active: true},
			line: 3,
			want: true,
		},
		{
			name: "single line selection contains itself",
			sel:  selection{startLine: 3, endLine: 3, startCol: 0, endCol: 5, active: true},
			line: 3,
			want: true,
		},
		{
			name: "reversed range still contains middle line",
			sel:  selection{startLine: 5, endLine: 2, active: true},
			line: 3,
			want: true,
		},
		{
			name: "reversed range contains start line",
			sel:  selection{startLine: 5, endLine: 2, active: true},
			line: 5,
			want: true,
		},
		{
			name: "reversed range contains end line",
			sel:  selection{startLine: 5, endLine: 2, active: true},
			line: 2,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.sel.containsLine(tt.line)
			if got != tt.want {
				t.Errorf("containsLine(%d) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestSelectionHighlightLine(t *testing.T) {
	sel := selection{startLine: 1, endLine: 3, active: true}

	// Line outside selection should be unchanged (no ANSI reverse codes).
	outside := sel.highlightLine("hello", 0)
	if outside != "hello" {
		t.Errorf("highlightLine outside range: got %q, want %q", outside, "hello")
	}

	// Line inside selection should contain ANSI reverse escape codes.
	inside := sel.highlightLine("world", 2)
	if inside == "world" {
		t.Error("highlightLine inside range: line was not styled")
	}
	// lipgloss Reverse is ANSI code 7 (reverse video).
	if !containsANSISequence(inside) {
		t.Errorf("highlightLine inside range: expected ANSI escape codes, got %q", inside)
	}

	// Inactive selection should not highlight.
	inactive := selection{startLine: 1, endLine: 3, active: false}
	unchanged := inactive.highlightLine("test", 2)
	if unchanged != "test" {
		t.Errorf("highlightLine with inactive selection: got %q, want %q", unchanged, "test")
	}
}

func TestSelectionHighlightRange(t *testing.T) {
	sel := selection{startLine: 1, endLine: 2, active: true}
	lines := []string{"line0", "line1", "line2", "line3"}

	result := sel.highlightRange(lines)

	if len(result) != len(lines) {
		t.Fatalf("highlightRange returned %d lines, want %d", len(result), len(lines))
	}

	// line0 should be unchanged.
	if result[0] != "line0" {
		t.Errorf("line0: got %q, want %q", result[0], "line0")
	}

	// line1 and line2 should be styled (contain ANSI codes).
	if !containsANSISequence(result[1]) {
		t.Errorf("line1 should be styled but got %q", result[1])
	}
	if !containsANSISequence(result[2]) {
		t.Errorf("line2 should be styled but got %q", result[2])
	}

	// line3 should be unchanged.
	if result[3] != "line3" {
		t.Errorf("line3: got %q, want %q", result[3], "line3")
	}
}

func TestSelectionHighlightRangeEmpty(t *testing.T) {
	sel := selection{}
	lines := []string{"a", "b", "c"}
	result := sel.highlightRange(lines)
	if len(result) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(result))
	}
	for i, line := range result {
		if line != lines[i] {
			t.Errorf("line %d: got %q, want %q", i, line, lines[i])
		}
	}
}

func TestSelectionHighlightRangeNil(t *testing.T) {
	sel := selection{startLine: 0, endLine: 0, active: true}
	result := sel.highlightRange(nil)
	if result != nil {
		t.Errorf("highlightRange(nil) should return nil, got %v", result)
	}
}

func TestExtractSelectedText(t *testing.T) {
	tests := []struct {
		name  string
		sel   selection
		lines []string
		want  string
	}{
		{
			name:  "empty selection returns empty",
			sel:   selection{},
			lines: []string{"hello", "world"},
			want:  "",
		},
		{
			name:  "inactive selection returns empty",
			sel:   selection{startLine: 0, startCol: 0, endLine: 0, endCol: 5, active: false},
			lines: []string{"hello", "world"},
			want:  "",
		},
		{
			name:  "same position returns empty",
			sel:   selection{startLine: 0, startCol: 2, endLine: 0, endCol: 2, active: true},
			lines: []string{"hello", "world"},
			want:  "",
		},
		{
			name:  "single line partial selection",
			sel:   selection{startLine: 0, startCol: 1, endLine: 0, endCol: 4, active: true},
			lines: []string{"hello", "world"},
			want:  "ell",
		},
		{
			name:  "single line full selection",
			sel:   selection{startLine: 0, startCol: 0, endLine: 0, endCol: 5, active: true},
			lines: []string{"hello", "world"},
			want:  "hello",
		},
		{
			name:  "multi-line selection",
			sel:   selection{startLine: 0, startCol: 1, endLine: 2, endCol: 4, active: true},
			lines: []string{"hello", "beautiful", "world", "extra"},
			want:  "ello\nbeautiful\nworl",
		},
		{
			name:  "reverse multi-line selection",
			sel:   selection{startLine: 2, startCol: 4, endLine: 0, endCol: 1, active: true},
			lines: []string{"hello", "beautiful", "world", "extra"},
			want:  "ello\nbeautiful\nworl",
		},
		{
			name:  "selection to end of line",
			sel:   selection{startLine: 0, startCol: 3, endLine: 1, endCol: 9, active: true},
			lines: []string{"hello", "beautiful", "world"},
			want:  "lo\nbeautiful",
		},
		{
			name:  "selection from start of line",
			sel:   selection{startLine: 1, startCol: 0, endLine: 2, endCol: 3, active: true},
			lines: []string{"hello", "beautiful", "world"},
			want:  "beautiful\nwor",
		},
		{
			name:  "col out of bounds clamped to line length",
			sel:   selection{startLine: 0, startCol: 3, endLine: 0, endCol: 99, active: true},
			lines: []string{"hello", "world"},
			want:  "lo",
		},
		{
			name:  "startCol exceeds line length",
			sel:   selection{startLine: 0, startCol: 10, endLine: 0, endCol: 15, active: true},
			lines: []string{"hello", "world"},
			want:  "",
		},
		{
			name:  "selection with ANSI codes stripped",
			sel:   selection{startLine: 0, startCol: 0, endLine: 0, endCol: 5, active: true},
			lines: []string{"\x1b[31mhello\x1b[0m", "world"},
			want:  "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.sel.extractSelectedText(tt.lines)
			if got != tt.want {
				t.Errorf("extractSelectedText() = %q, want %q", got, tt.want)
			}
		})
	}
}

// containsANSISequence checks if a string contains ANSI escape codes (CSI sequences).
func containsANSISequence(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			return true
		}
	}
	return false
}
