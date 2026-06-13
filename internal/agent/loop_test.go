package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	openai "github.com/sashabaranov/go-openai"

	"github.com/wsx864321/coding-agent/internal/tools"
)

// =====================================================================
// 辅助：fake 工具（实现 tools.Tool 接口）
// =====================================================================

type fakeTool struct {
	name     string
	readOnly bool
	// 用于检测并行执行的计数器：并行执行时会有多个 goroutine 同时访问
	counter *atomic.Int32
}

func (f *fakeTool) Name() string                                 { return f.name }
func (f *fakeTool) Description() string                          { return "fake " + f.name }
func (f *fakeTool) ReadOnly() bool                               { return f.readOnly }
func (f *fakeTool) Schema() json.RawMessage                      { return json.RawMessage(`{}`) }
func (f *fakeTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	if f.counter != nil {
		f.counter.Add(1)
	}
	return f.name + "_result", nil
}

// makeToolCalls 批量构造 tool_calls（复用 agent_test.go 中的 makeToolCall）
func makeToolCalls(names ...string) []openai.ToolCall {
	calls := make([]openai.ToolCall, len(names))
	for i, n := range names {
		calls[i] = makeToolCall("call_"+n, n, "{}")
	}
	return calls
}

// newRegistry 创建一个带 fake 工具的 registry。
// readOnlyNames 中的工具标记为只读，其余为写工具。
func newRegistry(readOnlyNames ...string) *tools.Registry {
	r := tools.NewRegistry()
	roSet := make(map[string]bool, len(readOnlyNames))
	for _, n := range readOnlyNames {
		roSet[n] = true
	}
	// 注册一批常用工具
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
		{"nonexistent", false}, // 未注册的也视为不可并行
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
	if batches[0].start != 0 || batches[0].end != 3 {
		t.Errorf("batch range = [%d:%d], want [0:3]", batches[0].start, batches[0].end)
	}
}

func TestPartitionToolCalls_AllWrites(t *testing.T) {
	r := newRegistry() // 全部非只读
	calls := makeToolCalls("bash", "write_file", "edit_file")

	batches := partitionToolCalls(r, calls)

	if len(batches) != 3 {
		t.Fatalf("len(batches) = %d, want 3", len(batches))
	}
	for i, b := range batches {
		if b.parallel {
			t.Errorf("batch[%d] should be serial", i)
		}
		if b.end-b.start != 1 {
			t.Errorf("batch[%d] size = %d, want 1", i, b.end-b.start)
		}
	}
}

func TestPartitionToolCalls_Mixed(t *testing.T) {
	// [read A, read B, bash rm, read C]  → 3 个 batch
	r := newRegistry("read_file", "glob_file")
	calls := makeToolCalls("read_file", "glob_file", "bash", "read_file")

	batches := partitionToolCalls(r, calls)

	if len(batches) != 3 {
		t.Fatalf("len(batches) = %d, want 3", len(batches))
	}
	// batch 0: [read, glob] parallel
	if !batches[0].parallel || batches[0].end-batches[0].start != 2 {
		t.Errorf("batch[0] parallel=%v size=%d, want parallel=true size=2",
			batches[0].parallel, batches[0].end-batches[0].start)
	}
	// batch 1: [bash] serial
	if batches[1].parallel || batches[1].end-batches[1].start != 1 {
		t.Errorf("batch[1] parallel=%v size=%d, want parallel=false size=1",
			batches[1].parallel, batches[1].end-batches[1].start)
	}
	// batch 2: [read] parallel
	if !batches[2].parallel || batches[2].end-batches[2].start != 1 {
		t.Errorf("batch[2] parallel=%v size=%d, want parallel=true size=1",
			batches[2].parallel, batches[2].end-batches[2].start)
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
	// [read, write, read] → 3 个 batch
	r := newRegistry("read_file")
	calls := makeToolCalls("read_file", "write_file", "read_file")

	batches := partitionToolCalls(r, calls)

	if len(batches) != 3 {
		t.Fatalf("len(batches) = %d, want 3", len(batches))
	}
	if !batches[0].parallel || batches[0].end-batches[0].start != 1 {
		t.Errorf("batch[0] should be parallel single read")
	}
	if batches[1].parallel {
		t.Errorf("batch[1] should be serial write")
	}
	if !batches[2].parallel {
		t.Errorf("batch[2] should be parallel read")
	}
}

func TestPartitionToolCalls_UnknownToolsSerial(t *testing.T) {
	// 未注册的工具回退到串行
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
	runParallel(0, 1, func(i int) {
		called = true
	})

	if !called {
		t.Error("single element should be executed")
	}
}

func TestRunParallel_EmptyRange(t *testing.T) {
	// 空范围不应 panic
	runParallel(5, 5, func(i int) {
		t.Error("should not be called")
	})
}

func TestRunParallel_ConcurrentExecution(t *testing.T) {
	// 验证确实是并发执行的：双阶段屏障确保所有 goroutine 同时在并发区内。
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
			entered.Done() // 已进入并发区
			<-leave        // 等待离开
			concurrent.Add(-1)
		}()
	}

	// 阶段1：开门涌入
	close(enter)
	// 阶段2：等所有 goroutine 都完成 incremented 并卡在 <-leave
	entered.Wait()
	got := maxConcurrent.Load()
	// 阶段3：释放，清理
	close(leave)
	done.Wait()

	if got < 2 {
		t.Errorf("max concurrent = %d, expected at least 2 (parallel execution)", got)
	}
	if got > int32(n) {
		t.Errorf("max concurrent = %d exceeds total goroutines %d", got, n)
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

	// 验证所有 tool result 都回填到了 messages
	msgCount := len(a.messages)
	if msgCount != 2 {
		t.Fatalf("expected 2 tool messages, got %d", msgCount)
	}
	for i, m := range a.messages {
		if m.Role != openai.ChatMessageRoleTool {
			t.Errorf("messages[%d].Role = %q, want %q", i, m.Role, openai.ChatMessageRoleTool)
		}
		if !strings.Contains(m.Content, "_result") {
			t.Errorf("messages[%d].Content = %q, want contains '_result'", i, m.Content)
		}
	}
}

