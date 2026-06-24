package hooks

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRunner_PreToolUse_BlockChain(t *testing.T) {
	dir := t.TempDir()
	sp := mockSpawner(map[string]SpawnResult{
		"block-hook": {ExitCode: 2, Stderr: "denied"},
	})
	runner := NewRunner([]ResolvedHook{testHook(EventPreToolUse, HookConfig{Command: "block-hook", Match: "echo", Timeout: 5000})}, dir, sp, nil)

	blocked, msg := runner.PreToolUse(context.Background(), "echo", map[string]any{"input": "x"})
	if !blocked {
		t.Fatalf("expected block, msg=%q", msg)
	}
	if msg != "denied" {
		t.Fatalf("msg=%q, want denied", msg)
	}
}

func TestRunner_PreToolUse_Pass(t *testing.T) {
	dir := t.TempDir()
	sp := mockSpawner(map[string]SpawnResult{
		"pass-hook": {ExitCode: 0},
	})
	runner := NewRunner([]ResolvedHook{{
		Event:      EventPreToolUse,
		HookConfig: HookConfig{Command: "pass-hook"},
	}}, dir, sp, nil)

	blocked, msg := runner.PreToolUse(context.Background(), "bash", nil)
	if blocked {
		t.Fatalf("expected pass, blocked=%v msg=%q", blocked, msg)
	}
}

func TestRunner_EmptyHooks_NoOp(t *testing.T) {
	runner := NewRunner(nil, t.TempDir(), DefaultSpawner, nil)
	if err := runner.UserPromptSubmit(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	blocked, _ := runner.PreToolUse(context.Background(), "bash", nil)
	if blocked {
		t.Fatal("empty hooks should not block")
	}
	force, ok := runner.Stop(context.Background(), nil)
	if ok || force != "" {
		t.Fatalf("force=%q ok=%v", force, ok)
	}
}

func TestRunner_Stop_ForceSemantic(t *testing.T) {
	dir := t.TempDir()
	sp := mockSpawner(map[string]SpawnResult{
		"stop-hook": {ExitCode: 2, Stdout: "请继续完成待办"},
	})
	runner := NewRunner([]ResolvedHook{{
		Event:      EventStop,
		HookConfig: HookConfig{Command: "stop-hook"},
	}}, dir, sp, nil)

	force, ok := runner.Stop(context.Background(), nil)
	if !ok || force != "请继续完成待办" {
		t.Fatalf("force=%q ok=%v", force, ok)
	}
}

func TestRunner_Stop_NoForce(t *testing.T) {
	dir := t.TempDir()
	sp := mockSpawner(map[string]SpawnResult{
		"stop-hook": {ExitCode: 0},
	})
	runner := NewRunner([]ResolvedHook{{
		Event:      EventStop,
		HookConfig: HookConfig{Command: "stop-hook"},
	}}, dir, sp, nil)

	force, ok := runner.Stop(context.Background(), nil)
	if ok || force != "" {
		t.Fatalf("force=%q ok=%v", force, ok)
	}
}

func TestRunner_PostToolUse_CallsRun(t *testing.T) {
	dir := t.TempDir()
	var gotPayload Payload
	sp := Spawner(func(_ context.Context, in SpawnInput) SpawnResult {
		_ = json.Unmarshal([]byte(in.Stdin), &gotPayload)
		return SpawnResult{ExitCode: 0}
	})
	runner := NewRunner([]ResolvedHook{testHook(EventPostToolUse, HookConfig{Command: "post-hook", Match: "bash"})}, dir, sp, nil)

	runner.PostToolUse(context.Background(), "bash", map[string]any{"cmd": "ls"}, "ok")

	if gotPayload.Event != EventPostToolUse {
		t.Fatalf("event=%q", gotPayload.Event)
	}
	if gotPayload.Cwd != dir {
		t.Fatalf("cwd=%q", gotPayload.Cwd)
	}
	if gotPayload.ToolName != "bash" {
		t.Fatalf("toolName=%q", gotPayload.ToolName)
	}
	if gotPayload.ToolResult != "ok" {
		t.Fatalf("toolResult=%q", gotPayload.ToolResult)
	}
}

func TestRunner_UserPromptSubmit_NonBlocking(t *testing.T) {
	dir := t.TempDir()
	sp := mockSpawner(map[string]SpawnResult{
		"prompt-hook": {ExitCode: 2, Stdout: "bad prompt"},
	})
	runner := NewRunner([]ResolvedHook{{
		Event:      EventUserPromptSubmit,
		HookConfig: HookConfig{Command: "prompt-hook"},
	}}, dir, sp, nil)

	err := runner.UserPromptSubmit(context.Background(), "hello")
	if err != nil {
		t.Fatalf("UserPromptSubmit exit 2 should not block: %v", err)
	}
}

func TestRunner_Count(t *testing.T) {
	runner := NewRunner([]ResolvedHook{
		{Event: EventPreToolUse, HookConfig: HookConfig{Command: "a"}},
		{Event: EventPreToolUse, HookConfig: HookConfig{Command: "b"}},
		{Event: EventStop, HookConfig: HookConfig{Command: "c"}},
	}, t.TempDir(), DefaultSpawner, nil)

	m := runner.Count()
	if m[EventPreToolUse] != 2 || m[EventStop] != 1 {
		t.Fatalf("count=%v", m)
	}
}

func TestRunner_NewRunner_NilSpawner(t *testing.T) {
	runner := NewRunner(nil, t.TempDir(), nil, nil)
	if runner == nil {
		t.Fatal("expected non-nil runner")
	}
	force, ok := runner.Stop(context.Background(), nil)
	if ok || force != "" {
		t.Fatalf("force=%q ok=%v", force, ok)
	}
}
