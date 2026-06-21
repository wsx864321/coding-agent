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

// streamClosedMsg 在流通道关闭且未收到显式完成/错误时触发。
type streamClosedMsg struct{}
