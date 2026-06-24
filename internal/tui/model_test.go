package tui

import (
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
}
