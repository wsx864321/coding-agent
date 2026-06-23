package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/wsx864321/coding-agent/internal/permission"
	"github.com/wsx864321/coding-agent/internal/provider"
	"github.com/wsx864321/coding-agent/internal/tools"
)

// ErrMaxTurnsExceeded 当 loop 超过 Config.MaxTurns 时返回
var ErrMaxTurnsExceeded = errors.New("agent loop 超过最大轮数")

const (
	maxStreamRecoveries = 1
)

// loopStep 跑一次"调用 LLM → 处理 tool_calls → 收集 tool 结果"循环。
// 返回 final assistant content 表示结束；返回 (空, nil) 表示还有下一步。
func (a *Agent) loopStep(ctx context.Context) (final string, err error) {
	return a.loopStepWithText(ctx, EmitterFromContext(ctx))
}

// loopStepWithText 与 loopStep 相同，但在流式收集时可推送文本增量与工具事件。
func (a *Agent) loopStepWithText(ctx context.Context, emitter StreamEmitter) (final string, err error) {
	onText := func(s string) {
		if emitter != nil {
			emitter.OnChunk(s)
		}
	}
	// 修复历史里的孤儿 tool 消息，避免 provider 400
	a.messages = provider.SanitizeToolPairing(a.messages)

	a.maybeCompact(ctx, a.lastPromptTokens)

	req := a.buildRequest()

	ch, err := a.prov.Stream(ctx, req)
	if err != nil {
		// 上下文溢出恢复：捕获 prompt_too_long 并做 reactive compact
		if provider.IsPromptTooLong(err) && !a.hasAttemptedReactiveCompact {
			a.reactiveCompact(ctx)
			a.hasAttemptedReactiveCompact = true
			return "", nil // 回到 Run 的 for 循环重试
		}
		return "", fmt.Errorf("调用 LLM 失败: %w", err)
	}

	msg, usage, streamErr := provider.CollectWithText(ch, onText)
	if streamErr != nil {
		// 流中断恢复：如果已有部分输出且在恢复次数内，注入恢复 prompt 重试
		if provider.IsStreamInterrupted(streamErr) {
			if strings.TrimSpace(msg.Content) != "" {
				a.messages = append(a.messages, provider.Message{
					Role:    provider.RoleAssistant,
					Content: msg.Content,
				})
			}
			a.messages = append(a.messages, provider.Message{
				Role:    provider.RoleUser,
				Content: "[流式传输中断] 请从断点继续，不要重复已输出的内容。",
			})
			return "", nil
		}
		// 上下文溢出也可能在流式阶段抛出
		if provider.IsPromptTooLong(streamErr) && !a.hasAttemptedReactiveCompact {
			a.reactiveCompact(ctx)
			a.hasAttemptedReactiveCompact = true
			return "", nil
		}
		return "", fmt.Errorf("调用 LLM 失败: %w", streamErr)
	}

	// 缓存真实 PromptTokens
	if usage != nil && usage.PromptTokens > 0 {
		a.lastPromptTokens = usage.PromptTokens
	}

	// 将 assistant 消息原样存入历史
	a.messages = append(a.messages, msg)

	// 无 tool_calls：LLM 已给出最终答案
	if len(msg.ToolCalls) == 0 {
		if a.memSet != nil {
			a.maybeExtractMemories(ctx)
		}

		if force := a.checkTodoGuard(ctx); force != "" {
			a.messages = append(a.messages, provider.Message{
				Role:    provider.RoleUser,
				Content: force,
			})
			return "", nil
		}

		if a.hooks != nil {
			force, ok := a.hooks.Stop(ctx, a.messages)
			if ok {
				a.messages = append(a.messages, provider.Message{
					Role:    provider.RoleUser,
					Content: force,
				})
				return "", nil
			}
		}
		return msg.Content, nil
	}

	// 执行 tool_calls
	a.executeBatch(ctx, msg.ToolCalls)
	return "", nil
}

// reactiveCompact 紧急压缩：当 prompt_too_long 时执行一次最小化压缩
func (a *Agent) reactiveCompact(ctx context.Context) {
	_, _ = a.compactHistory(ctx, "reactive", "", true)
}

// compactFocusFromArgs 从 compact 工具的参数中提取 focus 字段。
func compactFocusFromArgs(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	var in struct {
		Focus string `json:"focus"`
	}
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return ""
	}
	return strings.TrimSpace(in.Focus)
}

