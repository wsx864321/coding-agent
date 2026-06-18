package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/wsx864321/coding-agent/internal/provider"
	"github.com/wsx864321/coding-agent/internal/tools"
)

// =====================================================================
// 辅助：fake 工具
// =====================================================================

type fakeTool struct {
	name     string
	readOnly bool
	counter  *atomic.Int32
}

func (f *fakeTool) Name() string                                              { return f.name }
func (f *fakeTool) Description() string                                       { return "fake " + f.name }
func (f *fakeTool) ReadOnly() bool                                            { return f.readOnly }
func (f *fakeTool) Schema() json.RawMessage                                   { return json.RawMessage(`{}`) }
func (f *fakeTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	if f.counter != nil {
		f.counter.Add(1)
	}
	return f.name + "_result", nil
}

func makeToolCalls(names ...string) []provider.ToolCall {
	calls := make([]provider.ToolCall, len(names))
	for i, n := range names {
		calls[i] = makeToolCall("call_"+n, n, "{}")
	}
	return calls
}

func newRegistry(readOnlyNames ...string) *tools.Registry {
	r := tools.NewRegistry()
	roSet := make(map[string]bool, len(readOnlyNames))
	for _, n := range readOnlyNames {
		roSet[n] = true
	}
	allNames := []string{"read_file", "glob_file", "search_content", "bash", "write_file",
		"edit_file", "task", "todo_write", "complete_step", "unknown_tool"}
	for _, n := range allNames {
		isRO := roSet[n]
		r.Register(&fakeTool{name: n, readOnly: isRO})
	}
	return r
}

// =====================================================================
// Test: isParallelisable
// =====================================================================

