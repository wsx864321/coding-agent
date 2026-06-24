package tui

import (
	"fmt"
	"time"

	"charm.land/lipgloss/v2"
)

// ---- 三行状态栏渲染 ----

// renderWorkingLine 在 busy 时渲染 spinner 工作行，idle 时返回空字符串。
func renderWorkingLine(m Model) string {
	if !m.busy {
		return ""
	}
	spin := m.spinner.View()
	elapsed := time.Since(m.runStart).Truncate(time.Second)
	label := m.statusLabel
	if label == "" {
		label = "thinking"
	}
	return fmt.Sprintf("%s %s %s", spin, label, elapsed)
}

// renderModeLine 始终渲染模式信息行。
func renderModeLine(m Model) string {
	model := m.modelName
	if model == "" {
		model = "coding-agent"
	}

	gitPart := renderGitStatusStr(m.gitStatus)

	return fmt.Sprintf("Plan · %s · %s", model, gitPart)
}

// renderDataLine 始终渲染数据信息行。
func renderDataLine(m Model) string {
	var parts []string

	// 上下文仪表
	if gauge := renderContextGauge(m.contextUsed, m.contextWindow); gauge != "" {
		parts = append(parts, gauge)
	}

	// 缓存命中率
	if m.cacheHitRate > 0 {
		parts = append(parts, fmt.Sprintf("cache %d%%", m.cacheHitRate))
	}

	// 余额
	if m.balance != "" {
		parts = append(parts, m.balance)
	}

	return joinWithSep(parts, " · ")
}

// ---- 上下文仪表 ----

// renderContextGauge 渲染上下文窗口用量仪表。window <= 0 时返回空。
func renderContextGauge(used, window int) string {
	if window <= 0 {
		return ""
	}
	pct := used * 100 / window
	return fmt.Sprintf("ctx %s/%s (%d%%)", shortTokens(used), shortTokens(window), pct)
}

// gaugeColor 根据百分比返回对应颜色样式。
// <50: 绿色, 50-80: 黄色, >80: 红色
func gaugeColor(pct int) lipgloss.Style {
	switch {
	case pct > 80:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // 红
	case pct >= 50:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // 黄
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // 绿
	}
}

// shortTokens 将 token 数转为人类可读格式（K/M）。
func shortTokens(n int) string {
	switch {
	case n >= 1_000_000:
		v := float64(n) / 1_000_000
		if v == float64(int(v)) {
			return fmt.Sprintf("%.0fM", v)
		}
		return fmt.Sprintf("%.1fM", v)
	case n >= 1_000:
		v := float64(n) / 1_000
		if v == float64(int(v)) {
			return fmt.Sprintf("%.0fK", v)
		}
		return fmt.Sprintf("%.1fK", v)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// ---- Git 状态 ----

// renderGitStatusStr 格式化 git 状态字符串。
func renderGitStatusStr(gs gitStatus) string {
	if gs.branch == "" {
		return "—"
	}
	s := gs.branch
	if gs.dirty {
		s += " *"
	} else {
		if gs.ahead > 0 {
			s += fmt.Sprintf(" ↑%d", gs.ahead)
		}
		if gs.behind > 0 {
			s += fmt.Sprintf(" ↓%d", gs.behind)
		}
	}
	return s
}

// ---- 辅助函数 ----

// joinWithSep 用分隔符连接非空字符串。
func joinWithSep(parts []string, sep string) string {
	var filtered []string
	for _, p := range parts {
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	result := filtered[0]
	for i := 1; i < len(filtered); i++ {
		result += sep + filtered[i]
	}
	return result
}

// ---- renderStatusBar (backward-compatible with view.go) ----

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

// ---- bottomHeight 动态计算 ----

func (m Model) bottomHeight() int {
	h := 0
	if m.busy {
		h++ // 工作行
	}
	h += 2 // 模式行 + 数据行
	if m.todoArgs != "" {
		h++ // Todo 面板
	}
	h += m.textarea.Height() // 输入区
	h += 1                    // 帮助行
	if m.approval != nil {
		h += 2 // 审批横幅
	}
	if m.lastError != "" {
		h += 1
	}
	return h
}
