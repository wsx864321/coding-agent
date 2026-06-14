package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wsx864321/coding-agent/internal/memory"
)

// ForgetTool 允许模型删除长期记忆（实际执行软删除，归档而非销毁）
type ForgetTool struct {
	Store *memory.Store
	Queue *memory.Queue
}

// NewForgetTool 创建 forget 工具
func NewForgetTool(store *memory.Store, queue *memory.Queue) *ForgetTool {
	return &ForgetTool{Store: store, Queue: queue}
}

// SetStore 延迟注入 memory Store（供 WireMemoryTools 使用）
func (t *ForgetTool) SetStore(s *memory.Store) { t.Store = s }

// SetQueue 延迟注入 memory Queue（供 WireMemoryTools 使用）
func (t *ForgetTool) SetQueue(q *memory.Queue) { t.Queue = q }

func (t *ForgetTool) ReadOnly() bool { return false }

func (t *ForgetTool) Name() string { return "forget" }

func (t *ForgetTool) Description() string {
	return "删除一条长期记忆。记忆会被归档到 .archive/ 目录（软删除），可从索引中恢复。" +
		"适用于记忆过时或错误时使用。"
}

func (t *ForgetTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "要删除的记忆 name（kebab-case slug）",
			},
		},
		"required": []string{"name"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

func (t *ForgetTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	if t.Store == nil {
		return "", fmt.Errorf("forget 工具未正确初始化（缺少 memory Store）")
	}

	name, _ := args["name"].(string)
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("name 不能为空")
	}

	name = strings.ToLower(strings.TrimSpace(name))

	if err := t.Store.Delete(name); err != nil {
		return "", fmt.Errorf("删除记忆失败: %w", err)
	}

	// 中会话通知
	if t.Queue != nil {
		t.Queue.EnqueueDelete(name)
	}

	return fmt.Sprintf("已归档记忆 %q。", name), nil
}
