package tui

import (
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
)

// chromaStyle is the chroma syntax highlighting style used for code blocks.
// Uses monokai if available, otherwise falls back to the default fallback style.
var chromaStyle = func() *chroma.Style {
	if s := styles.Get("monokai"); s != nil && s != styles.Fallback {
		return s
	}
	return styles.Fallback
}()

// MarkdownRenderer renders markdown to ANSI-styled terminal text.
type MarkdownRenderer interface {
	Render(markdown string, width int) string
}

type glamourRenderer struct{}

// NewGlamourRenderer returns a glamour-backed MarkdownRenderer.
func NewGlamourRenderer() MarkdownRenderer {
	return glamourRenderer{}
}

func (g glamourRenderer) Render(md string, width int) string {
	if width <= 0 {
		width = 80
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(width),
		glamour.WithChromaFormatter("terminal256"),
	)
	if err != nil {
		return md
	}
	out, err := r.Render(md)
	if err != nil {
		return md
	}
	// If the markdown contains a diff code block, apply diff coloring overlay.
	if isDiffMarkdown(md) {
		out = applyDiffColoring(out)
	}
	return out
}

// isDiffMarkdown reports whether the markdown source contains a diff code block
// (i.e. a fenced code block with the "diff" language tag).
func isDiffMarkdown(md string) bool {
	// Look for ```diff or ~~~diff fence openers.
	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```diff") || strings.HasPrefix(trimmed, "~~~diff") {
			return true
		}
	}
	return false
}

// diff line styles.
var (
	diffAddedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#2ecc71")) // green
	diffRemovedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#e74c3c")) // red
	diffHunkStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#00bcd4")) // cyan
)

// applyDiffColoring applies diff-specific ANSI foreground coloring to a
// glamour-rendered output that contains diff lines. Lines starting with '+'
// get green, '-' get red, and '@@' get cyan.
func applyDiffColoring(out string) string {
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "@@"):
			lines[i] = diffHunkStyle.Render(line)
		case strings.HasPrefix(trimmed, "+"):
			lines[i] = diffAddedStyle.Render(line)
		case strings.HasPrefix(trimmed, "-"):
			lines[i] = diffRemovedStyle.Render(line)
		}
	}
	return strings.Join(lines, "\n")
}
