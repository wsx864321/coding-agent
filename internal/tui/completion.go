package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// completion 表示斜杠命令自动补全菜单的状态。
type completion struct {
	items    []string // 匹配的命令列表
	selected int      // 当前选中索引
	active   bool     // 菜单是否可见
}

var (
	completionMenuStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("8")).
				Padding(0, 1)

	completionItemStyle     = lipgloss.NewStyle()
	completionSelectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
)

// renderCompletion 渲染补全菜单。如果补全不活跃或 items 为空，返回空字符串。
func renderCompletion(c completion, width int) string {
	if !c.active || len(c.items) == 0 {
		return ""
	}
	var lines []string
	for i, item := range c.items {
		if i == c.selected {
			lines = append(lines, completionSelectedStyle.Render("▶ "+item))
		} else {
			lines = append(lines, completionItemStyle.Render("  "+item))
		}
	}
	content := strings.Join(lines, "\n")
	return completionMenuStyle.Width(width - 2).Render(content)
}

// filterCommands 根据前缀过滤命令列表，返回匹配的子集。
// prefix 包含开头的 "/"，如 "/s"。
func filterCommands(cmds []string, prefix string) []string {
	if prefix == "" {
		return append([]string(nil), cmds...)
	}
	var result []string
	for _, c := range cmds {
		if strings.HasPrefix(c, prefix) {
			result = append(result, c)
		}
	}
	return result
}
