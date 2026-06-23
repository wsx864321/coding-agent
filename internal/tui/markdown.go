package tui

import (
	"charm.land/glamour/v2"
)

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
	)
	if err != nil {
		return md
	}
	out, err := r.Render(md)
	if err != nil {
		return md
	}
	return out
}
