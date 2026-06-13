package agent

import (
	"fmt"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// ensureToolMessageLinks 修复历史中的孤儿 tool 消息，避免 provider 拒绝：
// "role=tool 必须对应前面的 assistant tool_calls"。
//
// 处理策略：
// 1) 若最近的非 tool 消息就是包含对应 tool_call_id 的 assistant，保持原样；
// 2) 否则尝试插入一条合成 assistant(tool_calls) 进行修复；
// 3) 若 tool_call_id 为空无法关联，则把 tool 消息降级为 user 说明消息。
func (a *Agent) ensureToolMessageLinks() {
	if len(a.messages) == 0 {
		return
	}
	fixed := make([]openai.ChatCompletionMessage, 0, len(a.messages)+4)
	changed := false

	for _, m := range a.messages {
		if m.Role != openai.ChatMessageRoleTool {
			fixed = append(fixed, m)
			continue
		}

		prev := previousNonTool(fixed)
		if prev != nil && prev.Role == openai.ChatMessageRoleAssistant &&
			assistantHasToolCallID(*prev, m.ToolCallID) {
			fixed = append(fixed, m)
			continue
		}

		changed = true
		if strings.TrimSpace(m.ToolCallID) == "" {
			fixed = append(fixed, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: fmt.Sprintf("[检测到无法关联的历史工具输出（工具=%s），已转为说明文本以保证会话结构合法。]\n%s", strings.TrimSpace(m.Name), m.Content),
			})
			continue
		}

		name := strings.TrimSpace(m.Name)
		if name == "" {
			name = "unknown_tool"
		}
		fixed = append(fixed, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: " ",
			ToolCalls: []openai.ToolCall{
				{
					ID:   m.ToolCallID,
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      name,
						Arguments: "{}",
					},
				},
			},
		})
		fixed = append(fixed, m)
	}

	if changed {
		a.messages = fixed
	}
}

func previousNonTool(msgs []openai.ChatCompletionMessage) *openai.ChatCompletionMessage {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != openai.ChatMessageRoleTool {
			return &msgs[i]
		}
	}
	return nil
}

func assistantHasToolCallID(msg openai.ChatCompletionMessage, toolCallID string) bool {
	if msg.Role != openai.ChatMessageRoleAssistant || len(msg.ToolCalls) == 0 {
		return false
	}
	id := strings.TrimSpace(toolCallID)
	if id == "" {
		return true
	}
	for _, tc := range msg.ToolCalls {
		if tc.ID == id {
			return true
		}
	}
	return false
}
