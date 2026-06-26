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

const helpText = "Enter 发送 · Ctrl+J 换行 · Esc 中断 · Ctrl+O 推理 · Ctrl+B Shell · Ctrl+C 退出/复制"

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

	// 斜杠命令输出浮动面板
	if m.slashOverlay != "" {
		overlay := renderSlashOverlay(m.slashOverlay, m.contentWidth())
		parts = append(parts, overlay)
	}

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

	// 斜杠命令补全菜单（渲染在输入区上方）
	if m.completion.active {
		menu := renderCompletion(m.completion, m.contentWidth())
		if menu != "" {
			// 插入到输入区之前
			idx := len(parts) - 2 // textarea is second-to-last
			if idx >= 0 {
				parts = append(parts[:idx], append([]string{menu}, parts[idx:]...)...)
			}
		}
	}

	v := tea.NewView(joinLines(parts))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func renderSlashOverlay(content string, maxW int) string {
	if maxW < 40 {
		maxW = 40
	}
	w := maxW
	if w > 76 {
		w = 76
	}

	// 从 content 中提取标题行（第二行）
	lines := strings.Split(content, "\n")
	title := "Output"
	if len(lines) > 1 {
		t := strings.TrimSpace(lines[1])
		if t != "" {
			title = t
		}
	}

	var b strings.Builder
	// 顶部 bar
	bar := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(strings.Repeat("─", w))
	b.WriteString(bar)
	b.WriteByte('\n')

	// 标题
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true).Render("  " + title))
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("  (ESC 关闭)"))
	b.WriteByte('\n')
	b.WriteString(bar)
	b.WriteByte('\n')

	// 内容（跳过标题和 divider line）
	start := 0
	if len(lines) > 2 && strings.Contains(lines[0], "──") {
		start = 3 // 跳过分隔线、标题、空行
	}
	for i := start; i < len(lines); i++ {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("  " + lines[i]))
		b.WriteByte('\n')
	}
	b.WriteString(bar)
	return b.String()
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


