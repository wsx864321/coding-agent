package tui

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
)

const maxInputRows = 5

func newTextarea() textarea.Model {
	ti := textarea.New()
	ti.Prompt = "> "
	ti.CharLimit = 16384
	ti.DynamicHeight = true
	ti.MinHeight = 1
	ti.MaxHeight = maxInputRows
	ti.SetHeight(1)
	ti.ShowLineNumbers = false
	ti.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "ctrl+j", "shift+enter"))
	ti.Focus()
	return ti
}

func newViewport() viewport.Model {
	vp := viewport.New(viewport.WithWidth(80))
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = 3
	return vp
}

func newSpinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return s
}
