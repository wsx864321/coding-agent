package tui

import (
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

var userBubbleStyle = lipgloss.NewStyle().
	Padding(0, 1).
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("63"))

var reasoningDimStyle = lipgloss.NewStyle().Faint(true)

func (m Model) appendEntry(e TranscriptEntry) Model {
	if e.Content == "" {
		e = m.renderEntry(e)
	}
	m.transcript = append(m.transcript, e)
	return m
}

func (m Model) appendUserMessage(raw string) Model {
	return m.appendEntry(TranscriptEntry{Kind: EntryUserMessage, Raw: raw})
}

func (m Model) appendAssistantRendered(rendered, raw string) Model {
	if raw == "" {
		return m
	}
	if len(m.transcript) == 0 || m.transcript[len(m.transcript)-1].Kind != EntryAssistantChunk {
		e := TranscriptEntry{Kind: EntryAssistantChunk, Raw: raw, Content: m.formatAssistantBody(rendered)}
		m.transcript = append(m.transcript, e)
		return m
	}
	last := len(m.transcript) - 1
	if m.transcript[last].Raw != "" {
		m.transcript[last].Raw += "\n"
	}
	m.transcript[last].Raw += raw
	if m.transcript[last].Content == "" {
		m.transcript[last].Content = m.formatAssistantBody(rendered)
	} else {
		m.transcript[last].Content += "\n" + m.formatAssistantContinuation(rendered)
	}
	return m
}

func (m Model) renderEntry(e TranscriptEntry) TranscriptEntry {
	w := m.contentWidth()
	switch e.Kind {
	case EntryUserMessage:
		e.Content = renderUserBubble(e.Raw, w)
	case EntryAssistantChunk:
		e.Content = m.renderAssistantText(e.Raw, w)
	case EntryToolCard:
		name, args, isError := decodeToolCardRaw(e.Raw)
		if isError {
			e.Content = renderToolCardError(name, args, w)
		} else {
			e.Content = renderToolCard(name, args, w)
		}
	case EntryToolOutput:
		toolCallID, output := decodeToolOutputRaw(e.Raw)
		if toolCallID != "" && m.shellExpanded[toolCallID] {
			// Show full output when expanded (use len of lines as maxLines to avoid collapse)
			fullOutput := m.shellOutputs[toolCallID]
			if fullOutput == "" {
				fullOutput = output
			}
			lines := splitLines(fullOutput)
			e.Content = renderToolOutput(fullOutput, len(lines))
		} else {
			e.Content = renderToolOutput(output, toolOutputCollapseLines)
		}
	case EntryError:
		e.Content = errorStyle.Render(e.Raw)
	case EntryReasoning:
		e = m.renderReasoningEntry(e)
	case EntryToolStream:
		e.Content = m.renderToolStreamBlock()
	default:
		if e.Content == "" {
			e.Content = e.Raw
		}
	}
	return e
}

func (m Model) contentWidth() int {
	width := m.width
	if width <= 0 {
		width = 80
	}
	contentWidth := width - 2
	if contentWidth < 10 {
		contentWidth = 10
	}
	return contentWidth
}

func (m Model) assistantInnerWidth() int {
	prefixWidth := runewidth.StringWidth("> ")
	w := m.contentWidth() - prefixWidth
	if w < 4 {
		w = 4
	}
	return w
}

func renderUserBubble(raw string, width int) string {
	innerWidth := width - 4
	if innerWidth < 4 {
		innerWidth = 4
	}
	lines := WrapText(raw, innerWidth)
	return userBubbleStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m Model) renderAssistantText(raw string, width int) string {
	prefix := "> "
	prefixWidth := runewidth.StringWidth(prefix)
	innerWidth := width - prefixWidth
	if innerWidth < 4 {
		innerWidth = 4
	}

	var body string
	if m.mdRenderer != nil {
		body = strings.TrimRight(m.mdRenderer.Render(raw, innerWidth), "\n")
	} else {
		body = strings.Join(WrapText(raw, innerWidth), "\n")
	}
	return m.applyAssistantPrefix(body, prefix, prefixWidth)
}

func (m Model) formatAssistantBody(rendered string) string {
	prefix := "> "
	prefixWidth := runewidth.StringWidth(prefix)
	body := strings.TrimRight(rendered, "\n")
	return m.applyAssistantPrefix(body, prefix, prefixWidth)
}

func (m Model) formatAssistantContinuation(rendered string) string {
	prefix := "> "
	prefixWidth := runewidth.StringWidth(prefix)
	body := strings.TrimRight(rendered, "\n")
	if body == "" {
		return ""
	}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		lines[i] = strings.Repeat(" ", prefixWidth) + line
	}
	return strings.Join(lines, "\n")
}

func (m Model) applyAssistantPrefix(body, prefix string, prefixWidth int) string {
	if body == "" {
		return prefix
	}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if i == 0 {
			lines[i] = prefix + line
		} else {
			lines[i] = strings.Repeat(" ", prefixWidth) + line
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderTranscriptContent() string {
	if len(m.transcript) == 0 {
		return m.renderWelcomeBanner()
	}
	parts := make([]string, len(m.transcript))
	for i, e := range m.transcript {
		parts[i] = e.Content
	}
	return joinLines(parts)
}

func (m Model) renderWelcomeBanner() string {
	w := m.contentWidth()
	if w < 20 {
		w = 20
	}
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(1, 3).
		Width(w)

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")).Render("coding-agent")
	subtitle := lipgloss.NewStyle().Faint(true).Render("AI 编码助手 — 在 Agent Loop 中驱动 LLM 操作文件系统")

	cwd, _ := os.Getwd()
	info := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("  Model : ") + m.modelName,
		lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render("  CWD   : ") + cwd,
	)

	shortcuts := lipgloss.NewStyle().Faint(true).Render(
		"  /help 帮助  ·  /skills Skill  ·  Esc 中断  ·  Ctrl+C 退出",
	)

	body := lipgloss.JoinVertical(lipgloss.Center,
		title,
		"",
		subtitle,
		"",
		info,
		"",
		shortcuts,
	)
	// Center the banner vertically by adding empty lines
	h := strings.Count(body, "\n") + 6 // border + padding
	vh := m.viewport.Height
	if vh > h+4 {
		topPad := (vh - h) / 2
		if topPad > 0 {
			body = strings.Repeat("\n", topPad) + body
		}
	}
	return border.Render(body)
}

func (m Model) rebuildViewport() Model {
	return m.syncViewportContent()
}

func (m Model) rerenderTranscript() Model {
	for i := range m.transcript {
		m.transcript[i] = m.renderEntry(m.transcript[i])
	}
	return m
}

func (m Model) rebuildTranscript() Model {
	return m.rerenderTranscript()
}

func (m Model) renderReasoningSummary(raw string) string {
	if raw == "" {
		return ""
	}
	summary := raw
	if len(summary) > 60 {
		summary = summary[:60] + "…"
	}
	return reasoningDimStyle.Render("💭 " + summary)
}

func (m Model) renderReasoningEntry(e TranscriptEntry) TranscriptEntry {
	summary := m.renderReasoningSummary(e.Raw)
	if !m.showReasoning {
		e.Content = summary
		return e
	}
	// Expanded: summary line + separator + full reasoning text
	sep := reasoningDimStyle.Render(strings.Repeat("─", m.contentWidth()))
	e.Content = summary + "\n" + sep + "\n" + reasoningDimStyle.Render(e.Raw)
	return e
}
