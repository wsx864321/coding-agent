package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true)
	messageStyle = lipgloss.NewStyle()
	helpStyle    = lipgloss.NewStyle().Faint(true)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	statusStyle  = lipgloss.NewStyle().Faint(true)
	spinnerStyle = lipgloss.NewStyle()
)

// View 渲染标题、消息区、输入区与快捷键帮助。
func (m Model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	title := titleStyle.Render("coding-agent TUI")
	messagePane := messageStyle.Render(m.viewport.View())
	inputPane := m.textarea.View()
	help := helpStyle.Render("↑↓/jk 滚动 · Shift+Enter 换行 · Enter 发送 · Esc 中断 · Ctrl+C 退出")

	var parts []string
	parts = append(parts, title, "", messagePane, "", inputPane)
	if m.busy {
		parts = append(parts, "", spinnerStyle.Render(m.spinner.View()+" 思考中…"))
	}
	if m.lastError != "" {
		parts = append(parts, "", errorStyle.Render("错误: "+m.lastError))
	}
	if m.statusMsg != "" {
		parts = append(parts, "", statusStyle.Render(m.statusMsg))
	}
	parts = append(parts, "", help)

	v := tea.NewView(joinLines(parts))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
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
