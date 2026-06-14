package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/wsx864321/coding-agent/internal/memory"
)

var slugRe = regexp.MustCompile(`[^\p{L}\p{N}-]+`)

// RememberTool 允许模型持久化事实到长期记忆
type RememberTool struct {
	Store *memory.Store
	Queue *memory.Queue
}

// NewRememberTool 创建 remember 工具
//
// store: 记忆存储实例
// queue: 中会话通知队列（用于在当前会话内即时生效）
func NewRememberTool(store *memory.Store, queue *memory.Queue) *RememberTool {
	return &RememberTool{Store: store, Queue: queue}
}

// SetStore 延迟注入 memory Store（供 WireMemoryTools 使用）
func (t *RememberTool) SetStore(s *memory.Store) { t.Store = s }

// SetQueue 延迟注入 memory Queue（供 WireMemoryTools 使用）
func (t *RememberTool) SetQueue(q *memory.Queue) { t.Queue = q }

func (t *RememberTool) ReadOnly() bool { return false }

func (t *RememberTool) Name() string { return "remember" }

func (t *RememberTool) Description() string {
	return "保存事实、偏好或约束到跨会话长期记忆。适合记录用户偏好、项目信息、重要约束。" +
		"每次调用保存一条记忆。更新已有记忆时使用相同的 name 即可覆盖。"
}

func (t *RememberTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "唯一标识名（kebab-case，如 prefers-tabs）。更新已有记忆时使用同名覆盖。",
			},
			"title": map[string]any{
				"type":        "string",
				"description": "人类可读的简短标签（用于索引显示）",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "单行摘要（用于索引和搜索，不超过一行）",
			},
			"type": map[string]any{
				"type":        "string",
				"enum":        []string{"user", "feedback", "project", "reference"},
				"description": "记忆类型：user=用户偏好，feedback=工作方式指导，project=项目事实，reference=外部指针",
			},
			"body": map[string]any{
				"type":        "string",
				"description": "事实的完整正文（Markdown）。应包含 why 和 how-to-apply 便于后续应用。",
			},
		},
		"required": []string{"name", "title", "description", "type", "body"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

func (t *RememberTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	if t.Store == nil {
		return "", fmt.Errorf("remember 工具未正确初始化（缺少 memory Store）")
	}

	name, _ := args["name"].(string)
	title, _ := args["title"].(string)
	desc, _ := args["description"].(string)
	typStr, _ := args["type"].(string)
	body, _ := args["body"].(string)

	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("name 不能为空")
	}
	if strings.TrimSpace(desc) == "" {
		return "", fmt.Errorf("description 不能为空")
	}

	// 标准化 name → kebab-case slug
	name = toSlug(name)

	var typ memory.Type
	switch strings.ToLower(typStr) {
	case "user":
		typ = memory.TypeUser
	case "feedback":
		typ = memory.TypeFeedback
	case "project":
		typ = memory.TypeProject
	case "reference":
		typ = memory.TypeReference
	default:
		return "", fmt.Errorf("未知记忆类型: %q，可选 user/feedback/project/reference", typStr)
	}

	m := memory.Memory{
		Name:        name,
		Title:       title,
		Description: desc,
		Type:        typ,
		Body:        body,
	}

	if err := t.Store.Save(m); err != nil {
		return "", fmt.Errorf("保存记忆失败: %w", err)
	}

	// 中会话通知
	if t.Queue != nil {
		t.Queue.EnqueueSave(m)
	}

	return fmt.Sprintf("已保存记忆 %q (type=%s)。", name, typ), nil
}

// toSlug 将字符串转换为 kebab-case slug
func toSlug(s string) string {
	s = strings.ToLower(s)
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "memory"
	}
	return s
}
