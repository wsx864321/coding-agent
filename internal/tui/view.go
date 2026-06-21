package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	messageStyle  = lipgloss.NewStyle()
	inputStyle    = lipgloss.NewStyle()
	helpStyle     = lipgloss.NewStyle().Faint(true)
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	statusStyle   = lipgloss.NewStyle().Faint(true)
)

// View 渲染标题、消息区、输入区与快捷键帮助。
func (m Model) View() string {
	if m.quitting {
		return ""
	}

	title := titleStyle.Render("coding-agent TUI")
	messagePane := m.renderMessagePane()
	inputPane := inputStyle.Render("> " + m.input)
	help := helpStyle.Render("↑↓/jk 滚动 · Enter 发送 · Esc 中断 · Ctrl+C 退出")

	var parts []string
	parts = append(parts, title, "", messagePane, "", inputPane)
	if m.lastError != "" {
		parts = append(parts, "", errorStyle.Render("错误: "+m.lastError))
	}
	if m.statusMsg != "" {
		parts = append(parts, "", statusStyle.Render(m.statusMsg))
	}
	parts = append(parts, "", help)
	return joinLines(parts)
}

func (m Model) renderMessagePane() string {
	lines := m.renderMessageLines()
	if len(lines) == 0 {
		return messageStyle.Render("(暂无消息)")
	}

	viewport := m.messageViewportHeight()
	start := m.clampScroll(m.scrollOffset)
	end := start + viewport
	if end > len(lines) {
		end = len(lines)
	}

	visible := lines[start:end]
	return messageStyle.Render(joinLines(visible))
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	out := lines[0]
	for i := 1; i < len(lines); i++ {
		out += "\n" + lines[i]
	}
	return out
}
