package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var (
	messageStyle = lipgloss.NewStyle()
	helpStyle    = lipgloss.NewStyle().Faint(true)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	statusStyle  = lipgloss.NewStyle().Faint(true)
)

const helpText = "Shift+Enter 换行 · Enter 发送 · Esc 中断 · Ctrl+C 退出"

// View 渲染对话区、审批横幅、状态栏、输入区与快捷键帮助。
func (m Model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var parts []string
	parts = append(parts, messageStyle.Render(m.viewport.View()))

	if m.approval != nil {
		banner := renderApprovalBanner(*m.approval, m.contentWidth())
		parts = append(parts, statusStyle.Render(banner))
	}
	if m.lastError != "" {
		parts = append(parts, errorStyle.Render("错误: "+m.lastError))
	}
	parts = append(parts, statusStyle.Render(renderStatusBar(m)))
	parts = append(parts, m.textarea.View())
	parts = append(parts, helpStyle.Render(helpText))

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
