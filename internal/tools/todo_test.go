package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/wsx864321/coding-agent/internal/evidence"
)

func TestTodoWriteTool_Basic(t *testing.T) {
	tool := NewTodoWriteTool()

	if tool.Name() != "todo_write" {
		t.Errorf("Name() = %q", tool.Name())
	}

	// 无 ledger 的 context：基础功能仍可用
	out, err := tool.Execute(context.Background(), map[string]any{
		"todos": []any{
			map[string]any{"content": "step 1", "status": "in_progress"},
			map[string]any{"content": "step 2", "status": "pending"},
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "2 total") {
		t.Errorf("output should contain summary, got %q", out)
	}
	if !strings.Contains(out, "1 in progress") {
		t.Errorf("output should show in_progress count, got %q", out)
	}
}

func TestTodoWriteTool_EmptyTodos(t *testing.T) {
	tool := NewTodoWriteTool()
	_, err := tool.Execute(context.Background(), map[string]any{
		"todos": []any{},
	})
	if err == nil {
		t.Fatal("expected error for empty todos")
	}
}

func TestTodoWriteTool_InvalidStatus(t *testing.T) {
	tool := NewTodoWriteTool()
	_, err := tool.Execute(context.Background(), map[string]any{
		"todos": []any{
			map[string]any{"content": "step 1", "status": "invalid"},
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid status")
	}
	if !strings.Contains(err.Error(), "无效") {
		t.Errorf("error = %q, should mention invalid status", err.Error())
	}
}

func TestTodoWriteTool_EmptyContent(t *testing.T) {
	tool := NewTodoWriteTool()
	_, err := tool.Execute(context.Background(), map[string]any{
		"todos": []any{
			map[string]any{"content": "", "status": "pending"},
		},
	})
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestTodoWriteTool_CompletionWithoutEvidence(t *testing.T) {
	tool := NewTodoWriteTool()
	ledger := evidence.NewLedger()

	// 设置基线
	ctx := evidence.WithLedger(context.Background(), ledger)
	_, err := tool.Execute(ctx, map[string]any{
		"todos": []any{
			map[string]any{"content": "step 1", "status": "in_progress"},
			map[string]any{"content": "step 2", "status": "pending"},
		},
	})
	if err != nil {
		t.Fatalf("first todo_write: %v", err)
	}

	// 尝试标记 step 1 为 completed，但没有 complete_step 凭证
	_, err = tool.Execute(ctx, map[string]any{
		"todos": []any{
			map[string]any{"content": "step 1", "status": "completed"},
			map[string]any{"content": "step 2", "status": "in_progress"},
		},
	})
	if err == nil {
		t.Fatal("expected error for completion without evidence")
	}
	if !strings.Contains(err.Error(), "complete_step") {
		t.Errorf("error = %q, should mention complete_step", err.Error())
	}
}

func TestTodoWriteTool_CompletionWithEvidence(t *testing.T) {
	tool := NewTodoWriteTool()
	ledger := evidence.NewLedger()
	ctx := evidence.WithLedger(context.Background(), ledger)

	// 设置基线
	_, err := tool.Execute(ctx, map[string]any{
		"todos": []any{
			map[string]any{"content": "step 1", "status": "in_progress"},
			map[string]any{"content": "step 2", "status": "pending"},
		},
	})
	if err != nil {
		t.Fatalf("first todo_write: %v", err)
	}

	// 签收 step 1
	ledger.MarkStepCompleted("step 1")

	// 现在可以标记为 completed
	out, err := tool.Execute(ctx, map[string]any{
		"todos": []any{
			map[string]any{"content": "step 1", "status": "completed"},
			map[string]any{"content": "step 2", "status": "in_progress"},
		},
	})
	if err != nil {
		t.Fatalf("second todo_write: %v", err)
	}
	if !strings.Contains(out, "1 completed") {
		t.Errorf("output = %q, should show 1 completed", out)
	}
}

func TestTodoWriteTool_ActiveForm(t *testing.T) {
	tool := NewTodoWriteTool()
	out, err := tool.Execute(context.Background(), map[string]any{
		"todos": []any{
			map[string]any{
				"content":    "Add type hints",
				"status":     "in_progress",
				"activeForm": "Adding type hints to functions",
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Adding type hints to functions") {
		t.Errorf("output should contain activeForm, got %q", out)
	}
}

func TestTodoWriteTool_UpdatesLedger(t *testing.T) {
	tool := NewTodoWriteTool()
	ledger := evidence.NewLedger()
	ctx := evidence.WithLedger(context.Background(), ledger)

	_, err := tool.Execute(ctx, map[string]any{
		"todos": []any{
			map[string]any{"content": "step 1", "status": "pending"},
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	todos := ledger.CurrentTodos()
	if len(todos) != 1 || todos[0].Content != "step 1" {
		t.Errorf("ledger should be updated, got %+v", todos)
	}
}