// invokeTool 真正执行工具调用，返回结果字符串（成功或失败都返回字符串）
func (a *Agent) invokeTool(ctx context.Context, tc provider.ToolCall, emitter StreamEmitter) string {
	name := tc.Name

	if emitter != nil {
		emitter.OnToolStart(name, tc.Arguments)
	}

	var result string
	defer func() {
		if emitter != nil {
			isErr := strings.HasPrefix(result, "Error:") || strings.HasPrefix(result, "Permission denied")
			emitter.OnToolEnd(name, result, isErr)
		}
	}()

	var args map[string]any
	if tc.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
			result = fmt.Sprintf("Error: 参数解析失败: %v (raw=%s)", err, tc.Arguments)
			return result
		}
	}

	if a.hooks != nil {
		if blocked, msg := a.hooks.PreToolUse(ctx, name, args); blocked {
			result = fmt.Sprintf("Blocked by hook: %s", msg)
			return result
		}
	}

	if a.checker != nil {
		r := a.checker.Check(ctx, name, args)
		if r.Decision == permission.DecisionDeny {
			result = fmt.Sprintf("Permission denied: %s", r.Reason)
			return result
		}
	}

	tool := a.registry.Get(name)
	if tool == nil {
		result = fmt.Sprintf("Error: 工具 %q 未注册", name)
		return result
	}

	out, err := tool.Execute(ctx, args)

	if a.ledger != nil {
		a.ledger.Record(name, args, out, err == nil)
	}

	if err != nil {
		if a.hooks != nil {
			a.hooks.PostToolUse(ctx, name, args, fmt.Sprintf("Error: %v", err))
		}
		result = fmt.Sprintf("Error: %v", err)
		return result
	}

	if a.hooks != nil {
		a.hooks.PostToolUse(ctx, name, args, out)
	}
	result = out
	return result
}

// buildRequest 根据当前 messages 构造 provider.Request
func (a *Agent) buildRequest() provider.Request {
	req := provider.Request{
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
	return req
}

// buildTools 把 registry 中的工具转换为 provider.ToolSchema 列表
func (a *Agent) buildTools() []provider.ToolSchema {
	toolList := a.registry.List()
	schemas := make([]provider.ToolSchema, 0, len(toolList))
	for _, t := range toolList {
		schemas = append(schemas, provider.ToolSchema{
			Name:        t.Name(),
			Description: t.Description(),
			Parameters:  t.Schema(),
		})
	}
	return schemas
}

// maxParallelTools 是单批并行工具调用的最大 goroutine 数。
const maxParallelTools = 8

// toolCallBatch 描述一组 tool_calls 的执行策略。
type toolCallBatch struct {
	start    int
	end      int
	parallel bool
}

// executeBatch 分区执行一批 tool_calls
func (a *Agent) executeBatch(ctx context.Context, calls []provider.ToolCall) {
	results := make([]string, len(calls))
	emitter := EmitterFromContext(ctx)

	for _, batch := range partitionToolCalls(a.registry, calls) {
		if batch.parallel && batch.end-batch.start > 1 {
			runParallel(batch.start, batch.end, func(i int) {
				results[i] = a.invokeTool(ctx, calls[i], emitter)
			})
			continue
		}
		for i := batch.start; i < batch.end; i++ {
			results[i] = a.invokeTool(ctx, calls[i], emitter)
		}
	}

	for i, tc := range calls {
		result := results[i]
		if tc.Name == "compact" {
			focus := compactFocusFromArgs(tc.Arguments)
			if err := a.CompactNow(ctx, focus); err != nil {
				result = fmt.Sprintf("Error: 手动压缩失败: %v", err)
			} else {
				result = "手动压缩已完成。"
			}
		}
		if result == "" {
			result = " "
		}
		a.messages = append(a.messages, provider.Message{
			Role:       provider.RoleTool,
			ToolCallID: tc.ID,
			Name:       tc.Name,
			Content:    result,
		})
	}
}

// partitionToolCalls 将 tool_calls 按可并行性切分为多个批次
func partitionToolCalls(r *tools.Registry, calls []provider.ToolCall) []toolCallBatch {
	var batches []toolCallBatch
	for i := 0; i < len(calls); {
		if isParallelisable(r, calls[i].Name) {
			start := i
			i++
			for i < len(calls) && isParallelisable(r, calls[i].Name) {
				i++
			}
			batches = append(batches, toolCallBatch{start: start, end: i, parallel: true})
			continue
		}
		batches = append(batches, toolCallBatch{start: i, end: i + 1})
		i++
	}
	return batches
}

func isParallelisable(r *tools.Registry, name string) bool {
	t := r.Get(name)
	if t == nil {
		return false
	}
	return t.ReadOnly()
}

// runParallel 在 [start, end) 范围内并发执行 run(i)，最多 maxParallelTools 个 goroutine。
// 包含 panic 恢复，防止单个工具 panic 崩溃整个 agent。
func runParallel(start, end int, run func(int)) {
	sem := make(chan struct{}, maxParallelTools)
	results := make([]string, end-start)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := start; i < end; i++ {
		i := i
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					mu.Lock()
					results[i-start] = fmt.Sprintf("Error: panic in tool: %v", r)
					mu.Unlock()
				}
			}()
			run(i)
		}()
	}
	wg.Wait()
}
