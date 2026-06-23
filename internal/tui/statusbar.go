package tui

import (
	"fmt"
	"time"
)

func (m Model) bottomHeight() int {
	h := 1 // status bar
	h += 1 // help
	h += m.textarea.Height()
	if m.approval != nil {
		h += 2 // approval banner
	}
	if m.lastError != "" {
		h += 1
	}
	return h
}

func renderStatusBar(m Model) string {
	if m.busy {
		spin := m.spinner.View()
		elapsed := time.Since(m.runStart).Truncate(time.Second)
		label := m.statusLabel
		if label == "" {
			label = "thinking"
		}
		return fmt.Sprintf("%s %s (%s)", spin, label, elapsed)
	}
	model := m.modelName
	if model == "" {
		model = "coding-agent"
	}
	if m.statusMsg != "" {
		return fmt.Sprintf("%s │ %s", model, m.statusMsg)
	}
	return fmt.Sprintf("%s │ idle", model)
}
