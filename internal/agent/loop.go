package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	openai "github.com/sashabaranov/go-openai"

	"github.com/wsx864321/coding-agent/internal/permission"
	"github.com/wsx864321/coding-agent/internal/tools"
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
	// 每轮 LLM 调用前先做低成本上下文整理；必要时触发自动摘要压缩。
	a.maybeCompact(ctx)
	// 兜底修复历史里的孤儿 tool 消息，避免 provider 400。
	a.ensureToolMessageLinks()

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
		// 尝试自动提取长期记忆（在 Stop hook 之前，确保最终返回是 LLM 的答案）
		if a.memSet != nil {
			a.maybeExtractMemories(ctx)
		}

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

	// 分区并行执行 tool_calls：连续只读工具并发执行，写工具串行分隔。
	// 单个失败不中断后续，结果以 "Error: ..." 形式回填。
	a.executeBatch(ctx, choice.Message.ToolCalls)
	return "", nil
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
//
// 调用顺序：
//
//  1. PreToolUse hook（如果注册）→ 首个返回非空 block 的 hook 阻断
//
//  2. permission.Pipeline（Deny / Allow）→ 硬约束（不受 hook 放行影响）
//
//  3. registry 查表 + Execute
//
//  4. PostToolUse hook → 副作用（日志 / 截断 / 统计）
//
//     +----------+   +-----------------+   +-------------+   +----------+   +--------+   +-----------------+
//     | tc 进入  | ->| PreToolUse hook | ->| permission  | ->| registry | ->| exec   | ->| PostToolUse hook|
//     |          |   |  (可阻断)        |   |  Pipeline   |   | lookup   |   |        |   |  (纯副作用)     |
//     +----------+   +-----------------+   +-------------+   +----------+   +--------+   +-----------------+
//     |
//     +--> Deny → 把拒绝原因回填给 LLM（不调 Execute）
//     +--> Allow → 继续 registry 查表 + Execute
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

// maxParallelTools 是单批并行工具调用的最大 goroutine 数。
// 通过 channel 信号量控制并发度，防止过于激进的 I/O 打满系统资源。
const maxParallelTools = 8

// toolCallBatch 描述一组 tool_calls 的执行策略。
type toolCallBatch struct {
	start    int  // calls[start:end] 在原始数组中的起始索引
	end      int  // 结束索引（不含）
	parallel bool // 该批次是否可以并行执行
}

// executeBatch 分区执行一批 tool_calls：
//   - 连续出现的只读工具合并为并行批次（goroutine 并发）
//   - 写工具作为独立串行批次分隔并行区域
//
// 执行顺序与 LLM 返回的原始顺序语义等价：并发批次的内部顺序不重要，
// 但批次之间严格串行，因此写操作不会被后续只读操作重排。
func (a *Agent) executeBatch(ctx context.Context, calls []openai.ToolCall) {
	results := make([]string, len(calls))

	for _, batch := range partitionToolCalls(a.registry, calls) {
		if batch.parallel && batch.end-batch.start > 1 {
			runParallel(batch.start, batch.end, func(i int) {
				results[i] = a.invokeTool(ctx, calls[i])
			})
			continue
		}
		for i := batch.start; i < batch.end; i++ {
			results[i] = a.invokeTool(ctx, calls[i])
		}
	}

	// 所有工具执行完毕后，按原始顺序回填 tool message。
	// compact 工具需要特殊后处理（真实的压缩逻辑在 CompactNow 中）。
	for i, tc := range calls {
		result := results[i]
		if tc.Function.Name == "compact" {
			focus := compactFocusFromArgs(tc.Function.Arguments)
			if err := a.CompactNow(ctx, focus); err != nil {
				result = fmt.Sprintf("Error: 手动压缩失败: %v", err)
			} else {
				result = "手动压缩已完成。"
			}
		}
		if result == "" {
			result = " " // DeepSeek 等 API 要求 content 字段必须存在
		}
		a.messages = append(a.messages, openai.ChatCompletionMessage{
			Role:       openai.ChatMessageRoleTool,
			ToolCallID: tc.ID,
			Content:    result,
		})
	}
}

// partitionToolCalls 将 LLM 返回的 tool_calls 列表按可并行性切分为多个批次。
//
// 分区规则：
//   - 连续出现的只读（ReadOnly()==true）工具合并为一个并行批次
//   - 每个写工具（ReadOnly()==false）作为独立的串行批次
//   - 未知工具（registry 中未注册）视为写工具，安全回退到串行
//
// 示例：
//
//	[read A, read B, bash rm, read C]
//	→ batch1(并行): [read A, read B]
//	→ batch2(串行): [bash rm]
//	→ batch3(并行): [read C]
func partitionToolCalls(r *tools.Registry, calls []openai.ToolCall) []toolCallBatch {
	var batches []toolCallBatch
	for i := 0; i < len(calls); {
		if isParallelisable(r, calls[i].Function.Name) {
			start := i
			i++
			for i < len(calls) && isParallelisable(r, calls[i].Function.Name) {
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

// isParallelisable 判断工具是否可以参与并行批次。
// 仅当工具已在 registry 注册且 ReadOnly()==true 时才返回 true。
func isParallelisable(r *tools.Registry, name string) bool {
	t := r.Get(name)
	if t == nil {
		return false
	}
	return t.ReadOnly()
}

// runParallel 在 [start, end) 范围内并发执行 run(i)，最多 maxParallelTools 个 goroutine。
// 使用 channel 信号量控制并发度，sync.WaitGroup 等待全部完成后返回。
func runParallel(start, end int, run func(int)) {
	sem := make(chan struct{}, maxParallelTools)
	var wg sync.WaitGroup
	for i := start; i < end; i++ {
		i := i
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			run(i)
		}()
	}
	wg.Wait()
}
