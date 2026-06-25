package jsonrpc

import (
	"encoding/json"
	"fmt"
)

// JSON-RPC 2.0 常量
const Version = "2.0"

// Message 是 JSON-RPC 2.0 的通用消息类型，同时表示请求、响应和通知。
//
// 请求：ID + Method + Params（Result 为空）
// 响应：ID + Result（Method 为空）
// 通知：Method + Params（ID 为 0）
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error 是 JSON-RPC 2.0 错误
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// NewRequest 构造一条 JSON-RPC 请求
func NewRequest(id int64, method string, params any) (*Message, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}
	return &Message{
		JSONRPC: Version,
		ID:      id,
		Method:  method,
		Params:  raw,
	}, nil
}

// NewNotify 构造一条 JSON-RPC 通知（无 id）
func NewNotify(method string, params any) (*Message, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}
	return &Message{
		JSONRPC: Version,
		Method:  method,
		Params:  raw,
	}, nil
}

// IsNotification 判断是否为通知（无 id）
func (m *Message) IsNotification() bool {
	return m.ID == 0 && m.Method != ""
}

// IsResponse 判断是否为响应（有 id，无 method）
func (m *Message) IsResponse() bool {
	return m.ID != 0 && m.Method == ""
}
