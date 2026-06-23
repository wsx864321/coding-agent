package tui

import (
	"context"

	"github.com/wsx864321/coding-agent/internal/agent"
)

// StreamEmitter 与 agent 包共享同一接口，避免循环依赖。
type StreamEmitter = agent.StreamEmitter

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

func (e chanEmitter) OnToolStart(name, args string) {
	e.ch <- ToolStartMsg{Name: name, Args: args}
}

func (e chanEmitter) OnToolEnd(name, result string, isError bool) {
	e.ch <- ToolEndMsg{Name: name, Result: result, IsError: isError}
}

func (e chanEmitter) OnApprovalRequest(name string, args map[string]any, respond func(bool)) {
	e.ch <- ApprovalRequestMsg{Name: name, Args: args, Respond: respond}
}

func (e chanEmitter) OnDone() {
	e.ch <- StreamDoneMsg{}
}

func (e chanEmitter) OnError(err error) {
	e.ch <- StreamErrorMsg{Err: err}
}
