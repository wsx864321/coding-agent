// Package hooks 实现 coding-agent 的事件回调机制。
//
// 设计目标：
//   - 把 "扩展点" 从 agent 主循环里拆出来，循环只负责触发，不再写业务逻辑
//   - 与 permission 包解耦：hooks 是上层编排，permission 是系统级硬约束
//   - 任意模块都可以往 Registry 注册回调，回调按注册顺序链式调用
//   - 4 个事件：UserPromptSubmit / PreToolUse / PostToolUse / Stop
//
// 安全不变式（与 Claude Code 同型）：
//   PreToolUse 流程中，hook 不阻断时，仍要走 system-level permission.Checker。
//   即使用户 hook "放行"，settings.json 的 deny/ask 规则仍会拦截。
//   换句话说：hook 可以"放水"（允许更多操作），但不能"开闸"（绕过系统硬拒绝）。
//
// 事件语义：
//
//	UserPromptSubmit  追加 user 消息前；仅做日志 / 注入，error 不阻断
//	PreToolUse        工具执行前；首个返回非空 block 的 hook 阻断本次调用
//	PostToolUse       工具执行后；纯副作用（截断 / 统计），无返回语义
//	Stop              主循环即将结束；首个返回非空 force 的 hook 强制续跑
//	                  （把 force 作为 user 消息注入，下一轮 LLM 必须继续工作）
package hooks

import (
	"context"
	"sync"

	openai "github.com/sashabaranov/go-openai"

	"github.com/wsx864321/coding-agent/internal/permission"
)

// Event 标识 4 类事件
type Event string

const (
	EventUserPromptSubmit Event = "UserPromptSubmit"
	EventPreToolUse       Event = "PreToolUse"
	EventPostToolUse      Event = "PostToolUse"
	EventStop             Event = "Stop"
)

// UserPromptSubmitHook 在追加 user 消息前被调用
//
// 入参：用户输入的原始字符串
// 返回：error 仅用于日志，不会阻断主流程（UserPromptSubmit 是"通知型"事件）
type UserPromptSubmitHook func(ctx context.Context, content string) error

// PreToolUseHook 在工具执行前被调用
//
// 入参：工具名 + 参数
// 返回：
//   - block 非空 → 立即阻断本次工具调用，把 block 字符串作为 tool_result 回填给 LLM
//   - block 为空 → 继续执行后续 hook；全部 hook 放行后仍要走 system permission.Checker
//
// reason 是阻断时的原因，会被拼到回填消息里（便于 LLM 看到完整上下文）
type PreToolUseHook func(ctx context.Context, call permission.ToolCall) (block string, reason string)

// PostToolUseHook 在工具执行成功后被调用
//
// 入参：call + 执行输出
// 返回：无（纯副作用：日志 / 截断 / 统计等）
type PostToolUseHook func(ctx context.Context, call permission.ToolCall, output string)

// StopHook 在主循环即将结束（拿到 final answer）时被调用
//
// 入参：当前消息历史
// 返回：
//   - force 非空 → 强制续跑：把 force 作为 user 消息追加到历史，下一轮 LLM 必须继续
//   - force 为空 → 正常退出
//
// 典型用途：summary 摘要、缺失工具自检、最终检查等
type StopHook func(ctx context.Context, messages []openai.ChatCompletionMessage) (force string, ok bool)

// Registry 持有 4 类事件的回调链表
//
// 线程安全：所有 Register / Trigger 方法都受 mu 保护，可并发注册 + 并发触发
type Registry struct {
	mu sync.RWMutex

	userPromptSubmit []UserPromptSubmitHook
	preToolUse       []PreToolUseHook
	postToolUse      []PostToolUseHook
	stop             []StopHook
}

// NewRegistry 构造一个空的 Registry
func NewRegistry() *Registry {
	return &Registry{}
}

// RegisterUserPromptSubmit 注册一个 UserPromptSubmit hook
func (r *Registry) RegisterUserPromptSubmit(h UserPromptSubmitHook) {
	if h == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.userPromptSubmit = append(r.userPromptSubmit, h)
}

// RegisterPreToolUse 注册一个 PreToolUse hook
func (r *Registry) RegisterPreToolUse(h PreToolUseHook) {
	if h == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.preToolUse = append(r.preToolUse, h)
}

// RegisterPostToolUse 注册一个 PostToolUse hook
func (r *Registry) RegisterPostToolUse(h PostToolUseHook) {
	if h == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.postToolUse = append(r.postToolUse, h)
}

// RegisterStop 注册一个 Stop hook
func (r *Registry) RegisterStop(h StopHook) {
	if h == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stop = append(r.stop, h)
}

// TriggerUserPromptSubmit 触发所有 UserPromptSubmit hook
//
// 全部执行（不短路），单个 hook 返回 error 仅记日志，不阻断主流程
func (r *Registry) TriggerUserPromptSubmit(ctx context.Context, content string) {
	r.mu.RLock()
	hooks := append([]UserPromptSubmitHook(nil), r.userPromptSubmit...)
	r.mu.RUnlock()

	for _, h := range hooks {
		if err := h(ctx, content); err != nil {
			// UserPromptSubmit 是"通知型"事件，error 不阻断；调用方通常写日志
			_ = err
		}
	}
}

// TriggerPreToolUse 触发所有 PreToolUse hook
//
// 返回是否被阻断、阻断原因
// 首个返回非空 block 的 hook 短路；后续 hook 不再执行
func (r *Registry) TriggerPreToolUse(ctx context.Context, call permission.ToolCall) (blocked bool, reason string) {
	r.mu.RLock()
	hooks := append([]PreToolUseHook(nil), r.preToolUse...)
	r.mu.RUnlock()

	for _, h := range hooks {
		block, why := h(ctx, call)
		if block != "" {
			return true, block
		}
		_ = why // reason 目前仅作信息保留；block 非空即视为阻断
	}
	return false, ""
}

// TriggerPostToolUse 触发所有 PostToolUse hook
//
// 全部执行，无返回值；单个 panic 不影响后续（调用方通常用 defer recover）
func (r *Registry) TriggerPostToolUse(ctx context.Context, call permission.ToolCall, output string) {
	r.mu.RLock()
	hooks := append([]PostToolUseHook(nil), r.postToolUse...)
	r.mu.RUnlock()

	for _, h := range hooks {
		h(ctx, call, output)
	}
}

// TriggerStop 触发所有 Stop hook
//
// 返回是否续跑、续跑消息
// 首个返回非空 force 的 hook 短路；后续 hook 不再执行
func (r *Registry) TriggerStop(ctx context.Context, messages []openai.ChatCompletionMessage) (force string, ok bool) {
	r.mu.RLock()
	hooks := append([]StopHook(nil), r.stop...)
	r.mu.RUnlock()

	for _, h := range hooks {
		f, _ := h(ctx, messages)
		if f != "" {
			return f, true
		}
	}
	return "", false
}

// Count 返回每个事件注册的 hook 数量（用于调试 / /hooks 之类 CLI）
func (r *Registry) Count() map[Event]int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return map[Event]int{
		EventUserPromptSubmit: len(r.userPromptSubmit),
		EventPreToolUse:       len(r.preToolUse),
		EventPostToolUse:      len(r.postToolUse),
		EventStop:             len(r.stop),
	}
}
