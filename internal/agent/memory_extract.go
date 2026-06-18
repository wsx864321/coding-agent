package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/wsx864321/coding-agent/internal/memory"
	"github.com/wsx864321/coding-agent/internal/provider"
)

const (
	DefaultMemoryExtractInterval  = 5 * time.Minute
	DefaultMemoryExtractThreshold = 5
	maxExtractMessages            = 12
	maxExtractNewMemories         = 3
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

func (a *Agent) maybeExtractMemories(ctx context.Context) {
	if a.memSet == nil || a.memSet.Store == nil {
		return
	}
	if !a.lastExtractTime.IsZero() && time.Since(a.lastExtractTime) < a.extractInterval {
		return
	}
	a.extractTurnCount++
	if a.extractTurnCount < a.memExtractThresh {
		return
	}
	a.extractTurnCount = 0

	msgs := a.preCompactSnapshot
	if len(msgs) == 0 {
		msgs = a.messages
	}

	start := len(msgs) - maxExtractMessages
	if start < 0 {
		start = 0
	}
	recent := msgs[start:]

	transcript := renderMessagesTranscript(recent)
	if strings.TrimSpace(transcript) == "" {
		return
	}

	existing := a.memSet.Store.ListActive("")
	existingList := formatExistingMemories(existing)

	extractPrompt := extractSystemPrompt
	if existingList != "" {
		extractPrompt += "\n\n已有记忆（避免重复）：\n" + existingList
	}
	extractPrompt += "\n\n对话片段：\n" + transcript

	a.lastExtractTime = time.Now()

	memories, err := a.extractWithLLM(ctx, extractPrompt)
	if err != nil || len(memories) == 0 {
		return
	}

	for _, m := range memories {
		if err := a.memSet.Store.Save(m); err != nil {
			continue
		}
		if a.memQueue != nil {
			a.memQueue.EnqueueSave(m)
		}
	}
}

// extractWithLLM 通过 Provider 流式接口提取记忆
func (a *Agent) extractWithLLM(ctx context.Context, prompt string) ([]memory.Memory, error) {
	sctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req := provider.Request{
		Model: a.cfg.Model,
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: prompt},
		},
		MaxTokens:   800,
		Temperature: 0,
	}
	ch, err := a.prov.Stream(sctx, req)
	if err != nil {
		return nil, err
	}
	msg, _, err := provider.Collect(ch)
	if err != nil {
		return nil, err
	}

	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil, nil
	}

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

func renderMessagesTranscript(msgs []provider.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		switch m.Role {
		case provider.RoleUser:
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
		case provider.RoleAssistant:
			s := strings.TrimSpace(m.Content)
			if s != "" {
				if len(s) > 400 {
					s = s[:400] + "..."
				}
				fmt.Fprintf(&b, "[助手]: %s\n", s)
			}
			for _, tc := range m.ToolCalls {
				fmt.Fprintf(&b, "[助手调用工具 %s]: %s\n", tc.Name, truncateStr(tc.Arguments, 200))
			}
		case provider.RoleTool:
			s := m.Content
			if strings.HasPrefix(s, "[历史工具结果已折叠") {
				continue
			}
			fmt.Fprintf(&b, "[工具 %s 输出]: %s\n", m.Name, truncateStr(s, 200))
		}
	}
	return b.String()
}

func formatExistingMemories(memories []memory.Memory) string {
	var b strings.Builder
	for _, m := range memories {
		fmt.Fprintf(&b, "- %s [%s]: %s\n", m.Name, m.Type, m.Description)
	}
	return b.String()
}

func extractJSONArray(content string) string {
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

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

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
