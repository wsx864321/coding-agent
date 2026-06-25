package tui

import (
	"encoding/json"
	"fmt"
	"strings"
)

// todoStatusIcon 返回任务状态对应的图标。
func todoStatusIcon(status string) string {
	switch status {
	case "completed":
		return "✓"
	case "in_progress":
		return "⟳"
	default:
		return "⏳"
	}
}

// parseTodoItems 解析 todo_write 的 JSON 参数，返回任务列表。
// 解析失败或 JSON 非数组时返回空切片。
func parseTodoItems(rawJSON string) []todoItem {
	rawJSON = strings.TrimSpace(rawJSON)
	if rawJSON == "" {
		return nil
	}
	var items []todoItem
	if err := json.Unmarshal([]byte(rawJSON), &items); err != nil {
		return nil
	}
	return items
}

// renderTodoPanel 渲染 Todo 任务面板。
// 空任务列表返回空字符串。
func renderTodoPanel(items []todoItem) string {
	if len(items) == 0 {
		return ""
	}
	var parts []string
	for _, it := range items {
		icon := todoStatusIcon(it.Status)
		parts = append(parts, fmt.Sprintf("%s %s", icon, it.Content))
	}
	return strings.Join(parts, " · ")
}
