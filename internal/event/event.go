package event

type Kind int

const (
	Text Kind = iota
	ToolDispatch
	ToolResult
	ApprovalRequest
	TurnDone
	Notice
)

type Level int

const (
	LevelInfo Level = iota
	LevelWarn
)

type Event struct {
	Kind  Kind
	Level Level

	Text string

	ToolName   string
	ToolArgs   string
	ToolOutput string
	ToolIsErr  bool

	ApprovalName    string
	ApprovalArgs    map[string]any
	ApprovalRespond func(bool)

	Err error
}
