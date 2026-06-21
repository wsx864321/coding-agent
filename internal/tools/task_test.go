package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/wsx864321/coding-agent/internal/jobs"
)

func TestTaskTool_Name(t *testing.T) {
	tt := NewTaskTool(nil)
	if tt.Name() != "task" {
		t.Errorf("Name() = %q, want %q", tt.Name(), "task")
	}
}

func TestTaskTool_Schema(t *testing.T) {
	tt := NewTaskTool(nil)
	s := tt.Schema()
	if len(s) == 0 {
		t.Fatal("Schema() returned empty")
	}
	if !strings.Contains(string(s), "prompt") {
		t.Error("Schema should contain 'prompt' property")
	}
}

func TestTaskTool_Execute_Success(t *testing.T) {
	runner := func(ctx context.Context, prompt string) (string, error) {
		return "子 agent 回答: " + prompt, nil
	}
	tt := NewTaskTool(runner)

	out, err := tt.Execute(context.Background(), map[string]any{
		"prompt":      "请列出所有 Go 文件",
		"description": "列出Go文件",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "子 agent 回答") {
		t.Errorf("output = %q, want contains '子 agent 回答'", out)
	}
}

func TestTaskTool_Execute_EmptyPrompt(t *testing.T) {
	tt := NewTaskTool(func(ctx context.Context, p string) (string, error) {
		return "", nil
	})
	_, err := tt.Execute(context.Background(), map[string]any{
		"prompt": "  ",
	})
	if err == nil {
		t.Fatal("expected error for empty prompt")
	}
}

func TestTaskTool_Execute_NilRunner(t *testing.T) {
	tt := NewTaskTool(nil)
	_, err := tt.Execute(context.Background(), map[string]any{
		"prompt": "hello",
	})
	if err == nil {
		t.Fatal("expected error for nil runner")
	}
	if !strings.Contains(err.Error(), "未配置") {
		t.Errorf("error = %q, want contains '未配置'", err.Error())
	}
}

func TestTaskTool_Execute_RunnerError(t *testing.T) {
	runner := func(ctx context.Context, prompt string) (string, error) {
		return "", errors.New("LLM 超时")
	}
	tt := NewTaskTool(runner)
	_, err := tt.Execute(context.Background(), map[string]any{
		"prompt": "do something",
	})
	if err == nil {
		t.Fatal("expected error from runner")
	}
	if !strings.Contains(err.Error(), "LLM 超时") {
		t.Errorf("error = %q, want contains 'LLM 超时'", err.Error())
	}
}

func TestTaskTool_SetRunner(t *testing.T) {
	tt := NewTaskTool(nil)

	tt.SetRunner(func(ctx context.Context, prompt string) (string, error) {
		return "wired", nil
	})

	out, err := tt.Execute(context.Background(), map[string]any{"prompt": "test"})
	if err != nil {
		t.Fatalf("Execute after SetRunner: %v", err)
	}
	if out != "wired" {
		t.Errorf("output = %q, want %q", out, "wired")
	}
}

// =====================================================================
// run_in_background
// =====================================================================

func TestTaskTool_RunInBackground_Success(t *testing.T) {
	m := jobs.NewManager()
	defer m.Close()
	ctx := jobs.WithManager(context.Background(), m)

	tt := NewTaskTool(func(ctx context.Context, prompt string) (string, error) {
		return "bg answer: " + prompt, nil
	})

	out, err := tt.Execute(ctx, map[string]any{
		"prompt":             "do something",
		"description":        "bg task",
		"run_in_background":  true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "task-1") {
		t.Errorf("output %q should contain job id", out)
	}
	if !strings.Contains(out, "bash_output") {
		t.Errorf("output %q should mention bash_output", out)
	}
}

func TestTaskTool_RunInBackground_NoManager(t *testing.T) {
	tt := NewTaskTool(func(ctx context.Context, prompt string) (string, error) {
		return "x", nil
	})

	_, err := tt.Execute(context.Background(), map[string]any{
		"prompt":            "test",
		"run_in_background": true,
	})
	if err == nil {
		t.Fatal("expected error when no manager")
	}
}

func TestTaskTool_RunInBackground_Result(t *testing.T) {
	m := jobs.NewManager()
	defer m.Close()
	ctx := jobs.WithManager(context.Background(), m)

	tt := NewTaskTool(func(ctx context.Context, prompt string) (string, error) {
		time.Sleep(20 * time.Millisecond)
		return "final answer", nil
	})

	out, err := tt.Execute(ctx, map[string]any{
		"prompt":            "x",
		"run_in_background": true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	jobID := extractTaskJobID(t, out)
	results := m.Wait(context.Background(), []string{jobID}, 5)
	if len(results) != 1 {
		t.Fatalf("Wait returned %d, want 1", len(results))
	}
	if results[0].Status != jobs.Done {
		t.Errorf("status = %q, want done", results[0].Status)
	}
	if results[0].Output != "final answer" {
		t.Errorf("output = %q, want 'final answer'", results[0].Output)
	}
}

func TestTaskTool_RunInBackground_RunnerError(t *testing.T) {
	m := jobs.NewManager()
	defer m.Close()
	ctx := jobs.WithManager(context.Background(), m)

	tt := NewTaskTool(func(ctx context.Context, prompt string) (string, error) {
		return "", errors.New("subagent failed")
	})

	out, err := tt.Execute(ctx, map[string]any{
		"prompt":            "x",
		"run_in_background": true,
	})
	if err != nil {
		t.Fatalf("Execute should not fail immediately: %v", err)
	}

	jobID := extractTaskJobID(t, out)
	results := m.Wait(context.Background(), []string{jobID}, 5)
	if len(results) != 1 {
		t.Fatalf("Wait returned %d, want 1", len(results))
	}
	if results[0].Status != jobs.Failed {
		t.Errorf("status = %q, want failed", results[0].Status)
	}
}

func TestTaskTool_Schema_ContainsRunInBackground(t *testing.T) {
	tt := NewTaskTool(nil)
	s := string(tt.Schema())
	if !strings.Contains(s, "run_in_background") {
		t.Error("Schema should contain run_in_background")
	}
}

func extractTaskJobID(t *testing.T, out string) string {
	t.Helper()
	start := strings.Index(out, "\"")
	if start < 0 {
		t.Fatalf("cannot find job id in %q", out)
	}
	rest := out[start+1:]
	end := strings.Index(rest, "\"")
	if end < 0 {
		t.Fatalf("cannot find closing quote in %q", out)
	}
	return rest[:end]
}