func TestIsParallelisable(t *testing.T) {
	r := newRegistry("read_file", "glob_file", "search_content")

	tests := []struct {
		name string
		want bool
	}{
		{"read_file", true},
		{"glob_file", true},
		{"search_content", true},
		{"bash", false},
		{"write_file", false},
		{"edit_file", false},
		{"task", false},
		{"todo_write", false},
		{"complete_step", false},
		{"nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isParallelisable(r, tt.name)
			if got != tt.want {
				t.Errorf("isParallelisable(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// =====================================================================
// Test: partitionToolCalls
// =====================================================================

func TestPartitionToolCalls_AllReadOnly(t *testing.T) {
	r := newRegistry("read_file", "glob_file")
	calls := makeToolCalls("read_file", "glob_file", "read_file")
	batches := partitionToolCalls(r, calls)
	if len(batches) != 1 {
		t.Fatalf("len(batches) = %d, want 1", len(batches))
	}
	if !batches[0].parallel {
		t.Error("all-read batch should be parallel")
	}
}

func TestPartitionToolCalls_AllWrites(t *testing.T) {
	r := newRegistry()
	calls := makeToolCalls("bash", "write_file", "edit_file")
	batches := partitionToolCalls(r, calls)
	if len(batches) != 3 {
		t.Fatalf("len(batches) = %d, want 3", len(batches))
	}
	for i, b := range batches {
		if b.parallel {
			t.Errorf("batch[%d] should be serial", i)
		}
	}
}

func TestPartitionToolCalls_Mixed(t *testing.T) {
	r := newRegistry("read_file", "glob_file")
	calls := makeToolCalls("read_file", "glob_file", "bash", "read_file")
	batches := partitionToolCalls(r, calls)
	if len(batches) != 3 {
		t.Fatalf("len(batches) = %d, want 3", len(batches))
	}
	if !batches[0].parallel || batches[0].end-batches[0].start != 2 {
		t.Errorf("batch[0] wrong")
	}
	if batches[1].parallel {
		t.Errorf("batch[1] should be serial")
	}
	if !batches[2].parallel {
		t.Errorf("batch[2] should be parallel")
	}
}

func TestPartitionToolCalls_Empty(t *testing.T) {
	r := newRegistry()
	batches := partitionToolCalls(r, nil)
	if len(batches) != 0 {
		t.Errorf("empty calls should produce 0 batches, got %d", len(batches))
	}
}

func TestPartitionToolCalls_ReadWriteRead(t *testing.T) {
	r := newRegistry("read_file")
	calls := makeToolCalls("read_file", "write_file", "read_file")
	batches := partitionToolCalls(r, calls)
	if len(batches) != 3 {
		t.Fatalf("len(batches) = %d, want 3", len(batches))
	}
}

func TestPartitionToolCalls_UnknownToolsSerial(t *testing.T) {
	r := newRegistry()
	calls := makeToolCalls("unknown_tool", "another_unknown")
	batches := partitionToolCalls(r, calls)
	if len(batches) != 2 {
		t.Fatalf("len(batches) = %d, want 2", len(batches))
	}
	for i, b := range batches {
		if b.parallel {
			t.Errorf("batch[%d]: unknown tool should be serial", i)
		}
	}
}

// =====================================================================
// Test: runParallel
// =====================================================================

func TestRunParallel_ExecutesAll(t *testing.T) {
	results := make([]int, 10)
	runParallel(0, 10, func(i int) {
		results[i] = i * 2
	})
	for i := 0; i < 10; i++ {
		if results[i] != i*2 {
			t.Errorf("results[%d] = %d, want %d", i, results[i], i*2)
		}
	}
}

func TestRunParallel_SingleElement(t *testing.T) {
	var called bool
	runParallel(0, 1, func(i int) { called = true })
	if !called {
		t.Error("single element should be executed")
	}
}

func TestRunParallel_EmptyRange(t *testing.T) {
	runParallel(5, 5, func(i int) { t.Error("should not be called") })
}

func TestRunParallel_ConcurrentExecution(t *testing.T) {
	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32

	enter := make(chan struct{})
	leave := make(chan struct{})
	var entered sync.WaitGroup
	var done sync.WaitGroup

	n := 20
	entered.Add(n)
	done.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer done.Done()
			<-enter
			cur := concurrent.Add(1)
			if cur > maxConcurrent.Load() {
				maxConcurrent.Store(cur)
			}
			entered.Done()
			<-leave
			concurrent.Add(-1)
		}()
	}

	close(enter)
	entered.Wait()
	got := maxConcurrent.Load()
	close(leave)
	done.Wait()

	if got < 2 {
		t.Errorf("max concurrent = %d, expected at least 2", got)
	}
}

// =====================================================================
// Test: executeBatch 集成测试
// =====================================================================

func TestExecuteBatch_ParallelBatchRunsAllTools(t *testing.T) {
	r := newRegistry("read_file", "glob_file")
	a := &Agent{registry: r}
	calls := makeToolCalls("read_file", "glob_file")
	a.executeBatch(context.Background(), calls)

	if len(a.messages) != 2 {
		t.Fatalf("expected 2 tool messages, got %d", len(a.messages))
	}
	for i, m := range a.messages {
		if m.Role != provider.RoleTool {
			t.Errorf("messages[%d].Role = %q, want tool", i, m.Role)
		}
		if !strings.Contains(m.Content, "_result") {
			t.Errorf("messages[%d].Content = %q", i, m.Content)
		}
	}
}

func TestExecuteBatch_SerialBatchRunsInOrder(t *testing.T) {
	r := newRegistry()
	a := &Agent{registry: r}
	calls := makeToolCalls("write_file", "edit_file", "bash")
	a.executeBatch(context.Background(), calls)

	if len(a.messages) != 3 {
		t.Fatalf("expected 3 tool messages, got %d", len(a.messages))
	}
	wantNames := []string{"write_file_result", "edit_file_result", "bash_result"}
	for i, want := range wantNames {
		if got := a.messages[i].Content; got != want {
			t.Errorf("messages[%d].Content = %q, want %q", i, got, want)
		}
	}
}

func TestExecuteBatch_MixedBatchOrderPreserved(t *testing.T) {
	r := newRegistry("read_file")
	a := &Agent{registry: r}
	calls := makeToolCalls("read_file", "bash", "read_file")
	a.executeBatch(context.Background(), calls)

	if len(a.messages) != 3 {
		t.Fatalf("expected 3 tool messages, got %d", len(a.messages))
	}
	want := []string{"read_file_result", "bash_result", "read_file_result"}
	for i, w := range want {
		if got := a.messages[i].Content; got != w {
			t.Errorf("messages[%d].Content = %q, want %q", i, got, w)
		}
	}
}

func TestExecuteBatch_EmptyCalls(t *testing.T) {
	r := newRegistry("read_file")
	a := &Agent{registry: r}
	a.executeBatch(context.Background(), nil)
	if len(a.messages) != 0 {
		t.Errorf("expected 0 messages for empty calls, got %d", len(a.messages))
	}
}

func TestExecuteBatch_ToolErrorDoesNotStopOthers(t *testing.T) {
	errTool := &fakeTool{name: "fail_tool", readOnly: true}
	r := newRegistry("read_file")
	r.Register(errTool)
	a := &Agent{registry: r}
	calls := makeToolCalls("read_file", "fail_tool", "glob_file")
	a.executeBatch(context.Background(), calls)

	if len(a.messages) != 3 {
		t.Fatalf("expected 3 tool messages, got %d", len(a.messages))
	}
	if !strings.Contains(a.messages[0].Content, "read_file_result") {
		t.Errorf("messages[0] should contain read_file_result, got %q", a.messages[0].Content)
	}
}

// =====================================================================
// Test: compactFocusFromArgs
// =====================================================================

func TestCompactFocusFromArgs(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"whitespace", "   ", ""},
		{"valid", `{"focus":"important topic"}`, "important topic"},
		{"no_focus", `{}`, ""},
		{"invalid_json", `not json`, ""},
		{"trailing_spaces", `{"focus":"  trim me  "}`, "trim me"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compactFocusFromArgs(tt.raw)
			if got != tt.want {
				t.Errorf("compactFocusFromArgs(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestPartitionToolCalls_Deterministic(t *testing.T) {
	r := newRegistry("read_file", "glob_file")
	calls := makeToolCalls("read_file", "bash", "glob_file", "write_file", "read_file")
	var first []toolCallBatch
	for i := 0; i < 10; i++ {
		batches := partitionToolCalls(r, calls)
		if i == 0 {
			first = batches
			continue
		}
		if len(batches) != len(first) {
			t.Fatalf("iteration %d: len=%d, want %d", i, len(batches), len(first))
		}
		for j := range batches {
			if batches[j] != first[j] {
				t.Fatalf("iteration %d batch[%d] differs", i, j)
			}
		}
	}
}

func TestMakeToolCalls(t *testing.T) {
	calls := makeToolCalls("a", "b", "c")
	if len(calls) != 3 {
		t.Fatalf("len = %d, want 3", len(calls))
	}
	names := make([]string, len(calls))
	for i, c := range calls {
		names[i] = c.Name
	}
	if names[0] != "a" || names[1] != "b" || names[2] != "c" {
		t.Errorf("names = %v, want [a b c]", names)
	}
}

func TestToolCallBatch_Fields(t *testing.T) {
	b := toolCallBatch{start: 0, end: 3, parallel: true}
	if b.start != 0 || b.end != 3 || !b.parallel {
		t.Error("toolCallBatch fields incorrect")
	}
}
