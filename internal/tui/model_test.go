package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