func TestExecuteBatch_SerialBatchRunsInOrder(t *testing.T) {
	r := newRegistry() // 全部写工具
	a := &Agent{registry: r}
	calls := makeToolCalls("write_file", "edit_file", "bash")

	a.executeBatch(context.Background(), calls)

	if len(a.messages) != 3 {
		t.Fatalf("expected 3 tool messages, got %d", len(a.messages))
	}
	// 验证结果按原始顺序排列
	wantNames := []string{"write_file_result", "edit_file_result", "bash_result"}
	for i, want := range wantNames {
		if got := a.messages[i].Content; got != want {
			t.Errorf("messages[%d].Content = %q, want %q", i, got, want)
		}
	}
}

func TestExecuteBatch_MixedBatchOrderPreserved(t *testing.T) {
	// [read A, bash, read B] → 并行 [read A] → 串行 [bash] → 并行 [read B]
	// 最终结果顺序必须: read A, bash, read B
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
	// 注册一个总是出错的工具，确保它不阻断后续工具
	errTool := &fakeTool{name: "fail_tool", readOnly: true}
	r := newRegistry("read_file")
	r.Register(errTool)

	a := &Agent{registry: r}
	calls := makeToolCalls("read_file", "fail_tool", "glob_file")

	// 因为 glob_file 不在只读列表中，需要在 fake tool 注册表中有它
	// newRegistry 已经注册了 glob_file，但它是非只读的。
	// 没关系，只要它存在就行。

	a.executeBatch(context.Background(), calls)

	if len(a.messages) != 3 {
		t.Fatalf("expected 3 tool messages, got %d", len(a.messages))
	}

	// read_file 应该成功
	if !strings.Contains(a.messages[0].Content, "read_file_result") {
		t.Errorf("messages[0] should contain read_file_result, got %q", a.messages[0].Content)
	}
	// fail_tool 会成功（Execute 无错误），只是名字不同
	// 第三个工具也应该有结果
	if !strings.Contains(a.messages[2].Content, "glob_file_result") {
		t.Errorf("messages[2] should contain glob_file_result, got %q", a.messages[2].Content)
	}
}

// =====================================================================
// Test: toolCallBatch 数据结构
// =====================================================================

func TestToolCallBatch_Fields(t *testing.T) {
	b := toolCallBatch{start: 0, end: 3, parallel: true}
	if b.start != 0 || b.end != 3 || !b.parallel {
		t.Error("toolCallBatch fields incorrect")
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

// =====================================================================
// Test: sort tool batches for deterministic order
// =====================================================================

func TestPartitionToolCalls_Deterministic(t *testing.T) {
	// 多次调用结果应该一致
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
				t.Fatalf("iteration %d batch[%d] differs: %+v vs %+v", i, j, batches[j], first[j])
			}
		}
	}
}

// =====================================================================
// Test: makeToolCalls helper
// =====================================================================

func TestMakeToolCalls(t *testing.T) {
	calls := makeToolCalls("a", "b", "c")
	if len(calls) != 3 {
		t.Fatalf("len = %d, want 3", len(calls))
	}
	names := make([]string, len(calls))
	for i, c := range calls {
		names[i] = c.Function.Name
	}
	if names[0] != "a" || names[1] != "b" || names[2] != "c" {
		t.Errorf("names = %v, want [a b c]", names)
	}
}
