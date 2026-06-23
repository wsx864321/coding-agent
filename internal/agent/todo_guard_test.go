package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/wsx864321/coding-agent/internal/evidence"
)

func TestCheckTodoGuard_NoLedger(t *testing.T) {
	a := &Agent{}
	if got := a.checkTodoGuard(context.Background()); got != "" {
		t.Errorf("checkTodoGuard() = %q, want empty", got)
	}
}

func TestCheckTodoGuard_NoTodos(t *testing.T) {
	a := &Agent{}
	l := evidence.NewLedger()
	ctx := evidence.WithLedger(context.Background(), l)
	if got := a.checkTodoGuard(ctx); got != "" {
		t.Errorf("checkTodoGuard() = %q, want empty", got)
	}
}

func TestCheckTodoGuard_AllCompleted(t *testing.T) {
	a := &Agent{}
	l := evidence.NewLedger()
	l.SetTodos([]evidence.TodoItem{
		{Content: "task A", Status: "completed"},
		{Content: "task B", Status: "completed"},
	})
	ctx := evidence.WithLedger(context.Background(), l)
	if got := a.checkTodoGuard(ctx); got != "" {
		t.Errorf("checkTodoGuard() = %q, want empty", got)
	}
}

func TestCheckTodoGuard_IncompleteTodos_ForcesContinue(t *testing.T) {
	a := &Agent{}
	l := evidence.NewLedger()
	l.SetTodos([]evidence.TodoItem{
		{Content: "task A", Status: "completed"},
		{Content: "task B", Status: "pending"},
	})
	ctx := evidence.WithLedger(context.Background(), l)

	got := a.checkTodoGuard(ctx)
	if got == "" {
		t.Fatal("checkTodoGuard() = empty, want force message")
	}
	if !strings.Contains(got, "宿主就绪检查失败") {
		t.Errorf("force message = %q, want contains '宿主就绪检查失败'", got)
	}
	if !strings.Contains(got, "task B [pending]") {
		t.Errorf("force message = %q, want contains incomplete todo", got)
	}
}

func TestCheckTodoGuard_ExceedsMaxBlocks_AllowsThrough(t *testing.T) {
	a := &Agent{}
	l := evidence.NewLedger()
	l.SetTodos([]evidence.TodoItem{
		{Content: "task A", Status: "in_progress"},
	})
	ctx := evidence.WithLedger(context.Background(), l)

	for i := 0; i < maxGuardBlocks; i++ {
		if got := a.checkTodoGuard(ctx); got == "" {
			t.Fatalf("block %d: checkTodoGuard() = empty, want force message", i+1)
		}
	}
	if got := a.checkTodoGuard(ctx); got != "" {
		t.Errorf("after %d blocks, checkTodoGuard() = %q, want empty (allow through)", maxGuardBlocks, got)
	}
}
