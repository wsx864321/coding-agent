package builtin

import (
	"context"

	openai "github.com/sashabaranov/go-openai"
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
//
// 统计 messages 中 Role=tool 的条数；不阻断（不返回 force）
func (h *SummaryHook) Handle(_ context.Context, messages []openai.ChatCompletionMessage) (string, bool) {
	count := 0
	for _, m := range messages {
		if m.Role == openai.ChatMessageRoleTool {
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
