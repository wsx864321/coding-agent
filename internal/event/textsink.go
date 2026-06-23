package event

import (
	"fmt"
	"io"
	"sync"
)

type TextSink struct {
	mu  sync.Mutex
	Out io.Writer
	Err io.Writer
}

func (s *TextSink) Emit(e Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch e.Kind {
	case Text:
		io.WriteString(s.Out, e.Text)
	case ToolDispatch:
		if e.ToolArgs != "" {
			args := e.ToolArgs
			if len(args) > 60 {
				args = args[:60] + "..."
			}
			fmt.Fprintf(s.Err, "  ⚡ %s (%s)\n", e.ToolName, args)
		} else {
			fmt.Fprintf(s.Err, "  ⚡ %s\n", e.ToolName)
		}
	case ToolResult:
		if e.ToolIsErr {
			if e.ToolOutput != "" {
				out := e.ToolOutput
				if len(out) > 80 {
					out = out[:80] + "..."
				}
				fmt.Fprintf(s.Err, "  ✗ %s: %s\n", e.ToolName, out)
			} else {
				fmt.Fprintf(s.Err, "  ✗ %s\n", e.ToolName)
			}
		} else {
			fmt.Fprintf(s.Err, "  ✓ %s\n", e.ToolName)
		}
	case Notice:
		prefix := "·"
		if e.Level == LevelWarn {
			prefix = "⚠"
		}
		fmt.Fprintf(s.Err, "  %s %s\n", prefix, e.Text)
	case ApprovalRequest, TurnDone:
		// chat 审批走 StdinAsker；TurnDone 无终端输出
	}
}
