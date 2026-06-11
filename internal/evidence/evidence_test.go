package evidence

import (
	"context"
	"testing"
)

func TestNewLedger(t *testing.T) {
	l := NewLedger()
	if l == nil {
		t.Fatal("NewLedger returned nil")
	}
	if len(l.CurrentTodos()) != 0 {
		t.Error("new ledger should have empty todos")
	}
}

func TestContextRoundTrip(t *testing.T) {
	l := NewLedger()
	ctx := WithLedger(context.Background(), l)
	got, ok := FromContext(ctx)
	if !ok || got != l {
		t.Error("FromContext should return the injected ledger")
	}

	_, ok = FromContext(context.Background())
	if ok {
		t.Error("FromContext on plain context should return false")
	}
}

func TestRecord(t *testing.T) {
	l := NewLedger()
	l.Record("bash", map[string]any{"command": "ls"}, "file.go", true)
	l.Record("bash", map[string]any{"command": "rm"}, "", false)

	if !l.HasSuccessfulReceipt("bash") {
		t.Error("should have successful bash receipt")
	}
	if l.HasSuccessfulReceipt("write_file") {
		t.Error("should not have write_file receipt")
	}
}

func TestHasAnyWriteReceipt(t *testing.T) {
	l := NewLedger()
	if l.HasAnyWriteReceipt() {
		t.Error("empty ledger should not have write receipts")
	}

	l.Record("write_file", nil, "ok", true)
	if !l.HasAnyWriteReceipt() {
		t.Error("should have write receipt after write_file")
	}

	l2 := NewLedger()
	l2.Record("edit_file", nil, "ok", true)
	if !l2.HasAnyWriteReceipt() {
		t.Error("should have write receipt after edit_file")
	}

	l3 := NewLedger()
	l3.Record("write_file", nil, "", false)
	if l3.HasAnyWriteReceipt() {
		t.Error("failed write should not count")
	}
}

func TestSetTodosAndCurrentTodos(t *testing.T) {
	l := NewLedger()
	todos := []TodoItem{
		{Content: "step 1", Status: "in_progress"},
		{Content: "step 2", Status: "pending"},
	}
	l.SetTodos(todos)

	got := l.CurrentTodos()
	if len(got) != 2 {
		t.Fatalf("CurrentTodos len = %d, want 2", len(got))
	}
	if got[0].Content != "step 1" || got[0].Status != "in_progress" {
		t.Errorf("todo[0] = %+v", got[0])
	}

	// 修改返回值不影响内部状态
	got[0].Content = "tampered"
	if l.CurrentTodos()[0].Content == "tampered" {
		t.Error("CurrentTodos should return a copy")
	}
}

func TestSetTodos_PreviousTodos(t *testing.T) {
	l := NewLedger()
	first := []TodoItem{{Content: "a", Status: "pending"}}
	second := []TodoItem{{Content: "a", Status: "completed"}}

	l.SetTodos(first)
	l.SetTodos(second)

	prev := l.PreviousTodos()
	if len(prev) != 1 || prev[0].Status != "pending" {
		t.Errorf("PreviousTodos should be first snapshot, got %+v", prev)
	}
	curr := l.CurrentTodos()
	if len(curr) != 1 || curr[0].Status != "completed" {
		t.Errorf("CurrentTodos should be second snapshot, got %+v", curr)
	}
}

func (l *Ledger) PreviousTodos() []TodoItem {
	l.mu.Lock()
	defer l.mu.Unlock()
	cp := make([]TodoItem, len(l.previousTodos))
	copy(cp, l.previousTodos)
	return cp
}

func TestMarkStepCompleted(t *testing.T) {
	l := NewLedger()
	if l.IsStepCompleted("step 1") {
		t.Error("should not be completed before marking")
	}
	l.MarkStepCompleted("step 1")
	if !l.IsStepCompleted("step 1") {
		t.Error("should be completed after marking")
	}
}

func TestUnverifiedCompletedTodos_NoBaseline(t *testing.T) {
	l := NewLedger()
	todos := []TodoItem{{Content: "a", Status: "completed"}}
	missing, hasBaseline := l.UnverifiedCompletedTodos(todos)
	if hasBaseline {
		t.Error("should have no baseline on first call")
	}
	if missing != nil {
		t.Error("missing should be nil without baseline")
	}
}

func TestUnverifiedCompletedTodos_WithBaseline(t *testing.T) {
	l := NewLedger()
	// 设置基线
	l.SetTodos([]TodoItem{
		{Content: "step 1", Status: "in_progress"},
		{Content: "step 2", Status: "pending"},
	})

	// 尝试标记 step 1 为 completed，但没有 complete_step 凭证
	newTodos := []TodoItem{
		{Content: "step 1", Status: "completed"},
		{Content: "step 2", Status: "in_progress"},
	}
	missing, hasBaseline := l.UnverifiedCompletedTodos(newTodos)
	if !hasBaseline {
		t.Error("should have baseline")
	}
	if len(missing) != 1 || missing[0] != "step 1" {
		t.Errorf("missing = %v, want [step 1]", missing)
	}

	// 签收 step 1 后再检查
	l.MarkStepCompleted("step 1")
	missing2, _ := l.UnverifiedCompletedTodos(newTodos)
	if len(missing2) != 0 {
		t.Errorf("after marking, missing = %v, want []", missing2)
	}
}

func TestUnverifiedCompletedTodos_AlreadyCompleted(t *testing.T) {
	l := NewLedger()
	// 基线中 step 1 已经 completed
	l.SetTodos([]TodoItem{
		{Content: "step 1", Status: "completed"},
		{Content: "step 2", Status: "pending"},
	})

	// step 1 在新列表中仍然 completed → 不是"新标记完成"，不需要凭证
	newTodos := []TodoItem{
		{Content: "step 1", Status: "completed"},
		{Content: "step 2", Status: "completed"},
	}
	missing, _ := l.UnverifiedCompletedTodos(newTodos)
	// 只有 step 2 是新标记的
	if len(missing) != 1 || missing[0] != "step 2" {
		t.Errorf("missing = %v, want [step 2]", missing)
	}
}

func TestReset(t *testing.T) {
	l := NewLedger()
	l.Record("bash", nil, "ok", true)
	l.MarkStepCompleted("step 1")
	l.IncrementGuardBlock()
	l.SetTodos([]TodoItem{{Content: "a", Status: "pending"}})

	l.Reset()

	if l.HasSuccessfulReceipt("bash") {
		t.Error("receipts should be cleared after Reset")
	}
	if l.IsStepCompleted("step 1") {
		t.Error("completedSteps should be cleared after Reset")
	}
	if l.GuardBlocks() != 0 {
		t.Error("guardBlocks should be cleared after Reset")
	}
	// currentTodos should survive Reset
	if len(l.CurrentTodos()) != 1 {
		t.Error("currentTodos should survive Reset")
	}
}

func TestGuardBlocks(t *testing.T) {
	l := NewLedger()
	if l.GuardBlocks() != 0 {
		t.Error("initial guardBlocks should be 0")
	}
	n := l.IncrementGuardBlock()
	if n != 1 {
		t.Errorf("first increment = %d, want 1", n)
	}
	n = l.IncrementGuardBlock()
	if n != 2 {
		t.Errorf("second increment = %d, want 2", n)
	}
}

func TestIsStepCompleted(t *testing.T) {
	l := NewLedger()
	l.MarkStepCompleted("done task")
	if !l.IsStepCompleted("done task") {
		t.Error("marked step should be completed")
	}
	if l.IsStepCompleted("other task") {
		t.Error("unmarked step should not be completed")
	}
}
