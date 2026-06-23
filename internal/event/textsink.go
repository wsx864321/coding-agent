package event

import (
	"fmt"
	"io"
)

type TextSink struct {
	Out io.Writer
	Err io.Writer
}

func (s *TextSink) Emit(e Event) {
	switch e.Kind {
	case Text:
		io.WriteString(s.Out, e.Text)
	case ToolDispatch:
		fmt.Fprintf(s.Err, "  ⚡ %s\n", e.ToolName)
	case ToolResult:
		if e.ToolIsErr {
			fmt.Fprintf(s.Err, "  ✗ %s\n", e.ToolName)
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
