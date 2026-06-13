package tools

import (
	"context"
	"encoding/json"
)

// Tool 定义了可执行工具的接口，提供工具的名称、描述、参数模式及执行能力
type Tool interface {
	// Name 返回工具的名称
	Name() string
	// Description 返回工具的功能描述
	Description() string
	// Schema 返回工具参数的 JSON Schema
	Schema() json.RawMessage
	// Execute 使用给定的参数执行工具并返回结果
	Execute(ctx context.Context, args map[string]any) (string, error)
	// ReadOnly 返回该工具是否没有可观察到的副作用。
	// 连续出现的只读工具可以在同一个并行批次中并发执行。
	ReadOnly() bool
}
