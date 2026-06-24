package tui

// selection 表示用户在 viewport 中的文本选择范围。
// 当 active 为 true 且起止位置不同时表示有效选择。
type selection struct {
	startLine int
	startCol  int
	endLine   int
	endCol    int
	active    bool
}

// empty 返回选择是否为空（未激活或起止位置相同）。
func (s selection) empty() bool {
	if !s.active {
		return true
	}
	return s.startLine == s.endLine && s.startCol == s.endCol
}
