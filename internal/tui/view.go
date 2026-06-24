package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var (
	messageStyle = lipgloss.NewStyle()
	helpStyle    = lipgloss.NewStyle().Faint(true)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	statusStyle  = lipgloss.NewStyle().Faint(true)
)

const helpText = "Shift+Enter 换行 · Enter 发送 · Esc 中断 · Ctrl+O 推理 · Ctrl+B Shell · Ctrl+C 退出/复制"

// View 渲染对话区、审批横幅、三行状态栏、Todo 面板、输入区与快捷键帮助。
func (m Model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var parts []string
	vpContent := m.viewport.View()
	if !m.sel.empty() {
		lines := strings.Split(vpContent, "\n")
		lines = m.sel.highlightRange(lines)
		vpContent = strings.Join(lines, "\n")
	}
	parts = append(parts, messageStyle.Render(vpContent))

	if m.approval != nil {
		banner := renderApprovalBanner(*m.approval, m.contentWidth())
		parts = append(parts, statusStyle.Render(banner))
	}
	if m.lastError != "" {
		parts = append(parts, errorStyle.Render("错误: "+m.lastError))
	}

	// 三行状态栏
	if wl := renderWorkingLine(m); wl != "" {
		parts = append(parts, statusStyle.Render(wl))
	}
	parts = append(parts, statusStyle.Render(renderModeLine(m)))
	parts = append(parts, statusStyle.Render(renderDataLine(m)))

	// 状态消息
	if m.statusMsg != "" {
		parts = append(parts, statusStyle.Render(m.statusMsg))
	}

	// Todo 面板
	if m.todoArgs != "" {
		parts = append(parts, renderTodoPanel(m.todoItems))
	}

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


