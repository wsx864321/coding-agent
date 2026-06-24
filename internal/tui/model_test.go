package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/wsx864321/coding-agent/internal/event"
)

func viewContent(m Model) string {
	return m.View().Content
}

func applyKey(m Model, msg tea.KeyPressMsg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func typeText(m Model, text string) Model {
	for _, r := range text {
		m = applyKey(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func prepareScrollModel() Model {
	m := New()
	m.width = 40
	m.height = 12
	m.transcript = longTranscriptHistory()
	m = m.rerenderTranscript()
	m = m.syncLayout()
	m = m.syncViewportContent()
	return m
}

func TestNewModelDefaults(t *testing.T) {
	m := New()
	if m.quitting {
		t.Fatal("new model should not be quitting")
	}
	if m.width != 0 || m.height != 0 {
		t.Fatalf("initial size = %dx%d, want 0x0", m.width, m.height)
	}
	if len(m.transcript) != 0 {
		t.Fatalf("transcript = %d, want 0", len(m.transcript))
	}
	if m.textarea.Value() != "" {
		t.Fatalf("textarea = %q, want empty", m.textarea.Value())
	}
	if m.viewport.YOffset() != 0 {
		t.Fatalf("viewport YOffset = %d, want 0", m.viewport.YOffset())
	}
}

func TestUpdateWindowSize(t *testing.T) {
	m := New()
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if cmd != nil {
		t.Fatal("WindowSizeMsg should not return a command")
	}

	updated, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", next)
	}
	if updated.width != 120 || updated.height != 40 {
		t.Fatalf("size = %dx%d, want 120x40", updated.width, updated.height)
	}
}

func TestUpdateCtrlCQuits(t *testing.T) {
	m := New()
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("Ctrl+C should return tea.Quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("command = %T, want tea.QuitMsg", cmd())
	}

	updated, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", next)
	}
	if !updated.quitting {
		t.Fatal("Ctrl+C should set quitting=true")
	}
}

func TestInitReturnsNil(t *testing.T) {
	m := New()
	if cmd := m.Init(); cmd != nil {
		t.Fatalf("Init() = %v, want nil", cmd)
	}
}

func TestInsertInputAppendsRunes(t *testing.T) {
	m := New()
	updated := typeText(m, "hi")
	if got := updated.textarea.Value(); got != "hi" {
		t.Fatalf("textarea = %q, want %q", got, "hi")
	}
}

func TestInsertInputAccumulates(t *testing.T) {
	m := New()
	m.textarea.SetValue("hel")
	updated := typeText(m, "lo")
	if got := updated.textarea.Value(); got != "hello" {
		t.Fatalf("textarea = %q, want %q", got, "hello")
	}
}

func TestBackspaceRemovesLastRune(t *testing.T) {
	m := New()
	m.textarea.SetValue("hello")
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	updated := next.(Model)
	if got := updated.textarea.Value(); got != "hell" {
		t.Fatalf("textarea = %q, want %q", got, "hell")
	}
}

func TestBackspaceOnEmptyInputIsNoop(t *testing.T) {
	m := New()
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	updated := next.(Model)
	if updated.textarea.Value() != "" {
		t.Fatalf("textarea = %q, want empty", updated.textarea.Value())
	}
}

func TestAppendEntryStoresKindAndRaw(t *testing.T) {
	m := New()
	m.width = 80
	m = m.appendEntry(TranscriptEntry{Kind: EntryUserMessage, Raw: "hello"})
	m = m.appendEntry(TranscriptEntry{Kind: EntryAssistantChunk, Raw: "world"})
	m = m.appendEntry(TranscriptEntry{Kind: EntryError, Raw: "notice"})

	if len(m.transcript) != 3 {
		t.Fatalf("transcript = %d, want 3", len(m.transcript))
	}
	if m.transcript[0].Kind != EntryUserMessage || m.transcript[0].Raw != "hello" {
		t.Fatalf("transcript[0] = %+v, want user/hello", m.transcript[0])
	}
	if m.transcript[1].Kind != EntryAssistantChunk || m.transcript[1].Raw != "world" {
		t.Fatalf("transcript[1] = %+v, want assistant/world", m.transcript[1])
	}
	if m.transcript[2].Kind != EntryError || m.transcript[2].Raw != "notice" {
		t.Fatalf("transcript[2] = %+v, want error/notice", m.transcript[2])
	}
}

func TestScrollUpMovesAwayFromBottom(t *testing.T) {
	m := prepareScrollModel()
	m.viewport.GotoBottom()
	before := m.viewport.YOffset()

	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if cmd != nil {
		t.Fatal("scroll up should not return a command")
	}
	updated := next.(Model)
	if updated.viewport.YOffset() >= before {
		t.Fatalf("YOffset = %d, want < %d after KeyUp", updated.viewport.YOffset(), before)
	}
}

func TestScrollDownIncreasesOffsetTowardBottom(t *testing.T) {
	m := prepareScrollModel()
	m.viewport.GotoBottom()
	bottom := m.viewport.YOffset()

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	scrolledUp := next.(Model)
	if scrolledUp.viewport.YOffset() >= bottom {
		t.Fatalf("expected scroll up to move away from bottom, got offset %d (bottom=%d)", scrolledUp.viewport.YOffset(), bottom)
	}

	next, _ = scrolledUp.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	scrolledDown := next.(Model)
	if scrolledDown.viewport.YOffset() <= scrolledUp.viewport.YOffset() {
		t.Fatalf("YOffset = %d, want > %d after KeyDown", scrolledDown.viewport.YOffset(), scrolledUp.viewport.YOffset())
	}
}

func TestScrollUpStopsAtTop(t *testing.T) {
	m := prepareScrollModel()
	m.viewport.GotoTop()

	for i := 0; i < 20; i++ {
		next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
		m = next.(Model)
	}
	if m.viewport.YOffset() != 0 {
		t.Fatalf("YOffset = %d, want 0 at top boundary", m.viewport.YOffset())
	}
}

func TestScrollDownStopsAtBottom(t *testing.T) {
	m := prepareScrollModel()
	m.viewport.GotoBottom()
	bottom := m.viewport.YOffset()

	for i := 0; i < 20; i++ {
		next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = next.(Model)
	}
	if m.viewport.YOffset() != bottom {
		t.Fatalf("YOffset = %d, want %d at bottom boundary", m.viewport.YOffset(), bottom)
	}
}

func TestViewRendersMessageInputAndHelpAreas(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m = m.appendUserMessage("你好")
	m = m.appendEntry(TranscriptEntry{Kind: EntryAssistantChunk, Raw: "你好，我是助手"})
	m = m.appendEntry(TranscriptEntry{Kind: EntryToolOutput, Raw: "系统提示"})
	m.textarea.SetValue("draft text")
	m = m.syncLayout()
	m = m.syncViewportContent()

	view := viewContent(m)
	for _, want := range []string{
		"coding-agent",
		"你好",
		"assistant:",
		"你好，我是助手",
		"系统提示",
		"draft text",
		"Enter",
		"Ctrl+C",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("View missing %q:\n%s", want, view)
		}
	}
}

func TestViewScrollHidesOlderMessages(t *testing.T) {
	m := New()
	m.width = 40
	m.height = 10
	for _, content := range []string{
		"first-visible-marker",
		"line-2",
		"line-3",
		"line-4",
		"line-5",
		"line-6",
		"line-7",
		"line-8",
		"last-visible-marker",
	} {
		m = m.appendUserMessage(content)
	}
	m = m.syncLayout()
	m = m.syncViewportContent()

	bottomView := viewContent(m)
	if !strings.Contains(bottomView, "last-visible-marker") {
		t.Fatalf("bottom view should show latest message:\n%s", bottomView)
	}

	m.viewport.GotoTop()
	topView := viewContent(m)
	if strings.Contains(topView, "last-visible-marker") {
		t.Fatalf("top view should hide latest message when scrolled up:\n%s", topView)
	}
	if !strings.Contains(topView, "first-visible-marker") {
		t.Fatalf("top view should show earliest message:\n%s", topView)
	}
}

func TestSpaceKeyGoesToTextareaWhenHasText(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m = m.layout()
	m.textarea.SetValue("hello")

	next, _ := m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	updated := next.(Model)
	if got := updated.textarea.Value(); !strings.Contains(got, " ") {
		t.Fatalf("textarea = %q, want space in text when textarea has content", got)
	}
}

func TestSpaceKeyScrollsViewportWhenEmpty(t *testing.T) {
	m := prepareScrollModel()
	m.viewport.GotoTop()
	before := m.viewport.YOffset()

	next, _ := m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	updated := next.(Model)
	if updated.viewport.YOffset() <= before {
		t.Fatalf("YOffset = %d, want > %d when textarea empty and space pressed", updated.viewport.YOffset(), before)
	}
}

func TestLetterKeysGoToTextareaWhenHasText(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m = m.layout()
	m.textarea.SetValue("test")

	for _, ch := range "bfudhjkl" {
		next, _ := m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m = next.(Model)
	}
	if got := m.textarea.Value(); got != "testbfudhjkl" {
		t.Fatalf("textarea = %q, want %q", got, "testbfudhjkl")
	}
}

func longTranscriptHistory() []TranscriptEntry {
	entries := make([]TranscriptEntry, 0, 20)
	for i := 1; i <= 20; i++ {
		raw := strings.Repeat("x", i*3)
		entries = append(entries, TranscriptEntry{Kind: EntryUserMessage, Raw: raw})
	}
	return entries
}

func TestEntryReasoningKindExists(t *testing.T) {
	// EntryReasoning must be a distinct EntryKind with value 5.
	if EntryReasoning != 5 {
		t.Fatalf("EntryReasoning = %d, want 5", EntryReasoning)
	}
}

func TestNewModelInitializesReasoningFields(t *testing.T) {
	m := New()
	if m.reasoning == nil {
		t.Fatal("reasoning should be initialized in New()")
	}
	if m.reasoning.Len() != 0 {
		t.Fatalf("reasoning.Len() = %d, want 0", m.reasoning.Len())
	}
	if m.reasoningLineIdx != -1 {
		t.Fatal("reasoningLineIdx should default to -1")
	}
	if m.showReasoning {
		t.Fatal("showReasoning should default to false")
	}
	if !m.thinkStart.IsZero() {
		t.Fatal("thinkStart should be zero time initially")
	}
}

func TestAppendEntryReasoning(t *testing.T) {
	m := New()
	m.width = 80
	m = m.appendEntry(TranscriptEntry{Kind: EntryReasoning, Raw: "thinking..."})
	if len(m.transcript) != 1 {
		t.Fatalf("transcript = %d, want 1", len(m.transcript))
	}
	if m.transcript[0].Kind != EntryReasoning {
		t.Fatalf("transcript[0].Kind = %v, want EntryReasoning", m.transcript[0].Kind)
	}
	if m.transcript[0].Raw != "thinking..." {
		t.Fatalf("transcript[0].Raw = %q, want %q", m.transcript[0].Raw, "thinking...")
	}
}

func TestIngestReasoningChunkFirstCreatesEntry(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	m = m.ingestReasoningChunk("let me think...")

	if len(m.transcript) != 1 {
		t.Fatalf("transcript = %d, want 1", len(m.transcript))
	}
	if m.transcript[0].Kind != EntryReasoning {
		t.Fatalf("transcript[0].Kind = %v, want EntryReasoning", m.transcript[0].Kind)
	}
	if m.transcript[0].Raw != "let me think..." {
		t.Fatalf("transcript[0].Raw = %q, want 'let me think...'", m.transcript[0].Raw)
	}
	if m.reasoning.String() != "let me think..." {
		t.Fatalf("reasoning = %q, want 'let me think...'", m.reasoning.String())
	}
	if m.reasoningLineIdx != 0 {
		t.Fatalf("reasoningLineIdx = %d, want 0", m.reasoningLineIdx)
	}
}

func TestIngestReasoningChunkSubsequentUpdatesEntry(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	m = m.ingestReasoningChunk("hello")
	m = m.ingestReasoningChunk(" world")

	if len(m.transcript) != 1 {
		t.Fatalf("transcript = %d, want 1", len(m.transcript))
	}
	if m.transcript[0].Raw != "hello world" {
		t.Fatalf("transcript[0].Raw = %q, want 'hello world'", m.transcript[0].Raw)
	}
	if m.reasoning.String() != "hello world" {
		t.Fatalf("reasoning = %q, want 'hello world'", m.reasoning.String())
	}
}

func TestIngestReasoningChunkNoopWhenNotBusy(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = false

	m = m.ingestReasoningChunk("silent")

	if len(m.transcript) != 0 {
		t.Fatalf("transcript = %d, want 0 when not busy", len(m.transcript))
	}
}

func TestRenderReasoningSummary(t *testing.T) {
	m := New()
	m.width = 80

	summary := m.renderReasoningSummary("thinking...")
	if summary == "" {
		t.Fatal("renderReasoningSummary should return non-empty string")
	}
	if !strings.Contains(summary, "thinking...") {
		t.Fatalf("summary should contain the reasoning text, got: %q", summary)
	}
}

func TestRenderReasoningEntryExpanded(t *testing.T) {
	m := New()
	m.width = 80
	m.showReasoning = true

	e := TranscriptEntry{Kind: EntryReasoning, Raw: "step by step analysis"}
	e = m.renderReasoningEntry(e)

	if e.Content == "" {
		t.Fatal("renderReasoningEntry should produce content")
	}
	// Expanded: should contain summary line, separator, and reasoning text
	if !strings.Contains(e.Content, "step by step analysis") {
		t.Fatalf("expanded content should contain reasoning text, got: %q", e.Content)
	}
}

func TestRenderReasoningEntryCollapsed(t *testing.T) {
	m := New()
	m.width = 80
	m.showReasoning = false

	e := TranscriptEntry{Kind: EntryReasoning, Raw: "step by step analysis"}
	e = m.renderReasoningEntry(e)

	if e.Content == "" {
		t.Fatal("renderReasoningEntry should produce content even when collapsed")
	}
	// Collapsed: should contain summary line with the raw text
	if !strings.Contains(e.Content, "step by step analysis") {
		t.Fatalf("collapsed content should contain summary, got: %q", e.Content)
	}
	// Collapsed: should NOT contain a separator line (only expanded has separator)
	if strings.Contains(e.Content, "──") {
		t.Fatalf("collapsed content should not contain separator, got: %q", e.Content)
	}
}

func TestReasoningTextEventHandling(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true
	ch := make(chan event.Event, 1)
	m.streamCh = ch

	next, cmd := m.Update(event.Event{Kind: event.ReasoningText, ReasoningChunk: "analyzing..."})
	updated := next.(Model)

	if len(updated.transcript) != 1 {
		t.Fatalf("transcript = %d, want 1 after ReasoningText", len(updated.transcript))
	}
	if updated.transcript[0].Kind != EntryReasoning {
		t.Fatalf("transcript[0].Kind = %v, want EntryReasoning", updated.transcript[0].Kind)
	}
	if cmd == nil {
		t.Fatal("ReasoningText should return follow-up command to continue stream")
	}
}

func TestReasoningDimStyleExists(t *testing.T) {
	// reasoningDimStyle must be a non-nil lipgloss style with Faint(true)
	if reasoningDimStyle.GetFaint() != true {
		t.Fatal("reasoningDimStyle should have Faint(true)")
	}
}

func TestRenderEntryHandlesEntryReasoning(t *testing.T) {
	m := New()
	m.width = 80
	m.showReasoning = true

	e := TranscriptEntry{Kind: EntryReasoning, Raw: "think step"}
	e = m.renderEntry(e)

	if e.Content == "" {
		t.Fatal("renderEntry should produce content for EntryReasoning")
	}
	if !strings.Contains(e.Content, "think step") {
		t.Fatalf("rendered content should contain reasoning text, got: %q", e.Content)
	}
}

func TestReasoningResetOnSubmit(t *testing.T) {
	m, _ := newStubModel([]string{"ok"})
	m.width = 80
	m.textarea.SetValue("hello")
	m.reasoning.WriteString("old thinking")
	m.reasoningLineIdx = 3
	m.showReasoning = true

	updated, _ := m.submit()

	if updated.reasoning.Len() != 0 {
		t.Fatalf("reasoning should be reset on submit, got %q", updated.reasoning.String())
	}
	if updated.reasoningLineIdx != -1 {
		t.Fatalf("reasoningLineIdx should be reset to -1, got %d", updated.reasoningLineIdx)
	}
	if updated.showReasoning {
		t.Fatal("showReasoning should be reset to false on submit")
	}
}

func TestTextEventFinalizesReasoningSummary(t *testing.T) {
	// When a Text event arrives while reasoningLineIdx >= 0, the reasoning summary
	// should be updated to show "completed" state and reasoningLineIdx should be set to -1.
	m := New()
	m.width = 80
	m.busy = true
	m = m.ingestReasoningChunk("thinking step by step")
	m.showReasoning = false

	if m.reasoningLineIdx < 0 {
		t.Fatal("reasoningLineIdx should be >= 0 after ingestReasoningChunk")
	}

	// Send Text event to signal reasoning completion
	next, _ := m.Update(event.Event{Kind: event.Text, Text: "answer"})
	updated := next.(Model)

	if updated.reasoningLineIdx != -1 {
		t.Fatalf("reasoningLineIdx = %d, want -1 after Text event", updated.reasoningLineIdx)
	}
	// The transcript entry at the old reasoningLineIdx should still exist
	if len(updated.transcript) < 1 {
		t.Fatal("transcript should have at least one entry")
	}
}

func TestCtrlOTogglesShowReasoning(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true
	m = m.ingestReasoningChunk("my analysis")
	if m.reasoningLineIdx < 0 {
		t.Fatal("reasoningLineIdx should be >= 0 after ingestReasoningChunk")
	}

	// Initially showReasoning is false
	if m.showReasoning {
		t.Fatal("showReasoning should initially be false")
	}

	// Press Ctrl+O to toggle showReasoning to true
	next, _ := m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	updated := next.(Model)

	if !updated.showReasoning {
		t.Fatal("showReasoning should be true after first Ctrl+O")
	}

	// Press Ctrl+O again to toggle back to false
	next, _ = updated.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	updated = next.(Model)

	if updated.showReasoning {
		t.Fatal("showReasoning should be false after second Ctrl+O")
	}
}

func TestCtrlORerendersReasoningEntry(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true
	m = m.ingestReasoningChunk("detailed analysis")
	if m.reasoningLineIdx < 0 {
		t.Fatal("reasoningLineIdx should be >= 0 after ingestReasoningChunk")
	}

	// Get content before toggle (collapsed)
	beforeContent := m.transcript[m.reasoningLineIdx].Content

	// Toggle to expanded
	next, _ := m.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	updated := next.(Model)

	if !updated.showReasoning {
		t.Fatal("showReasoning should be true after Ctrl+O")
	}

	// Content should be re-rendered in expanded form
	afterContent := updated.transcript[updated.reasoningLineIdx].Content

	// Expanded content should be longer (includes separator and full text)
	if len(afterContent) <= len(beforeContent) {
		t.Fatalf("expanded content should be longer than collapsed, before=%d, after=%d", len(beforeContent), len(afterContent))
	}
}

func TestNewModelInitializesToolStreamFields(t *testing.T) {
	m := New()
	if m.toolStreamIdx != -1 {
		t.Fatalf("toolStreamIdx = %d, want -1", m.toolStreamIdx)
	}
	if m.toolStreamID != "" {
		t.Fatalf("toolStreamID = %q, want empty", m.toolStreamID)
	}
	if m.toolTail != nil {
		t.Fatalf("toolTail = %v, want nil", m.toolTail)
	}
	if m.toolPartial != "" {
		t.Fatalf("toolPartial = %q, want empty", m.toolPartial)
	}
	if m.toolLineCount != 0 {
		t.Fatalf("toolLineCount = %d, want 0", m.toolLineCount)
	}
	if !m.toolStreamStart.IsZero() {
		t.Fatal("toolStreamStart should be zero time initially")
	}
}

func TestNewModelInitializesShellOutputFields(t *testing.T) {
	m := New()
	if m.shellOutputs == nil {
		t.Fatal("shellOutputs should be initialized in New()")
	}
	if len(m.shellOutputs) != 0 {
		t.Fatalf("shellOutputs len = %d, want 0", len(m.shellOutputs))
	}
	if m.shellExpanded == nil {
		t.Fatal("shellExpanded should be initialized in New()")
	}
	if len(m.shellExpanded) != 0 {
		t.Fatalf("shellExpanded len = %d, want 0", len(m.shellExpanded))
	}
}

// --- Task 12: Shell output storage & 1MB truncation ---

func TestToolResultBashStoresInShellOutputs(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	next, _ := m.Update(event.Event{
		Kind:       event.ToolResult,
		ToolName:   "bash",
		ToolOutput: "file list:\nfoo.go\nbar.go\n",
		ToolCallID: "call_bash_1",
	})
	updated := next.(Model)

	if got, ok := updated.shellOutputs["call_bash_1"]; !ok {
		t.Fatal("shellOutputs should contain key 'call_bash_1' for bash tool")
	} else if got != "file list:\nfoo.go\nbar.go\n" {
		t.Fatalf("shellOutputs['call_bash_1'] = %q, want %q", got, "file list:\nfoo.go\nbar.go\n")
	}
}

func TestToolResultNonBashNotStored(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	next, _ := m.Update(event.Event{
		Kind:       event.ToolResult,
		ToolName:   "read_file",
		ToolOutput: "some content",
		ToolCallID: "call_read_1",
	})
	updated := next.(Model)

	if _, ok := updated.shellOutputs["call_read_1"]; ok {
		t.Fatal("shellOutputs should NOT contain key for non-bash tool")
	}
}

func TestToolResultBashTruncatesAt1MB(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	// Create output larger than 1MB
	bigLine := strings.Repeat("x", 100) + "\n"
	repeats := (1024*1024)/len(bigLine) + 10 // exceed 1MB
	bigOutput := strings.Repeat(bigLine, repeats)

	next, _ := m.Update(event.Event{
		Kind:       event.ToolResult,
		ToolName:   "bash",
		ToolOutput: bigOutput,
		ToolCallID: "call_big",
	})
	updated := next.(Model)

	stored := updated.shellOutputs["call_big"]
	if len(stored) > 1024*1024+len(bigLine) {
		t.Fatalf("stored output len = %d, should be <= ~1MB", len(stored))
	}
	if !strings.Contains(stored, "[output truncated]") {
		t.Fatal("truncated output should contain '[output truncated]' marker")
	}
	// The stored output should end with the tail of the original
	if !strings.HasSuffix(strings.TrimRight(stored, "\n"), strings.TrimRight(bigLine, "\n")) {
		t.Fatal("truncated output should preserve the tail of the original")
	}
}

func TestToolResultBashSmallOutputNotTruncated(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	small := "echo hello"
	next, _ := m.Update(event.Event{
		Kind:       event.ToolResult,
		ToolName:   "bash",
		ToolOutput: small,
		ToolCallID: "call_small",
	})
	updated := next.(Model)

	stored := updated.shellOutputs["call_small"]
	if stored != small {
		t.Fatalf("small output should not be modified, got %q", stored)
	}
	if strings.Contains(stored, "[output truncated]") {
		t.Fatal("small output should not have truncation marker")
	}
}

func TestToolResultBashOutputCollapsedRendering(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	// Create output with more than 8 lines
	var lines []string
	for i := 1; i <= 15; i++ {
		lines = append(lines, fmt.Sprintf("line-%02d", i))
	}
	output := strings.Join(lines, "\n")

	next, _ := m.Update(event.Event{
		Kind:       event.ToolResult,
		ToolName:   "bash",
		ToolOutput: output,
		ToolCallID: "call_collapse",
	})
	updated := next.(Model)

	// Find the EntryToolOutput entry
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
	if !strings.Contains(content, "line-01") {
		t.Fatal("collapsed output should show first line")
	}
	if !strings.Contains(content, "collapsed") {
		t.Fatal("collapsed output should show 'collapsed' summary")
	}
	if strings.Contains(content, "line-15") {
		t.Fatal("collapsed output should hide line 15")
	}
}

func TestIngestDrainEventBashStoresOutput(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	m = m.ingestDrainEvent(event.Event{
		Kind:       event.ToolResult,
		ToolName:   "bash",
		ToolOutput: "drain output",
		ToolCallID: "call_drain",
	})

	if got, ok := m.shellOutputs["call_drain"]; !ok {
		t.Fatal("shellOutputs should contain key 'call_drain' after ingestDrainEvent")
	} else if got != "drain output" {
		t.Fatalf("shellOutputs['call_drain'] = %q, want 'drain output'", got)
	}
}

func TestInterruptTurnResetsReasoningState(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true
	m.reasoning.WriteString("interrupted thinking")
	m.reasoningLineIdx = 0
	m.showReasoning = true
	m.transcript = append(m.transcript, TranscriptEntry{Kind: EntryReasoning, Raw: "interrupted thinking"})

	updated := m.interruptTurn()

	if updated.reasoning.Len() != 0 {
		t.Fatalf("reasoning should be reset on interrupt, got %q", updated.reasoning.String())
	}
	if updated.reasoningLineIdx != -1 {
		t.Fatalf("reasoningLineIdx = %d, want -1 after interrupt", updated.reasoningLineIdx)
	}
	if updated.showReasoning {
		t.Fatal("showReasoning should be reset to false on interrupt")
	}
}

// --- Task 13: Ctrl+B shell output toggle ---

func TestEncodeDecodeToolOutputRaw(t *testing.T) {
	toolCallID := "call_bash_abc"
	output := "line1\nline2\nline3"

	encoded := encodeToolOutputRaw(toolCallID, output)
	if encoded == "" {
		t.Fatal("encodeToolOutputRaw should return non-empty string")
	}

	decodedID, decodedOutput := decodeToolOutputRaw(encoded)
	if decodedID != toolCallID {
		t.Fatalf("decoded toolCallID = %q, want %q", decodedID, toolCallID)
	}
	if decodedOutput != output {
		t.Fatalf("decoded output = %q, want %q", decodedOutput, output)
	}
}

func TestDecodeToolOutputRawLegacy(t *testing.T) {
	// Legacy format: raw is just the output string (no toolCallID prefix)
	legacy := "just plain output"
	decodedID, decodedOutput := decodeToolOutputRaw(legacy)
	if decodedID != "" {
		t.Fatalf("decoded toolCallID from legacy = %q, want empty", decodedID)
	}
	if decodedOutput != legacy {
		t.Fatalf("decoded output from legacy = %q, want %q", decodedOutput, legacy)
	}
}

func TestToolResultBashWritesToolCallIDToRaw(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	next, _ := m.Update(event.Event{
		Kind:       event.ToolResult,
		ToolName:   "bash",
		ToolOutput: "hello world",
		ToolCallID: "call_bash_x",
	})
	updated := next.(Model)

	// Find the EntryToolOutput entry
	var outputEntry *TranscriptEntry
	for i := range updated.transcript {
		if updated.transcript[i].Kind == EntryToolOutput {
			outputEntry = &updated.transcript[i]
			break
		}
	}
	if outputEntry == nil {
		t.Fatal("should have EntryToolOutput in transcript")
	}

	decodedID, decodedOutput := decodeToolOutputRaw(outputEntry.Raw)
	if decodedID != "call_bash_x" {
		t.Fatalf("decoded toolCallID = %q, want 'call_bash_x'", decodedID)
	}
	if decodedOutput != "hello world" {
		t.Fatalf("decoded output = %q, want 'hello world'", decodedOutput)
	}
}

func TestToggleShellExpandTogglesState(t *testing.T) {
	m := New()
	m.width = 80

	// Set up: a bash tool output entry with toolCallID in Raw
	m.shellOutputs["call_bash_t"] = "full output content"
	m.shellExpanded["call_bash_t"] = false
	m.transcript = append(m.transcript, TranscriptEntry{
		Kind: EntryToolOutput,
		Raw:  encodeToolOutputRaw("call_bash_t", "collapsed view"),
	})
	m.transcript[0] = m.renderEntry(m.transcript[0])
	m = m.syncViewportContent()

	// First toggle: expand
	m = m.toggleShellExpand()
	if !m.shellExpanded["call_bash_t"] {
		t.Fatal("shellExpanded should be true after first toggle")
	}

	// Second toggle: collapse
	m = m.toggleShellExpand()
	if m.shellExpanded["call_bash_t"] {
		t.Fatal("shellExpanded should be false after second toggle")
	}
}

func TestToggleShellExpandReversesScan(t *testing.T) {
	m := New()
	m.width = 80

	// Two bash outputs: the last one should be found by reverse scan
	m.shellOutputs["call_first"] = "first output"
	m.shellOutputs["call_last"] = "last output"
	m.shellExpanded["call_first"] = false
	m.shellExpanded["call_last"] = false

	m.transcript = append(m.transcript, TranscriptEntry{
		Kind: EntryToolOutput,
		Raw:  encodeToolOutputRaw("call_first", "first collapsed"),
	})
	m.transcript = append(m.transcript, TranscriptEntry{
		Kind: EntryToolOutput,
		Raw:  encodeToolOutputRaw("call_last", "last collapsed"),
	})
	m = m.rerenderTranscript()
	m = m.syncViewportContent()

	// Reverse scan should find "call_last" (the most recent bash output)
	m = m.toggleShellExpand()
	if m.shellExpanded["call_first"] {
		t.Fatal("shellExpanded['call_first'] should remain false (not the nearest)")
	}
	if !m.shellExpanded["call_last"] {
		t.Fatal("shellExpanded['call_last'] should be true (nearest bash output)")
	}
}

func TestToggleShellExpandSkipsNonBashOutputs(t *testing.T) {
	m := New()
	m.width = 80

	// A non-bash output (legacy format, no toolCallID) followed by a bash output
	m.shellOutputs["call_bash_s"] = "bash output"
	m.shellExpanded["call_bash_s"] = false

	m.transcript = append(m.transcript, TranscriptEntry{
		Kind: EntryToolOutput,
		Raw:  "plain output without toolCallID",
	})
	m.transcript = append(m.transcript, TranscriptEntry{
		Kind: EntryToolOutput,
		Raw:  encodeToolOutputRaw("call_bash_s", "bash collapsed"),
	})
	m = m.rerenderTranscript()
	m = m.syncViewportContent()

	// Reverse scan should skip the legacy entry and find "call_bash_s"
	m = m.toggleShellExpand()
	if !m.shellExpanded["call_bash_s"] {
		t.Fatal("shellExpanded['call_bash_s'] should be true (found after skipping legacy)")
	}
}

func TestToggleShellExpandNoopWhenNoBashOutput(t *testing.T) {
	m := New()
	m.width = 80

	// No bash outputs at all
	m.transcript = append(m.transcript, TranscriptEntry{
		Kind: EntryUserMessage,
		Raw:  "hello",
	})
	m = m.rerenderTranscript()
	m = m.syncViewportContent()

	before := m.renderTranscriptContent()
	m = m.toggleShellExpand()
	after := m.renderTranscriptContent()

	if before != after {
		t.Fatal("toggleShellExpand should be noop when no bash output found")
	}
}

func TestCtrlBTogglesShellExpand(t *testing.T) {
	m := New()
	m.width = 80

	// Set up a bash output entry
	m.shellOutputs["call_ctrl_b"] = "full bash output for ctrl+b test"
	m.shellExpanded["call_ctrl_b"] = false
	m.transcript = append(m.transcript, TranscriptEntry{
		Kind: EntryToolOutput,
		Raw:  encodeToolOutputRaw("call_ctrl_b", "collapsed bash"),
	})
	m = m.rerenderTranscript()
	m = m.syncViewportContent()

	// Press Ctrl+B
	next, _ := m.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	updated := next.(Model)

	if !updated.shellExpanded["call_ctrl_b"] {
		t.Fatal("shellExpanded should be true after Ctrl+B")
	}
}

func TestCtrlBNoopWhenBusy(t *testing.T) {
	m := New()
	m.width = 80
	m.busy = true

	m.shellOutputs["call_busy"] = "output"
	m.shellExpanded["call_busy"] = false
	m.transcript = append(m.transcript, TranscriptEntry{
		Kind: EntryToolOutput,
		Raw:  encodeToolOutputRaw("call_busy", "collapsed"),
	})
	m = m.rerenderTranscript()

	// Ctrl+B while busy should be a noop
	next, _ := m.Update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	updated := next.(Model)

	if updated.shellExpanded["call_busy"] {
		t.Fatal("shellExpanded should remain false when busy")
	}
}

// --- Task 19: 文本选择与复制 — 鼠标交互处理 ---

func TestNewModelInitializesSelectionFields(t *testing.T) {
	m := New()
	if m.sel.active {
		t.Fatal("sel.active should default to false")
	}
	if m.sel.dragging {
		t.Fatal("sel.dragging should default to false")
	}
}

func TestMouseLeftClickStartsSelection(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m = m.layout()

	// Left click at position (5, 3) — X=col, Y=line in viewport content
	next, _ := m.Update(tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	updated := next.(Model)

	if !updated.sel.active {
		t.Fatal("sel.active should be true after left click")
	}
	if updated.sel.dragging {
		t.Fatal("sel.dragging should be false after click (not drag yet)")
	}
	if updated.sel.startLine != 3 || updated.sel.startCol != 5 {
		t.Fatalf("start position = (%d, %d), want (3, 5)", updated.sel.startLine, updated.sel.startCol)
	}
	if updated.sel.endLine != 3 || updated.sel.endCol != 5 {
		t.Fatalf("end position = (%d, %d), want (3, 5)", updated.sel.endLine, updated.sel.endCol)
	}
}

func TestMouseMotionExtendsSelection(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m = m.layout()

	// Start selection with left click
	next, _ := m.Update(tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	updated := next.(Model)

	// Drag to extend
	next, _ = updated.Update(tea.MouseMotionMsg{X: 15, Y: 7, Button: tea.MouseLeft})
	updated = next.(Model)

	if !updated.sel.active {
		t.Fatal("sel.active should remain true during drag")
	}
	if !updated.sel.dragging {
		t.Fatal("sel.dragging should be true during drag")
	}
	if updated.sel.startLine != 3 || updated.sel.startCol != 5 {
		t.Fatalf("start position should stay = (%d, %d), got (%d, %d)", 3, 5, updated.sel.startLine, updated.sel.startCol)
	}
	if updated.sel.endLine != 7 || updated.sel.endCol != 15 {
		t.Fatalf("end position = (%d, %d), want (7, 15)", updated.sel.endLine, updated.sel.endCol)
	}
}

func TestMouseReleaseKeepsSelection(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m = m.layout()

	// Click and drag
	next, _ := m.Update(tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	updated := next.(Model)
	next, _ = updated.Update(tea.MouseMotionMsg{X: 15, Y: 7, Button: tea.MouseLeft})
	updated = next.(Model)

	// Release
	next, _ = updated.Update(tea.MouseReleaseMsg{X: 15, Y: 7, Button: tea.MouseLeft})
	updated = next.(Model)

	if !updated.sel.active {
		t.Fatal("sel.active should remain true after release")
	}
	if updated.sel.dragging {
		t.Fatal("sel.dragging should be false after release")
	}
	if updated.sel.endLine != 7 || updated.sel.endCol != 15 {
		t.Fatalf("end position should be preserved, got (%d, %d)", updated.sel.endLine, updated.sel.endCol)
	}
}

func TestMouseClickWithoutDragCancelsSelection(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m = m.layout()

	// First click starts selection
	next, _ := m.Update(tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	updated := next.(Model)

	if !updated.sel.active {
		t.Fatal("sel.active should be true after first click")
	}

	// Second click at same position without drag should cancel
	next, _ = updated.Update(tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	updated = next.(Model)

	if updated.sel.active {
		t.Fatal("sel.active should be false after click cancels selection")
	}
}

func TestMouseClickAtDifferentPositionCancelsSelection(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m = m.layout()

	// Start selection
	next, _ := m.Update(tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	updated := next.(Model)

	// Click elsewhere without drag
	next, _ = updated.Update(tea.MouseClickMsg{X: 20, Y: 10, Button: tea.MouseLeft})
	updated = next.(Model)

	if updated.sel.active {
		t.Fatal("sel.active should be false after click at different position")
	}
}

func TestMouseWheelExtendsSelectionWhenActive(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m = m.layout()

	// Start selection
	next, _ := m.Update(tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	updated := next.(Model)

	// Scroll down while selecting
	next, _ = updated.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	updated = next.(Model)

	// endLine should have increased (scrolled down extends selection)
	if updated.sel.endLine <= 3 {
		t.Fatalf("endLine = %d, want > 3 after wheel down", updated.sel.endLine)
	}
}

func TestMouseWheelDoesNotExtendWhenNotSelecting(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m = m.layout()

	// No active selection
	next, _ := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	updated := next.(Model)

	if updated.sel.active {
		t.Fatal("sel.active should remain false when not selecting")
	}
}

func TestPageUpExtendsSelectionWhenActive(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m = m.layout()

	// Start selection at line 10
	next, _ := m.Update(tea.MouseClickMsg{X: 5, Y: 10, Button: tea.MouseLeft})
	updated := next.(Model)

	// PageUp while selecting
	next, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	updated = next.(Model)

	// endLine should have decreased (scrolled up extends selection upward)
	if updated.sel.endLine >= 10 {
		t.Fatalf("endLine = %d, want < 10 after PgUp", updated.sel.endLine)
	}
}

func TestPageDownExtendsSelectionWhenActive(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m = m.layout()

	// Start selection at line 5
	next, _ := m.Update(tea.MouseClickMsg{X: 5, Y: 5, Button: tea.MouseLeft})
	updated := next.(Model)

	// PageDown while selecting
	next, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	updated = next.(Model)

	// endLine should have increased
	if updated.sel.endLine <= 5 {
		t.Fatalf("endLine = %d, want > 5 after PgDown", updated.sel.endLine)
	}
}

func TestScrollKeysDoNotExtendWhenNotSelecting(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m = m.layout()

	// No active selection, press PgUp
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	updated := next.(Model)

	if updated.sel.active {
		t.Fatal("sel.active should remain false when not selecting")
	}
}

func TestRightClickDoesNotStartSelection(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m = m.layout()

	next, _ := m.Update(tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseRight})
	updated := next.(Model)

	if updated.sel.active {
		t.Fatal("sel.active should be false after right click")
	}
}

func TestToggleShellExpandRerendersEntry(t *testing.T) {
	m := New()
	m.width = 80

	m.shellOutputs["call_render"] = "expanded\nmulti\nline\noutput"
	m.shellExpanded["call_render"] = false
	m.transcript = append(m.transcript, TranscriptEntry{
		Kind: EntryToolOutput,
		Raw:  encodeToolOutputRaw("call_render", "collapsed"),
	})
	m.transcript[0] = m.renderEntry(m.transcript[0])
	m = m.syncViewportContent()

	collapsedContent := m.transcript[0].Content

	// Toggle to expand
	m = m.toggleShellExpand()
	expandedContent := m.transcript[0].Content

	if expandedContent == collapsedContent {
		t.Fatal("content should change after toggleShellExpand")
	}
	if !strings.Contains(expandedContent, "expanded") {
		t.Fatalf("expanded content should contain full output, got: %q", expandedContent)
	}
}

// --- Task 24: /diff-fold 斜杠命令 ---

func TestDiffFoldSetsMaxLines(t *testing.T) {
	m, _ := newStubModel(nil)
	m.textarea.SetValue("/diff-fold 10")

	updated, _ := m.submit()

	if updated.diffMaxLines != 10 {
		t.Fatalf("diffMaxLines = %d, want 10", updated.diffMaxLines)
	}
	if updated.textarea.Value() != "" {
		t.Fatalf("textarea = %q, want cleared", updated.textarea.Value())
	}
	if updated.busy {
		t.Fatal("busy should be false — /diff-fold should not start a turn")
	}
	if updated.statusMsg == "" {
		t.Fatal("statusMsg should be set after /diff-fold command")
	}
}

func TestDiffFoldZeroDisablesLimit(t *testing.T) {
	m, _ := newStubModel(nil)
	m.diffMaxLines = 5 // start with a limit
	m.textarea.SetValue("/diff-fold 0")

	updated, _ := m.submit()

	if updated.diffMaxLines != 0 {
		t.Fatalf("diffMaxLines = %d, want 0 (no limit)", updated.diffMaxLines)
	}
}

func TestDiffFoldNoArgsShowsCurrent(t *testing.T) {
	m, _ := newStubModel(nil)
	m.diffMaxLines = 7
	m.textarea.SetValue("/diff-fold")

	updated, _ := m.submit()

	// diffMaxLines should stay unchanged
	if updated.diffMaxLines != 7 {
		t.Fatalf("diffMaxLines = %d, want 7 (unchanged)", updated.diffMaxLines)
	}
	if updated.statusMsg == "" {
		t.Fatal("statusMsg should show current value")
	}
	if updated.busy {
		t.Fatal("busy should remain false — /diff-fold should not start a turn")
	}
}

func TestDiffFoldInvalidArgShowsError(t *testing.T) {
	m, _ := newStubModel(nil)
	m.diffMaxLines = 3
	m.textarea.SetValue("/diff-fold abc")

	updated, _ := m.submit()

	// diffMaxLines should stay unchanged on invalid input
	if updated.diffMaxLines != 3 {
		t.Fatalf("diffMaxLines = %d, want 3 (unchanged on error)", updated.diffMaxLines)
	}
	if updated.statusMsg == "" {
		t.Fatal("statusMsg should show error for invalid argument")
	}
	if updated.busy {
		t.Fatal("busy should be false — /diff-fold should not start a turn")
	}
}

func TestDiffFoldClearsTextarea(t *testing.T) {
	m, _ := newStubModel(nil)
	m.textarea.SetValue("/diff-fold 25")

	updated, _ := m.submit()

	if updated.textarea.Value() != "" {
		t.Fatalf("textarea = %q, want empty after /diff-fold", updated.textarea.Value())
	}
}

func TestDiffFoldWithLeadingTrailingSpaces(t *testing.T) {
	m, _ := newStubModel(nil)
	m.textarea.SetValue("  /diff-fold  15  ")

	updated, _ := m.submit()

	if updated.diffMaxLines != 15 {
		t.Fatalf("diffMaxLines = %d, want 15", updated.diffMaxLines)
	}
}

func TestRegularTextStillSubmitsNormally(t *testing.T) {
	m, _ := newStubModel([]string{"response"})
	m.textarea.SetValue("hello world")

	updated, cmd := m.submit()

	if cmd == nil {
		t.Fatal("regular text should start a turn (return command)")
	}
	if !updated.busy {
		t.Fatal("busy should be true after normal submit")
	}
	if updated.textarea.Value() != "" {
		t.Fatalf("textarea = %q, want cleared", updated.textarea.Value())
	}
	if len(updated.transcript) != 2 {
		t.Fatalf("transcript = %d, want 2 (user + assistant placeholder)", len(updated.transcript))
	}
}
