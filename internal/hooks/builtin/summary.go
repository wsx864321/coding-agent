package builtin

import (
	"context"

	"github.com/wsx864321/coding-agent/internal/provider"
)

// SummaryHook 在主循环结束时打印本次会话的工具调用统计
type SummaryHook struct {
	Sink *Sink
}

// NewSummaryHook 构造一个 SummaryHook
func NewSummaryHook() *SummaryHook {
	return &SummaryHook{}
}

// Handle 实现 hooks.StopHook
func (h *SummaryHook) Handle(_ context.Context, messages []provider.Message) (string, bool) {
	count := 0
	for _, m := range messages {
		if m.Role == provider.RoleTool {
			count++
		}
	}
	h.sink().Fprintf("[HOOK] Stop: session used %d tool calls\n", count)
	return "", false
}

func (h *SummaryHook) sink() *Sink {
	if h.Sink != nil {
		return h.Sink
	}
	return &Sink{}
}
