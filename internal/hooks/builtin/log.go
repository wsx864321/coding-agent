package builtin

import (
	"context"
	"fmt"
	"strings"
)

// LogHook 打印每次工具调用的摘要（用于 debug / 审计）
type LogHook struct {
	Sink *Sink
}

// NewLogHook 构造一个 LogHook
func NewLogHook() *LogHook {
	return &LogHook{}
}

// Handle 实现 hooks.PreToolUseHook（仅打日志，不阻断）
func (h *LogHook) Handle(_ context.Context, name string, args map[string]any) (string, string) {
	preview := argPreview(args, 60)
	h.sink().Fprintf("[HOOK] %s(%s)\n", name, preview)
	return "", ""
}

func (h *LogHook) sink() *Sink {
	if h.Sink != nil {
		return h.Sink
	}
	return &Sink{}
}

// argPreview 把 args 拍平成 "<key1>=<val1>, <key2>=<val2>" 形式并截断
func argPreview(args map[string]any, max int) string {
	if len(args) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(args))
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	s := strings.Join(parts, ", ")
	if len(s) > max {
		s = s[:max] + "..."
	}
	return s
}
