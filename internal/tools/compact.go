package tools

import (
	"context"
	"encoding/json"
)

// CompactTool 允许模型主动请求会话压缩。
// 真实压缩逻辑在 agent.executeToolCall 中处理，这里仅提供 schema 与调用入口。
type CompactTool struct{}

func NewCompactTool() *CompactTool { return &CompactTool{} }

func (t *CompactTool) Name() string { return "compact" }

func (t *CompactTool) Description() string {
	return "请求主机压缩当前会话上下文，可选 focus 用于强调摘要保留重点。"
}

func (t *CompactTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"focus": map[string]any{
				"type":        "string",
				"description": "压缩摘要时优先保留的重点信息",
			},
		},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

func (t *CompactTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	return "已请求手动压缩。", nil
}
