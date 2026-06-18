package provider

import "encoding/json"

// Role 标识消息角色
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message 是跨 provider 的统一消息类型
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolCall 描述一次工具调用请求
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolSchema 描述一个工具的 schema（供 LLM 使用）
type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Request 是发送给 Provider 的统一请求
type Request struct {
	Model       string
	Messages    []Message
	Tools       []ToolSchema
	MaxTokens   int
	Temperature float32
}

// ChunkType 标识流式响应中不同类型的增量
type ChunkType int

const (
	ChunkText          ChunkType = iota // 文本内容增量
	ChunkToolCallStart                  // 工具调用开始（携带 ID + Name）
	ChunkToolCallDelta                  // 工具调用参数增量
	ChunkUsage                          // 用量统计
	ChunkDone                           // 流结束
	ChunkError                          // 流中错误
)

// Chunk 是流式响应的一个增量片段
type Chunk struct {
	Type     ChunkType
	Text     string    // ChunkText
	ToolCall *ToolCall // ChunkToolCallStart / ChunkToolCallDelta
	Usage    *Usage    // ChunkUsage
	Err      error     // ChunkError
}

// Usage 统计本次请求的 token 用量
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	FinishReason     string
}

// FinishReason 常量
const (
	FinishReasonStop      = "stop"
	FinishReasonToolCalls = "tool_calls"
	FinishReasonLength    = "length"
)
