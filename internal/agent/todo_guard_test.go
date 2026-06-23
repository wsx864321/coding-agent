package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/wsx864321/coding-agent/internal/evidence"
	"github.com/wsx864321/coding-agent/internal/provider"
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

func TestCheckTodoGuard_MultipleIncomplete(t *testing.T) {
	a := &Agent{}
	l := evidence.NewLedger()
	l.SetTodos([]evidence.TodoItem{
		{Content: "task A", Status: "completed"},
		{Content: "task B", Status: "pending"},
		{Content: "task C", Status: "in_progress"},
	})
	ctx := evidence.WithLedger(context.Background(), l)

	got := a.checkTodoGuard(ctx)
	if got == "" {
		t.Fatal("checkTodoGuard() = empty, want force message")
	}
	if !strings.Contains(got, "2/3") {
		t.Errorf("force message = %q, want count 2/3", got)
	}
	if !strings.Contains(got, "task B [pending]") || !strings.Contains(got, "task C [in_progress]") {
		t.Errorf("force message = %q, want all incomplete items listed", got)
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

func TestLoopStep_TodoGuard_InjectsForceMessage(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "premature answer"})
	a := newTestAgent(t, f)
	a.messages = []provider.Message{{Role: provider.RoleUser, Content: "hi"}}
	a.ledger.SetTodos([]evidence.TodoItem{
		{Content: "finish refactor", Status: "pending"},
	})

	ctx := evidence.WithLedger(context.Background(), a.ledger)
	final, err := a.loopStep(ctx)
	if err != nil {
		t.Fatalf("loopStep: %v", err)
	}
	if final != "" {
		t.Fatalf("expected empty final (force continue), got %q", final)
	}
	if f.calls.Load() != 1 {
		t.Errorf("expected 1 LLM call, got %d", f.calls.Load())
	}

	last := a.messages[len(a.messages)-1]
	if last.Role != provider.RoleUser || !strings.Contains(last.Content, "宿主就绪检查失败") {
		t.Errorf("expected TodoGuard force message, got role=%q content=%q", last.Role, last.Content)
	}
	if !strings.Contains(last.Content, "finish refactor [pending]") {
		t.Errorf("force message missing incomplete todo: %q", last.Content)
	}
}

func TestLoopStep_TodoGuard_BeforeStopHook(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "done"})
	stopCalled := false
	hr := &stubToolHooks{
		stop: func(_ context.Context, _ []provider.Message) (string, bool) {
			stopCalled = true
			return "external force", true
		},
	}
	a := newTestAgentWithHooks(t, f, hr)
	a.messages = []provider.Message{{Role: provider.RoleUser, Content: "hi"}}
	a.ledger.SetTodos([]evidence.TodoItem{
		{Content: "pending task", Status: "in_progress"},
	})

	ctx := evidence.WithLedger(context.Background(), a.ledger)
	final, err := a.loopStep(ctx)
	if err != nil {
		t.Fatalf("loopStep: %v", err)
	}
	if final != "" {
		t.Fatalf("expected empty final, got %q", final)
	}
	if stopCalled {
		t.Fatal("Stop hook should not run when TodoGuard already forced continuation")
	}
	last := a.messages[len(a.messages)-1]
	if !strings.Contains(last.Content, "宿主就绪检查失败") {
		t.Errorf("expected TodoGuard message, got %q", last.Content)
	}
}

func TestRun_TodoGuard_AllCompleted_NoForce(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "done"})
	a := newTestAgent(t, f)
	a.ledger.SetTodos([]evidence.TodoItem{
		{Content: "task A", Status: "completed"},
	})

	out, err := a.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "done" {
		t.Errorf("output = %q, want 'done'", out)
	}
	if f.calls.Load() != 1 {
		t.Errorf("expected 1 LLM call, got %d", f.calls.Load())
	}
}
