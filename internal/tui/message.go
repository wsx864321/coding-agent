package tui

// EntryKind 标识 transcript 条目类型。
type EntryKind int

const (
	EntryUserMessage EntryKind = iota
	EntryAssistantChunk
	EntryToolCard
	EntryToolOutput
	EntryError
	EntryReasoning
	EntryToolStream
)

// TranscriptEntry 是聊天区的一条结构化 transcript 条目。
type TranscriptEntry struct {
	Kind    EntryKind
	Content string // pre-rendered ANSI
	Raw     string // raw for re-render on resize
}
