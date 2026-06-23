package agent

import (
	"context"
	"testing"

	"github.com/wsx864321/coding-agent/internal/provider"
)

type stubToolHooks struct {
	userPromptSubmit func(ctx context.Context, content string) error
	preToolUse       func(ctx context.Context, name string, args map[string]any) (bool, string)
	postToolUse      func(ctx context.Context, name string, args map[string]any, result string)
	stop             func(ctx context.Context, messages []provider.Message) (string, bool)
}

func (s *stubToolHooks) UserPromptSubmit(ctx context.Context, content string) error {
	if s.userPromptSubmit != nil {
		return s.userPromptSubmit(ctx, content)
	}
	return nil
}

func (s *stubToolHooks) PreToolUse(ctx context.Context, name string, args map[string]any) (bool, string) {
	if s.preToolUse != nil {
		return s.preToolUse(ctx, name, args)
	}
	return false, ""
}

func (s *stubToolHooks) PostToolUse(ctx context.Context, name string, args map[string]any, result string) {
	if s.postToolUse != nil {
		s.postToolUse(ctx, name, args, result)
	}
}

func (s *stubToolHooks) Stop(ctx context.Context, messages []provider.Message) (string, bool) {
	if s.stop != nil {
		return s.stop(ctx, messages)
	}
	return "", false
}

func TestSubsetHooks_NewSubsetHooksNil(t *testing.T) {
	if got := NewSubsetHooks(nil); got != nil {
		t.Fatalf("NewSubsetHooks(nil) = %v, want nil", got)
	}
}

func TestSubsetHooks_ForwardsPreAndPostToolUse(t *testing.T) {
	ctx := context.Background()
	preCalled := false
	postCalled := false

	inner := &stubToolHooks{
		preToolUse: func(_ context.Context, name string, args map[string]any) (bool, string) {
			preCalled = true
			if name != "read_file" {
				t.Errorf("PreToolUse name = %q, want read_file", name)
			}
			if args["path"] != "/tmp" {
				t.Errorf("PreToolUse args[path] = %v, want /tmp", args["path"])
			}
			return true, "blocked"
		},
		postToolUse: func(_ context.Context, name string, args map[string]any, result string) {
			postCalled = true
			if name != "read_file" {
				t.Errorf("PostToolUse name = %q, want read_file", name)
			}
			if result != "ok" {
				t.Errorf("PostToolUse result = %q, want ok", result)
			}
		},
	}

	subset := NewSubsetHooks(inner)
	block, msg := subset.PreToolUse(ctx, "read_file", map[string]any{"path": "/tmp"})
	if !preCalled {
		t.Fatal("PreToolUse was not forwarded to inner hooks")
	}
	if !block || msg != "blocked" {
		t.Errorf("PreToolUse = (%v, %q), want (true, blocked)", block, msg)
	}

	subset.PostToolUse(ctx, "read_file", map[string]any{"path": "/tmp"}, "ok")
	if !postCalled {
		t.Fatal("PostToolUse was not forwarded to inner hooks")
	}
}

func TestSubsetHooks_IgnoresUserPromptSubmitAndStop(t *testing.T) {
	ctx := context.Background()
	userCalled := false
	stopCalled := false

	inner := &stubToolHooks{
		userPromptSubmit: func(context.Context, string) error {
			userCalled = true
			return nil
		},
		stop: func(context.Context, []provider.Message) (string, bool) {
			stopCalled = true
			return "force", true
		},
	}

	subset := NewSubsetHooks(inner)

	if err := subset.UserPromptSubmit(ctx, "hello"); err != nil {
		t.Errorf("UserPromptSubmit err = %v, want nil", err)
	}
	if userCalled {
		t.Fatal("UserPromptSubmit should not be forwarded to inner hooks")
	}

	force, ok := subset.Stop(ctx, []provider.Message{{Role: provider.RoleUser, Content: "hi"}})
	if force != "" || ok {
		t.Errorf("Stop = (%q, %v), want (\"\", false)", force, ok)
	}
	if stopCalled {
		t.Fatal("Stop should not be forwarded to inner hooks")
	}
}
