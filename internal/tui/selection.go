package tui

import "charm.land/lipgloss/v2"

// selection 表示用户在 viewport 中的文本选择范围。
// 当 active 为 true 且起止位置不同时表示有效选择。
type selection struct {
	startLine int
	startCol  int
	endLine   int
	endCol    int
	active    bool
	dragging  bool
}

// empty 返回选择是否为空（未激活或起止位置相同）。
func (s selection) empty() bool {
	if !s.active {
		return true
	}
	return s.startLine == s.endLine && s.startCol == s.endCol
}

// containsLine 返回给定行号是否落在选择范围内（含两端）。
// 自动归一化 startLine/endLine 顺序，兼容反向选择。
func (s selection) containsLine(line int) bool {
	if !s.active {
		return false
	}
	lo, hi := s.startLine, s.endLine
	if lo > hi {
		lo, hi = hi, lo
	}
	return line >= lo && line <= hi
}

// highlightLine 若行号在选择范围内则叠加反色样式，否则原样返回。
func (s selection) highlightLine(line string, lineIdx int) string {
	if !s.containsLine(lineIdx) {
		return line
	}
	return lipgloss.NewStyle().Reverse(true).Render(line)
}

// highlightRange 对 lines 中每一行调用 highlightLine，返回新切片。
func (s selection) highlightRange(lines []string) []string {
	if lines == nil {
		return nil
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = s.highlightLine(l, i)
	}
	return out
}
