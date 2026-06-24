package tui

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"

	"charm.land/glamour/v2"
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
	return out
}
