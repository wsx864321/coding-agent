package tui

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi/parser"
	"github.com/mattn/go-runewidth"
)

// WrapText wraps text to the given display width using runewidth for CJK/emoji.
// ANSI escape sequences are preserved and do not count toward line width.
func WrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	if text == "" {
		return []string{""}
	}

	var lines []string
	var current strings.Builder
	currentW := 0

	flush := func() {
		if current.Len() > 0 {
			lines = append(lines, current.String())
			current.Reset()
			currentW = 0
		}
	}

	b := []byte(text)
	pstate := parser.GroundState
	for i := 0; i < len(b); {
		state, action := parser.Table.Transition(pstate, b[i])

		if state == parser.Utf8State {
			r, size := utf8.DecodeRune(b[i:])
			if size <= 0 {
				size = 1
			}
			rw := runewidth.RuneWidth(r)
			if currentW+rw > width && currentW > 0 {
				flush()
			}
			current.Write(b[i : i+size])
			currentW += rw
			i += size
			pstate = parser.GroundState
			continue
		}

		switch action {
		case parser.PrintAction:
			if b[i] == '\n' {
				flush()
				i++
				pstate = parser.GroundState
				continue
			}
			rw := runewidth.RuneWidth(rune(b[i]))
			if currentW+rw > width && currentW > 0 {
				flush()
			}
			current.WriteByte(b[i])
			currentW += rw
		case parser.ExecuteAction:
			current.WriteByte(b[i])
		default:
			current.WriteByte(b[i])
		}

		if pstate != parser.Utf8State {
			pstate = state
		}
		i++
	}
	flush()
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}
