package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
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
