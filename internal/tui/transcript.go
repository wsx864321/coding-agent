package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

var userBubbleStyle = lipgloss.NewStyle().
	Padding(0, 1).
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("63"))

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

func (m Model) renderEntry(e TranscriptEntry) TranscriptEntry {
	w := m.contentWidth()
	switch e.Kind {
	case EntryUserMessage:
		e.Content = renderUserBubble(e.Raw, w)
	case EntryAssistantChunk:
		e.Content = renderAssistantText(e.Raw, w)
	case EntryError:
		e.Content = errorStyle.Render(e.Raw)
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

func renderUserBubble(raw string, width int) string {
	innerWidth := width - 4
	if innerWidth < 4 {
		innerWidth = 4
	}
	lines := wrapText(raw, innerWidth)
	return userBubbleStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func renderAssistantText(raw string, width int) string {
	prefix := "assistant: "
	innerWidth := width - len(prefix)
	if innerWidth < 4 {
		innerWidth = 4
	}
	wrapped := wrapText(raw, innerWidth)
	if len(wrapped) == 0 {
		return prefix
	}
	var lines []string
	for i, line := range wrapped {
		if i == 0 {
			lines = append(lines, prefix+line)
		} else {
			lines = append(lines, strings.Repeat(" ", len(prefix))+line)
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderTranscriptContent() string {
	if len(m.transcript) == 0 {
		return "(暂无消息)"
	}
	parts := make([]string, len(m.transcript))
	for i, e := range m.transcript {
		parts[i] = e.Content
	}
	return joinLines(parts)
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
