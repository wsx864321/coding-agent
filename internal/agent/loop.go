package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	openai "github.com/sashabaranov/go-openai"

	"github.com/wsx864321/coding-agent/internal/permission"
)

// ErrMaxTurnsExceeded 当 loop 超过 Config.MaxTurns 时返回
var ErrMaxTurnsExceeded = errors.New("agent loop 超过最大轮数")

// loopStep 跑一次"调用 LLM → 处理 tool_calls → 收集 tool 结果"循环。
// 一次 loopStep 可能消耗 0 轮（直接拿到 final answer）或 1 轮（带 tool_calls）。
//
// 返回 final assistant content 表示结束。
// 返回 (空, nil) + turnConsumed=true 表示还有下一步。
// 返回 err 表示 API / 工具调用出错。
func (a *Agent) loopStep(ctx context.Context) (final string, err error) {
	req, err := a.buildRequest()
	if err != nil {
		return "", err
	}

	resp, err := a.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("调用 LLM 失败: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("LLM 返回的 choices 为空")
	}

	choice := resp.Choices[0]

	// DeepSeek 等 API 要求 assistant 消息必须有 content 字段，
	// 但 tool_calls 场景下 content 可能为空字符串，
	// go-openai 库的 omitempty tag 会将其省略导致请求被拒。
	msg := choice.Message
	if msg.Content == "" && len(msg.ToolCalls) > 0 {
		msg.Content = " " // 填充空格以保留 content 字段
	}
	// 将 assistant 消息原样存入历史（保留 ToolCalls）
	a.messages = append(a.messages, msg)

	// 无 tool_calls：LLM 已给出最终答案，进入 Stop 阶段
	if len(msg.ToolCalls) == 0 {
		// Stop hook：若首个返回 force 的 hook 强制续跑，
		// 把 force 作为 user 消息追加到历史并清空 final 信号，
		// 让 Run 入口继续 loopStep
		if a.hooks != nil {
			force, ok := a.hooks.TriggerStop(ctx, a.messages)
			if ok {
				a.messages = append(a.messages, openai.ChatCompletionMessage{
					Role:    openai.ChatMessageRoleUser,
					Content: force,
				})
				return "", nil
			}
		}
		return msg.Content, nil
	}

	// 串行执行 tool_calls；单个失败不中断后续，结果以 "Error: ..." 形式回填
	for _, tc := range choice.Message.ToolCalls {
		a.executeToolCall(ctx, tc)
	}
	return "", nil
}

// executeToolCall 解析 tool_call 的 JSON 参数并通过 registry 执行，结果作为 tool message 回填
func (a *Agent) executeToolCall(ctx context.Context, tc openai.ToolCall) {
	result := a.invokeTool(ctx, tc)
	if result == "" {
		result = " " // DeepSeek 等 API 要求 content 字段必须存在
	}
	a.messages = append(a.messages, openai.ChatCompletionMessage{
		Role:       openai.ChatMessageRoleTool,
		ToolCallID: tc.ID,
		Content:    result,
	})
}

// invokeTool 真正执行工具调用，返回结果字符串（成功或失败都返回字符串）
//
// 调用顺序：
//  1. PreToolUse hook（如果注册）→ 首个返回非空 block 的 hook 阻断
//  2. permission.Pipeline（Deny / Allow）→ 硬约束（不受 hook 放行影响）
//  3. registry 查表 + Execute
//  4. PostToolUse hook → 副作用（日志 / 截断 / 统计）
//
//	+----------+   +-----------------+   +-------------+   +----------+   +--------+   +-----------------+
//	| tc 进入  | ->| PreToolUse hook | ->| permission  | ->| registry | ->| exec   | ->| PostToolUse hook|
//	|          |   |  (可阻断)        |   |  Pipeline   |   | lookup   |   |        |   |  (纯副作用)     |
//	+----------+   +-----------------+   +-------------+   +----------+   +--------+   +-----------------+
//	                                     |
//	                                     +--> Deny → 把拒绝原因回填给 LLM（不调 Execute）
//	                                     +--> Allow → 继续 registry 查表 + Execute
func (a *Agent) invokeTool(ctx context.Context, tc openai.ToolCall) string {
	// 解析 JSON 参数字符串为 map[string]any（permission / hook 都要用）
	var args map[string]any
	if tc.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return fmt.Sprintf("Error: 参数解析失败: %v (raw=%s)", err, tc.Function.Arguments)
		}
	}
	name := tc.Function.Name

	// Stage 1: PreToolUse hook（首个返回非空 block 的 hook 短路）
	if a.hooks != nil {
		if blocked, reason := a.hooks.TriggerPreToolUse(ctx, name, args); blocked {
			return fmt.Sprintf("Blocked by hook: %s", reason)
		}
	}

	// Stage 2: system permission.Pipeline（硬约束，不可被 hook 覆盖）
	if a.checker != nil {
		r := a.checker.Check(ctx, name, args)
		if r.Decision == permission.DecisionDeny {
			return fmt.Sprintf("Permission denied: %s", r.Reason)
		}
	}

	tool := a.registry.Get(name)
	if tool == nil {
		return fmt.Sprintf("Error: 工具 %q 未注册", name)
	}

	out, err := tool.Execute(ctx, args)

	// 记录工具调用凭证到证据账本（无论成功失败都记录）
	if a.ledger != nil {
		a.ledger.Record(name, args, out, err == nil)
	}

	if err != nil {
		// 即便 Execute 失败，也走 PostToolUse hook（让日志/统计 hook 看到真实输出）
		if a.hooks != nil {
			a.hooks.TriggerPostToolUse(ctx, name, args, fmt.Sprintf("Error: %v", err))
		}
		return fmt.Sprintf("Error: %v", err)
	}

	// Stage 3: PostToolUse hook
	if a.hooks != nil {
		a.hooks.TriggerPostToolUse(ctx, name, args, out)
	}
	return out
}

// buildRequest 根据当前 messages 构造 ChatCompletionRequest
func (a *Agent) buildRequest() (openai.ChatCompletionRequest, error) {
	req := openai.ChatCompletionRequest{
		Model:    a.cfg.Model,
		Messages: a.messages,
		Tools:    a.buildTools(),
	}
	if a.cfg.MaxTokens > 0 {
		req.MaxTokens = a.cfg.MaxTokens
	}
	if a.cfg.Temperature > 0 {
		req.Temperature = a.cfg.Temperature
	}
	return req, nil
}

// buildTools 把 registry 中的工具转换为 openai.Tool 列表
func (a *Agent) buildTools() []openai.Tool {
	toolList := a.registry.List()
	tools := make([]openai.Tool, 0, len(toolList))
	for _, t := range toolList {
		tools = append(tools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Schema(),
			},
		})
	}

	return tools
}
