package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"github.com/wsx864321/coding-agent/internal/memory"
)

const (
	// DefaultMemoryExtractInterval 自动提取的最小时间间隔
	DefaultMemoryExtractInterval = 5 * time.Minute
	// DefaultMemoryExtractThreshold 累计轮数阈值，达到后触发提取
	DefaultMemoryExtractThreshold = 5
	// maxExtractMessages 每次提取时扫描的最近消息条数
	maxExtractMessages = 12
	// maxExtractNewMemories 单次提取最多新增记忆数
	maxExtractNewMemories = 3
)

const extractSystemPrompt = `你是一个编码助手的记忆提取模块。分析以下对话片段，识别值得持久化的事实。

返回 JSON 数组，每个元素为一个记忆对象：
- name: kebab-case 唯一标识（如 prefers-tabs）
- title: 人类可读简短标签
- description: 单行摘要
- type: 类型（user/feedback/project/reference）
- body: 完整正文，包含 why 和 how-to-apply

记忆类型说明：
- user: 用户身份、偏好、习惯、专长（跨项目全局）
- feedback: 工作方式指导、经验教训（跨项目全局）
- project: 项目事实、目标、约束、架构决策（项目专属）
- reference: 外部资源指针、URL、工单号（项目专属）

规则：
- 只在对话包含明确的新信息时才提取
- 避免重复已有记忆
- 避免提取临时性的讨论内容
- 如无新记忆，返回空数组 []`

// maybeExtractMemories 尝试从最近对话中提取长期记忆
//
// 触发条件：
//   - 至少间隔 extractInterval（默认 5 分钟）
//   - 累计轮数达到 memExtractThresh（默认 5 轮）
//
// 从 preCompactSnapshot 中取最近 N 条消息发送给 LLM 提取。
// 若 preCompactSnapshot 为空，则回退到当前 messages。
func (a *Agent) maybeExtractMemories(ctx context.Context) {
	if a.memSet == nil || a.memSet.Store == nil {
		return
	}

	// 节流：时间间隔
	if !a.lastExtractTime.IsZero() && time.Since(a.lastExtractTime) < a.extractInterval {
		return
	}

	// 节流：累计轮数
	a.extractTurnCount++
	if a.extractTurnCount < a.memExtractThresh {
		return
	}
	a.extractTurnCount = 0

	// 取消息源（优先压缩前快照）
	msgs := a.preCompactSnapshot
	if len(msgs) == 0 {
		msgs = a.messages
	}

	// 取最近 N 条消息
	start := len(msgs) - maxExtractMessages
	if start < 0 {
		start = 0
	}
	recent := msgs[start:]

	// 构建对话文本
	transcript := renderMessagesTranscript(recent)
	if strings.TrimSpace(transcript) == "" {
		return
	}

	// 读取已有记忆列表
	existing := a.memSet.Store.ListActive("")
	existingList := formatExistingMemories(existing)

	// 构建提取 prompt
	extractPrompt := extractSystemPrompt
	if existingList != "" {
		extractPrompt += "\n\n已有记忆（避免重复）：\n" + existingList
	}
	extractPrompt += "\n\n对话片段：\n" + transcript

	a.lastExtractTime = time.Now()

	// 调用 LLM 提取
	memories, err := a.extractWithLLM(ctx, extractPrompt)
	if err != nil || len(memories) == 0 {
		return
	}

	// 写入并通知
	for _, m := range memories {
		if err := a.memSet.Store.Save(m); err != nil {
			continue
		}
		if a.memQueue != nil {
			a.memQueue.EnqueueSave(m)
		}
	}
}

// extractWithLLM 用 LLM 从 prompt 中提取记忆
func (a *Agent) extractWithLLM(ctx context.Context, prompt string) ([]memory.Memory, error) {
	sctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req := openai.ChatCompletionRequest{
		Model: a.cfg.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
		MaxTokens:   800,
		Temperature: 0,
	}
	resp, err := a.client.CreateChatCompletion(sctx, req)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("提取接口返回空 choices")
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	if content == "" {
		return nil, nil
	}

	// 提取 JSON 数组（可能被 markdown code block 包裹）
	content = extractJSONArray(content)

	var raw []struct {
		Name        string `json:"name"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Type        string `json:"type"`
		Body        string `json:"body"`
	}
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, err
	}

	var out []memory.Memory
	for _, r := range raw {
		if strings.TrimSpace(r.Name) == "" || strings.TrimSpace(r.Description) == "" {
			continue
		}
		var typ memory.Type
		switch strings.ToLower(r.Type) {
		case "user":
			typ = memory.TypeUser
		case "feedback":
			typ = memory.TypeFeedback
		case "project":
			typ = memory.TypeProject
		case "reference":
			typ = memory.TypeReference
		default:
			continue
		}
		out = append(out, memory.Memory{
			Name:        toSlugStr(r.Name),
			Title:       r.Title,
			Description: r.Description,
			Type:        typ,
			Body:        r.Body,
		})
		if len(out) >= maxExtractNewMemories {
			break
		}
	}
	return out, nil
}

// renderMessagesTranscript 将消息列表渲染为对话文本
func renderMessagesTranscript(msgs []openai.ChatCompletionMessage) string {
	var b strings.Builder
	for _, m := range msgs {
		switch m.Role {
		case openai.ChatMessageRoleUser:
			if strings.HasPrefix(strings.TrimSpace(m.Content), "<memory-update>") {
				continue
			}
			if strings.HasPrefix(strings.TrimSpace(m.Content), "<compaction-summary>") {
				continue
			}
			s := strings.TrimSpace(m.Content)
			if len(s) > 400 {
				s = s[:400] + "..."
			}
			fmt.Fprintf(&b, "[用户]: %s\n", s)
		case openai.ChatMessageRoleAssistant:
			s := strings.TrimSpace(m.Content)
			if s != "" {
				if len(s) > 400 {
					s = s[:400] + "..."
				}
				fmt.Fprintf(&b, "[助手]: %s\n", s)
			}
			for _, tc := range m.ToolCalls {
				fmt.Fprintf(&b, "[助手调用工具 %s]: %s\n", tc.Function.Name, truncateStr(tc.Function.Arguments, 200))
			}
		case openai.ChatMessageRoleTool:
			s := m.Content
			if strings.HasPrefix(s, "[历史工具结果已折叠") {
				continue
			}
			fmt.Fprintf(&b, "[工具 %s 输出]: %s\n", m.Name, truncateStr(s, 200))
		}
	}
	return b.String()
}

// formatExistingMemories 格式化已有记忆列表
func formatExistingMemories(memories []memory.Memory) string {
	var b strings.Builder
	for _, m := range memories {
		fmt.Fprintf(&b, "- %s [%s]: %s\n", m.Name, m.Type, m.Description)
	}
	return b.String()
}

// extractJSONArray 从可能包含 markdown 代码块的文本中提取 JSON 数组
func extractJSONArray(content string) string {
	// 尝试找到 [ 开头
	content = strings.TrimSpace(content)
	start := strings.Index(content, "[")
	if start < 0 {
		return content
	}
	end := strings.LastIndex(content, "]")
	if end < 0 || end <= start {
		return content
	}
	return content[start : end+1]
}

// truncateStr 截断字符串
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// toSlugStr 将字符串转换为 kebab-case slug
func toSlugStr(s string) string {
	s = strings.ToLower(s)
	s = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, s)
	s = strings.Trim(s, "-")
	if s == "" {
		s = "memory"
	}
	return s
}
