package tui

// StreamChunkMsg 表示 assistant 回复的一个文本增量。
type StreamChunkMsg struct {
	Text string
}

// StreamDoneMsg 表示当前轮流式输出已完成。
type StreamDoneMsg struct{}

// StreamErrorMsg 表示当前轮执行失败。
type StreamErrorMsg struct {
	Err error
}

// ToolStartMsg 表示工具调用开始。
type ToolStartMsg struct {
	Name string
	Args string
}

// ToolEndMsg 表示工具调用结束。
type ToolEndMsg struct {
	Name    string
	Result  string
	IsError bool
}

// ApprovalRequestMsg 表示需要用户审批的工具调用。
type ApprovalRequestMsg struct {
	Name    string
	Args    map[string]any
	Respond func(bool)
}

// streamClosedMsg 在流通道关闭且未收到显式完成/错误时触发。
type streamClosedMsg struct{}
