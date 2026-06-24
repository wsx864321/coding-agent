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
