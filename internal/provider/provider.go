package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Provider 是 LLM 后端的核心抽象（策略模式接口）。
//
// 所有后端（OpenAI、Anthropic 等）都实现此接口。
// 只暴露流式方法——非流式场景通过 Collect 辅助函数收集全部 Chunk。
type Provider interface {
	// Name 返回 provider 实例名称，如 "openai"、"anthropic"
	Name() string

	// Stream 发起流式 completion 请求，返回 Chunk channel。
	// 取消 ctx 会中止底层请求；channel 关闭标志流结束。
	Stream(ctx context.Context, req Request) (<-chan Chunk, error)
}

// Factory 根据 Config 构建 Provider 实例
type Factory func(cfg Config) (Provider, error)

// Config 是解析后的 provider 实例配置
type Config struct {
	Name    string // 实例名，如 "openai"、"anthropic"
	BaseURL string // API 端点
	Model   string // 模型 ID
	APIKey  string // 解析后的 API key
	KeyEnv  string // API key 来源的环境变量名
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Factory{}
)

// Register 注册一个 provider 后端。kind 是唯一标识（如 "openai"、"anthropic"）。
// 重复注册相同 kind 会 panic。
func Register(kind string, f Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[kind]; dup {
		panic(fmt.Sprintf("provider: duplicate registration for %q", kind))
	}
	registry[kind] = f
}

// New 根据 kind 查找已注册的 Factory 并构建 Provider
func New(kind string, cfg Config) (Provider, error) {
	registryMu.RLock()
	f, ok := registry[kind]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("provider: 未知的 provider 类型 %q（已注册: %s）",
			kind, strings.Join(Kinds(), ", "))
	}
	return f(cfg)
}

// Kinds 返回所有已注册的 provider 类型名，按字母排序
func Kinds() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Collect 消费流式 channel，收集为完整的 assistant Message + Usage。
// 适用于不需要流式展示的场景（如压缩摘要、记忆提取）。
func Collect(ch <-chan Chunk) (Message, *Usage, error) {
	return CollectWithText(ch, nil)
}

// CollectWithText 与 Collect 相同，但在收到文本增量时可选调用 onText。
func CollectWithText(ch <-chan Chunk, onText func(string)) (Message, *Usage, error) {
	var text strings.Builder
	var toolCalls []ToolCall
	var usage *Usage
	currentTCIndex := -1

	for chunk := range ch {
		switch chunk.Type {
		case ChunkText:
			text.WriteString(chunk.Text)
			if onText != nil && chunk.Text != "" {
				onText(chunk.Text)
			}
		case ChunkToolCallStart:
			if chunk.ToolCall != nil {
				toolCalls = append(toolCalls, *chunk.ToolCall)
				currentTCIndex = len(toolCalls) - 1
			}
		case ChunkToolCallDelta:
			if chunk.ToolCall != nil && currentTCIndex >= 0 && currentTCIndex < len(toolCalls) {
				toolCalls[currentTCIndex].Arguments += chunk.ToolCall.Arguments
			}
		case ChunkUsage:
			usage = chunk.Usage
		case ChunkError:
			return Message{}, nil, chunk.Err
		case ChunkDone:
			// 流正常结束
		}
	}

	msg := Message{
		Role:      RoleAssistant,
		Content:   text.String(),
		ToolCalls: toolCalls,
	}
	return msg, usage, nil
}

// SanitizeToolPairing 修复消息历史中的配对问题，确保：
// 1. 每条 tool 消息都紧跟在包含对应 tool_call_id 的 assistant 消息之后
// 2. 每条含 tool_calls 的 assistant 消息都有对应的 tool result 跟随
func SanitizeToolPairing(msgs []Message) []Message {
	if len(msgs) == 0 {
		return msgs
	}
	fixed := make([]Message, 0, len(msgs)+4)
	changed := false

	for _, m := range msgs {
		if m.Role != RoleTool {
			fixed = append(fixed, m)
			continue
		}

		prev := previousNonToolMsg(fixed)
		if prev != nil && prev.Role == RoleAssistant && msgHasToolCallID(*prev, m.ToolCallID) {
			fixed = append(fixed, m)
			continue
		}

		changed = true
		if strings.TrimSpace(m.ToolCallID) == "" {
			fixed = append(fixed, Message{
				Role: RoleUser,
				Content: fmt.Sprintf("[检测到无法关联的历史工具输出（工具=%s），已转为说明文本。]\n%s",
					strings.TrimSpace(m.Name), m.Content),
			})
			continue
		}

		name := strings.TrimSpace(m.Name)
		if name == "" {
			name = "unknown_tool"
		}
		fixed = append(fixed, Message{
			Role:    RoleAssistant,
			Content: " ",
			ToolCalls: []ToolCall{
				{
					ID:        m.ToolCallID,
					Name:      name,
					Arguments: "{}",
				},
			},
		})
		fixed = append(fixed, m)
	}

	fixed = patchUnmatchedToolCalls(fixed, &changed)

	if changed {
		return fixed
	}
	return msgs
}

// patchUnmatchedToolCalls 为含 tool_calls 但缺少对应 tool result 的 assistant
// 消息补充占位 tool result，防止 API 返回 400 错误。
func patchUnmatchedToolCalls(msgs []Message, changed *bool) []Message {
	result := make([]Message, 0, len(msgs)+4)
	for i, m := range msgs {
		result = append(result, m)
		if m.Role != RoleAssistant || len(m.ToolCalls) == 0 {
			continue
		}
		following := collectFollowingToolIDs(msgs, i+1)
		for _, tc := range m.ToolCalls {
			if _, ok := following[tc.ID]; !ok {
				*changed = true
				result = append(result, Message{
					Role:       RoleTool,
					ToolCallID: tc.ID,
					Name:       tc.Name,
					Content:    "[工具调用被中断，未获得结果]",
				})
			}
		}
	}
	return result
}

// collectFollowingToolIDs 收集紧跟在 assistant 消息之后的所有 tool result 的 ID 集合。
func collectFollowingToolIDs(msgs []Message, start int) map[string]struct{} {
	ids := make(map[string]struct{})
	for j := start; j < len(msgs); j++ {
		if msgs[j].Role != RoleTool {
			break
		}
		if id := strings.TrimSpace(msgs[j].ToolCallID); id != "" {
			ids[id] = struct{}{}
		}
	}
	return ids
}

func previousNonToolMsg(msgs []Message) *Message {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != RoleTool {
			return &msgs[i]
		}
	}
	return nil
}

func msgHasToolCallID(msg Message, toolCallID string) bool {
	if msg.Role != RoleAssistant || len(msg.ToolCalls) == 0 {
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
