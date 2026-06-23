// Package hooks 实现 coding-agent 的事件回调机制。
//
// 设计目标：
//   - 把 "扩展点" 从 agent 主循环里拆出来，循环只负责触发，不再写业务逻辑
//   - 与 permission 包解耦：hooks 是上层编排，permission 是系统级硬约束
//   - 任意模块都可以往 Registry 注册回调，回调按注册顺序链式调用
//   - 4 个事件：UserPromptSubmit / PreToolUse / PostToolUse / Stop
//
// 安全不变式（与 Claude Code 同型）：
// PreToolUse 流程中，hook 不阻断时，仍要走 system-level permission.Checker。
// 即使用户 hook "放行"，settings.json 的 deny/ask 规则仍会拦截。
//
// 事件语义：
//
//	UserPromptSubmit  追加 user 消息前；仅做日志 / 注入，error 不阻断
//	PreToolUse        工具执行前；首个返回非空 block 的 hook 阻断本次调用
//	PostToolUse       工具执行后；纯副作用（截断 / 统计），无返回语义
//	Stop              主循环即将结束；首个返回非空 force 的 hook 强制续跑
package hooks

import (
	"context"
	"sync"

	"github.com/wsx864321/coding-agent/internal/provider"
)

// UserPromptSubmitHook 在追加 user 消息前被调用
type UserPromptSubmitHook func(ctx context.Context, content string) error

// PreToolUseHook 在工具执行前被调用
type PreToolUseHook func(ctx context.Context, name string, args map[string]any) (block string, reason string)

// PostToolUseHook 在工具执行后被调用
type PostToolUseHook func(ctx context.Context, name string, args map[string]any, output string)

// StopHook 在主循环即将结束时被调用
type StopHook func(ctx context.Context, messages []provider.Message) (force string, ok bool)

// Registry 持有 4 类事件的回调链表
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
func (r *Registry) TriggerUserPromptSubmit(ctx context.Context, content string) {
	r.mu.RLock()
	hooks := append([]UserPromptSubmitHook(nil), r.userPromptSubmit...)
	r.mu.RUnlock()

	for _, h := range hooks {
		if err := h(ctx, content); err != nil {
			_ = err
		}
	}
}

// TriggerPreToolUse 触发所有 PreToolUse hook
func (r *Registry) TriggerPreToolUse(ctx context.Context, name string, args map[string]any) (blocked bool, reason string) {
	r.mu.RLock()
	hooks := append([]PreToolUseHook(nil), r.preToolUse...)
	r.mu.RUnlock()

	for _, h := range hooks {
		block, why := h(ctx, name, args)
		if block != "" {
			return true, block
		}
		_ = why
	}
	return false, ""
}

// TriggerPostToolUse 触发所有 PostToolUse hook
func (r *Registry) TriggerPostToolUse(ctx context.Context, name string, args map[string]any, output string) {
	r.mu.RLock()
	hooks := append([]PostToolUseHook(nil), r.postToolUse...)
	r.mu.RUnlock()

	for _, h := range hooks {
		h(ctx, name, args, output)
	}
}

// TriggerStop 触发所有 Stop hook
func (r *Registry) TriggerStop(ctx context.Context, messages []provider.Message) (force string, ok bool) {
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

// WithoutStopAndPrompt 返回只保留 PreToolUse / PostToolUse 的新 Registry。
func (r *Registry) WithoutStopAndPrompt() *Registry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	child := &Registry{
		preToolUse:  append([]PreToolUseHook(nil), r.preToolUse...),
		postToolUse: append([]PostToolUseHook(nil), r.postToolUse...),
	}
	return child
}

// Count 返回每个事件注册的 hook 数量
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
