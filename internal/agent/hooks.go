package agent

import (
	"context"

	"github.com/wsx864321/coding-agent/internal/provider"
)

// ToolHooks 是 Agent 与 hook 实现之间的解耦边界（D1）。
type ToolHooks interface {
	UserPromptSubmit(ctx context.Context, content string) error
	PreToolUse(ctx context.Context, name string, args map[string]any) (block bool, message string)
	PostToolUse(ctx context.Context, name string, args map[string]any, result string)
	Stop(ctx context.Context, messages []provider.Message) (force string, ok bool)
}

// SubsetHooks 仅转发 PreToolUse / PostToolUse，供 subagent 使用（D1）。
type SubsetHooks struct {
	inner ToolHooks
}

func (s *SubsetHooks) UserPromptSubmit(_ context.Context, _ string) error { return nil }

func (s *SubsetHooks) PreToolUse(ctx context.Context, name string, args map[string]any) (bool, string) {
	return s.inner.PreToolUse(ctx, name, args)
}

func (s *SubsetHooks) PostToolUse(ctx context.Context, name string, args map[string]any, result string) {
	s.inner.PostToolUse(ctx, name, args, result)
}

func (s *SubsetHooks) Stop(_ context.Context, _ []provider.Message) (string, bool) { return "", false }

// NewSubsetHooks 构造 subagent 专用 hook 视图。
func NewSubsetHooks(h ToolHooks) ToolHooks {
	if h == nil {
		return nil
	}
	return &SubsetHooks{inner: h}
}
