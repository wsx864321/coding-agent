package hooks

import (
	"context"
	"errors"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

// TestRegistry_UserPromptSubmit_NotifyOnly 验证 UserPromptSubmit 是通知型
//
// 期望：所有 hook 都被执行，error 不阻断
func TestRegistry_UserPromptSubmit_NotifyOnly(t *testing.T) {
	r := NewRegistry()
	var got []string
	r.RegisterUserPromptSubmit(func(_ context.Context, c string) error {
		got = append(got, "a:"+c)
		return nil
	})
	r.RegisterUserPromptSubmit(func(_ context.Context, c string) error {
		got = append(got, "b:"+c)
		return errors.New("ignored")
	})

	r.TriggerUserPromptSubmit(context.Background(), "hi")

	if len(got) != 2 || got[0] != "a:hi" || got[1] != "b:hi" {
		t.Errorf("got=%v, want [a:hi b:hi]", got)
	}
}

// TestRegistry_PreToolUse_ShortCircuit 验证 PreToolUse 首个非空 block 短路
func TestRegistry_PreToolUse_ShortCircuit(t *testing.T) {
	r := NewRegistry()
	var calls []string
	r.RegisterPreToolUse(func(_ context.Context, _ string, _ map[string]any) (string, string) {
		calls = append(calls, "first")
		return "", "" // 放行
	})
	r.RegisterPreToolUse(func(_ context.Context, _ string, _ map[string]any) (string, string) {
		calls = append(calls, "second")
		return "blocked", "reason2" // 阻断
	})
	r.RegisterPreToolUse(func(_ context.Context, _ string, _ map[string]any) (string, string) {
		calls = append(calls, "third")
		return "should-not-reach", ""
	})

	blocked, reason := r.TriggerPreToolUse(context.Background(), "x", nil)
	if !blocked {
		t.Error("expected blocked=true")
	}
	if reason != "blocked" {
		t.Errorf("reason=%q, want 'blocked'", reason)
	}
	if len(calls) != 2 {
		t.Errorf("expected 2 hooks to run, got %d (%v)", len(calls), calls)
	}
}

// TestRegistry_PreToolUse_AllAllow 验证全部放行
func TestRegistry_PreToolUse_AllAllow(t *testing.T) {
	r := NewRegistry()
	r.RegisterPreToolUse(func(_ context.Context, _ string, _ map[string]any) (string, string) {
		return "", ""
	})
	r.RegisterPreToolUse(func(_ context.Context, _ string, _ map[string]any) (string, string) {
		return "", ""
	})

	blocked, _ := r.TriggerPreToolUse(context.Background(), "x", nil)
	if blocked {
		t.Error("expected blocked=false when all hooks return empty")
	}
}

// TestRegistry_PostToolUse_NoReturn 验证 PostToolUse 全部执行
func TestRegistry_PostToolUse_NoReturn(t *testing.T) {
	r := NewRegistry()
	var got []string
	for _, name := range []string{"a", "b", "c"} {
		n := name
		r.RegisterPostToolUse(func(_ context.Context, _ string, _ map[string]any, _ string) {
			got = append(got, n)
		})
	}
	r.TriggerPostToolUse(context.Background(), "x", nil, "out")
	if len(got) != 3 {
		t.Errorf("got=%v, want 3 entries", got)
	}
}

// TestRegistry_Stop_ShortCircuit 验证 Stop 首个非空 force 短路
func TestRegistry_Stop_ShortCircuit(t *testing.T) {
	r := NewRegistry()
	var calls []string
	r.RegisterStop(func(_ context.Context, _ []openai.ChatCompletionMessage) (string, bool) {
		calls = append(calls, "first")
		return "", false
	})
	r.RegisterStop(func(_ context.Context, _ []openai.ChatCompletionMessage) (string, bool) {
		calls = append(calls, "second")
		return "continue please", true
	})
	r.RegisterStop(func(_ context.Context, _ []openai.ChatCompletionMessage) (string, bool) {
		calls = append(calls, "third")
		return "should-not-reach", true
	})

	force, ok := r.TriggerStop(context.Background(), nil)
	if !ok {
		t.Error("expected ok=true")
	}
	if force != "continue please" {
		t.Errorf("force=%q, want 'continue please'", force)
	}
	if len(calls) != 2 {
		t.Errorf("expected 2 hooks, got %d", len(calls))
	}
}

// TestRegistry_Count 验证 Count 反映注册情况
func TestRegistry_Count(t *testing.T) {
	r := NewRegistry()
	r.RegisterUserPromptSubmit(func(_ context.Context, _ string) error { return nil })
	r.RegisterPreToolUse(func(_ context.Context, _ string, _ map[string]any) (string, string) { return "", "" })
	r.RegisterPreToolUse(func(_ context.Context, _ string, _ map[string]any) (string, string) { return "", "" })
	r.RegisterPostToolUse(func(_ context.Context, _ string, _ map[string]any, _ string) {})

	c := r.Count()
	if c[EventUserPromptSubmit] != 1 {
		t.Errorf("UserPromptSubmit count = %d, want 1", c[EventUserPromptSubmit])
	}
	if c[EventPreToolUse] != 2 {
		t.Errorf("PreToolUse count = %d, want 2", c[EventPreToolUse])
	}
	if c[EventPostToolUse] != 1 {
		t.Errorf("PostToolUse count = %d, want 1", c[EventPostToolUse])
	}
	if c[EventStop] != 0 {
		t.Errorf("Stop count = %d, want 0", c[EventStop])
	}
}

// TestRegistry_NilHook 验证注册 nil hook 不会 panic
func TestRegistry_NilHook(t *testing.T) {
	r := NewRegistry()
	r.RegisterUserPromptSubmit(nil)
	r.RegisterPreToolUse(nil)
	r.RegisterPostToolUse(nil)
	r.RegisterStop(nil)
	if c := r.Count(); c[EventUserPromptSubmit] != 0 || c[EventPreToolUse] != 0 {
		t.Errorf("nil hooks should not register, got %+v", c)
	}
}
