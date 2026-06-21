package tui

// Role 标识消息来源。
type Role int

const (
	RoleUser Role = iota
	RoleAssistant
	RoleSystem
)

// Message 是聊天区的一条消息。
type Message struct {
	Role    Role
	Content string
}
