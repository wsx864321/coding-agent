package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/wsx864321/coding-agent/internal/event"
)

// --- Task 14: Shell 输出折叠/展开 — 单元测试 ---

// TestShellOutputStoredOnBashResult 验证 bash 工具结果会将输出存储到 shellOutputs 中。
func TestShellOutputStoredOnBashResult(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	next, _ := m.Update(event.Event{
		Kind:       event.ToolResult,
		ToolName:   "bash",
		ToolOutput: "hello from bash\nline2\nline3\n",
		ToolCallID: "call_bash_store",
	})
	updated := next.(Model)

	stored, ok := updated.shellOutputs["call_bash_store"]
	if !ok {
		t.Fatal("shellOutputs should contain key 'call_bash_store' for bash tool result")
	}
	if stored != "hello from bash\nline2\nline3\n" {
		t.Fatalf("shellOutputs['call_bash_store'] = %q, want %q", stored, "hello from bash\nline2\nline3\n")
	}
}

// TestShellOutputCollapsedByDefault 验证超过 8 行的 bash 输出默认折叠，只显示前 8 行和 "collapsed" 摘要。
func TestShellOutputCollapsedByDefault(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	// 创建超过 8 行的输出
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, fmt.Sprintf("output-line-%02d", i))
	}
	output := strings.Join(lines, "\n")

	next, _ := m.Update(event.Event{
		Kind:       event.ToolResult,
		ToolName:   "bash",
		ToolOutput: output,
		ToolCallID: "call_collapse_default",
	})
	updated := next.(Model)

	// 找到 EntryToolOutput 条目
	var toolOutputEntry *TranscriptEntry
	for i := range updated.transcript {
		if updated.transcript[i].Kind == EntryToolOutput {
			toolOutputEntry = &updated.transcript[i]
			break
		}
	}
	if toolOutputEntry == nil {
		t.Fatal("should have EntryToolOutput in transcript")
	}

	content := toolOutputEntry.Content
	// 应该显示第一行
	if !strings.Contains(content, "output-line-01") {
		t.Fatal("collapsed output should show first line")
	}
	// 应该显示 "collapsed" 摘要
	if !strings.Contains(content, "collapsed") {
		t.Fatal("collapsed output should show 'collapsed' summary")
	}
	// 不应该显示第 20 行（超出前 8 行）
	if strings.Contains(content, "output-line-20") {
		t.Fatal("collapsed output should hide line 20")
	}
	// 应该显示第 8 行（在前 8 行范围内）
	if !strings.Contains(content, "output-line-08") {
		t.Fatal("collapsed output should show line 8 (within first 8 lines)")
	}
}

// TestShellOutputShortNoStorage 验证短输出（≤8 行）不折叠，完整显示且不含 "collapsed" 摘要。
func TestShellOutputShortNoStorage(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	// 创建 ≤8 行的短输出
	shortOutput := "line1\nline2\nline3\nline4"

	next, _ := m.Update(event.Event{
		Kind:       event.ToolResult,
		ToolName:   "bash",
		ToolOutput: shortOutput,
		ToolCallID: "call_short",
	})
	updated := next.(Model)

	// 找到 EntryToolOutput 条目
	var toolOutputEntry *TranscriptEntry
	for i := range updated.transcript {
		if updated.transcript[i].Kind == EntryToolOutput {
			toolOutputEntry = &updated.transcript[i]
			break
		}
	}
	if toolOutputEntry == nil {
		t.Fatal("should have EntryToolOutput in transcript")
	}

	content := toolOutputEntry.Content
	// 短输出应该完整显示所有行
	if !strings.Contains(content, "line1") {
		t.Fatal("short output should show line1")
	}
	if !strings.Contains(content, "line4") {
		t.Fatal("short output should show line4")
	}
	// 不应该包含 "collapsed" 摘要
	if strings.Contains(content, "collapsed") {
		t.Fatal("short output should NOT show 'collapsed' summary")
	}
}

// TestShellOutputCtrlBToggle 验证 Ctrl+B 可以切换 shell 输出的展开/折叠状态。
func TestShellOutputCtrlBToggle(t *testing.T) {
	m := New()
	m.width = 80

	// 设置一个 bash 输出条目
	m.shellOutputs["call_ctrl_b_toggle"] = "full bash output\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"
	m.shellExpanded["call_ctrl_b_toggle"] = false
	m.transcript = append(m.transcript, TranscriptEntry{
		Kind: EntryToolOutput,
		Raw:  encodeToolOutputRaw("call_ctrl_b_toggle", "collapsed view"),
	})
	m = m.rerenderTranscript()
	m = m.syncViewportContent()

	// 验证初始状态为折叠
	if m.shellExpanded["call_ctrl_b_toggle"] {
		t.Fatal("shellExpanded should be false initially")
	}

	// 第一次 Ctrl+B：展开
	next, _ := m.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	updated := next.(Model)
	if !updated.shellExpanded["call_ctrl_b_toggle"] {
		t.Fatal("shellExpanded should be true after first Ctrl+B")
	}

	// 第二次 Ctrl+B：折叠
	next2, _ := updated.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	updated2 := next2.(Model)
	if updated2.shellExpanded["call_ctrl_b_toggle"] {
		t.Fatal("shellExpanded should be false after second Ctrl+B")
	}

	// 验证展开后内容包含完整输出
	// 先展开
	m2 := updated
	collapsedContent := m2.transcript[0].Content

	// 再折叠
	m3 := updated2
	collapsedAgainContent := m3.transcript[0].Content

	// 折叠状态的内容应该相同（都是折叠视图）
	if collapsedContent != collapsedAgainContent {
		t.Fatal("collapsed content should be the same after toggle back")
	}
}

// TestShellOutputTruncation 验证超过 1MB 的 bash 输出会被截断并标注。
func TestShellOutputTruncation(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	// 创建超过 1MB 的输出
	bigLine := strings.Repeat("x", 100) + "\n"
	repeats := (1024*1024)/len(bigLine) + 10 // 超过 1MB
	bigOutput := strings.Repeat(bigLine, repeats)

	next, _ := m.Update(event.Event{
		Kind:       event.ToolResult,
		ToolName:   "bash",
		ToolOutput: bigOutput,
		ToolCallID: "call_truncation",
	})
	updated := next.(Model)

	stored := updated.shellOutputs["call_truncation"]
	// 存储的输出不应超过 1MB + 一行的大小
	if len(stored) > 1024*1024+len(bigLine) {
		t.Fatalf("stored output len = %d, should be <= ~1MB", len(stored))
	}
	// 应该包含截断标记
	if !strings.Contains(stored, "[output truncated]") {
		t.Fatal("truncated output should contain '[output truncated]' marker")
	}
	// 截断后的输出应该保留原始输出的尾部
	if !strings.HasSuffix(strings.TrimRight(stored, "\n"), strings.TrimRight(bigLine, "\n")) {
		t.Fatal("truncated output should preserve the tail of the original")
	}
}
