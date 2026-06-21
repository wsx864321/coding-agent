package tui

import (
	"context"
)

// StreamEmitter 接收 runner 产生的流式事件，由 TUI 转为 tea.Msg。
type StreamEmitter interface {
	OnChunk(text string)
	OnDone()
	OnError(err error)
}

// Runner 执行一轮用户输入并推送流式事件。
type Runner interface {
	RunTurn(ctx context.Context, prompt string, emit StreamEmitter) error
}

type chanEmitter struct {
	ch chan<- any
}

func (e chanEmitter) OnChunk(text string) {
	if text == "" {
		return
	}
	e.ch <- StreamChunkMsg{Text: text}
}

func (e chanEmitter) OnDone() {
	e.ch <- StreamDoneMsg{}
}

func (e chanEmitter) OnError(err error) {
	e.ch <- StreamErrorMsg{Err: err}
}
