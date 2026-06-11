package builtin

import (
	"context"

	"github.com/wsx864321/coding-agent/internal/hooks"
)

// LargeOutputThreshold 默认 50KB
const LargeOutputThreshold = 50000

// LargeOutputHook 在工具输出过大时打警告
type LargeOutputHook struct {
	Threshold int
	Sink      *Sink
}

// NewLargeOutputHook 构造一个 LargeOutputHook
func NewLargeOutputHook() *LargeOutputHook {
	return &LargeOutputHook{Threshold: LargeOutputThreshold}
}

// Handle 实现 hooks.PostToolUseHook
func (h *LargeOutputHook) Handle(ctx context.Context, name string, _ map[string]any, output string) {
	threshold := h.Threshold
	if threshold <= 0 {
		threshold = LargeOutputThreshold
	}
	if len(output) > threshold {
		prefix := "[HOOK]"
		if hooks.IsSubagent(ctx) {
			prefix = "[HOOK][sub]"
		}
		h.sink().Fprintf("%s ⚠ Large output from %s: %d chars (threshold=%d)\n",
			prefix, name, len(output), threshold)
	}
}

func (h *LargeOutputHook) sink() *Sink {
	if h.Sink != nil {
		return h.Sink
	}
	return &Sink{}
}
