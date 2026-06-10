package builtin

import (
	"context"
)

// ContextInjectHook 在 user 消息追加前打印 workdir 等上下文
type ContextInjectHook struct {
	Workdir string
	Sink    *Sink
}

// NewContextInjectHook 构造一个 ContextInjectHook
func NewContextInjectHook(workdir string) *ContextInjectHook {
	return &ContextInjectHook{Workdir: workdir}
}

// Handle 实现 hooks.UserPromptSubmitHook
func (h *ContextInjectHook) Handle(_ context.Context, _ string) error {
	h.sink().Fprintf("[HOOK] UserPromptSubmit: workdir=%s\n", h.Workdir)
	return nil
}

func (h *ContextInjectHook) sink() *Sink {
	if h.Sink != nil {
		return h.Sink
	}
	return &Sink{}
}
