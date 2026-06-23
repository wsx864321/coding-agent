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
		m = applyKey(m, tea.KeyPressMsg{Code: r})
	}
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
	if len(m.messages) != 0 {
		t.Fatalf("messages = %d, want 0", len(m.messages))
	}
	if m.input != "" {
		t.Fatalf("input = %q, want empty", m.input)
	}
	if m.scrollOffset != 0 {
		t.Fatalf("scrollOffset = %d, want 0", m.scrollOffset)
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
	if got := updated.input; got != "hi" {
		t.Fatalf("input = %q, want %q", got, "hi")
	}
}

func TestInsertInputAccumulates(t *testing.T) {
	m := New()
	m.input = "hel"
	updated := typeText(m, "lo")
	if got := updated.input; got != "hello" {
		t.Fatalf("input = %q, want %q", got, "hello")
	}
}

func TestBackspaceRemovesLastRune(t *testing.T) {
	m := New()
	m.input = "hello"
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if cmd != nil {
		t.Fatal("backspace should not return a command")
	}
	updated := next.(Model)
	if got := updated.input; got != "hell" {
		t.Fatalf("input = %q, want %q", got, "hell")
	}
}

func TestBackspaceOnEmptyInputIsNoop(t *testing.T) {
	m := New()
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	updated := next.(Model)
	if updated.input != "" {
		t.Fatalf("input = %q, want empty", updated.input)
	}
}

func TestAppendMessageStoresRoleAndContent(t *testing.T) {
	m := New()
	m = m.withMessage(RoleUser, "hello")
	m = m.withMessage(RoleAssistant, "world")
	m = m.withMessage(RoleSystem, "notice")

	if len(m.messages) != 3 {
		t.Fatalf("messages = %d, want 3", len(m.messages))
	}
	if m.messages[0].Role != RoleUser || m.messages[0].Content != "hello" {
		t.Fatalf("messages[0] = %+v, want user/hello", m.messages[0])
	}
	if m.messages[1].Role != RoleAssistant || m.messages[1].Content != "world" {
		t.Fatalf("messages[1] = %+v, want assistant/world", m.messages[1])
	}
	if m.messages[2].Role != RoleSystem || m.messages[2].Content != "notice" {
		t.Fatalf("messages[2] = %+v, want system/notice", m.messages[2])
	}
}

func TestScrollUpMovesAwayFromBottom(t *testing.T) {
	m := New()
	m.width = 40
	m.height = 12
	m.messages = longMessageHistory()
	m = m.clampScrollToBottom()

	before := m.scrollOffset
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if cmd != nil {
		t.Fatal("scroll up should not return a command")
	}
	updated := next.(Model)
	if updated.scrollOffset >= before {
		t.Fatalf("scrollOffset = %d, want < %d after KeyUp", updated.scrollOffset, before)
	}
}

func TestScrollDownIncreasesOffsetTowardBottom(t *testing.T) {
	m := New()
	m.width = 40
	m.height = 12
	m.messages = longMessageHistory()
	m = m.clampScrollToBottom()
	bottom := m.scrollOffset

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	scrolledUp := next.(Model)
	if scrolledUp.scrollOffset >= bottom {
		t.Fatalf("expected scroll up to move away from bottom, got offset %d (bottom=%d)", scrolledUp.scrollOffset, bottom)
	}

	next, _ = scrolledUp.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	scrolledDown := next.(Model)
	if scrolledDown.scrollOffset <= scrolledUp.scrollOffset {
		t.Fatalf("scrollOffset = %d, want > %d after KeyDown", scrolledDown.scrollOffset, scrolledUp.scrollOffset)
	}
}

func TestScrollUpStopsAtTop(t *testing.T) {
	m := New()
	m.width = 40
	m.height = 12
	m.messages = longMessageHistory()
	m.scrollOffset = 0

	for i := 0; i < 20; i++ {
		next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
		m = next.(Model)
	}
	if m.scrollOffset != 0 {
		t.Fatalf("scrollOffset = %d, want 0 at top boundary", m.scrollOffset)
	}
}

func TestScrollDownStopsAtBottom(t *testing.T) {
	m := New()
	m.width = 40
	m.height = 12
	m.messages = longMessageHistory()
	m = m.clampScrollToBottom()
	bottom := m.scrollOffset

	for i := 0; i < 20; i++ {
		next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m = next.(Model)
	}
	if m.scrollOffset != bottom {
		t.Fatalf("scrollOffset = %d, want %d at bottom boundary", m.scrollOffset, bottom)
	}
}

func TestViewRendersMessageInputAndHelpAreas(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.messages = []Message{
		{Role: RoleUser, Content: "你好"},
		{Role: RoleAssistant, Content: "你好，我是助手"},
		{Role: RoleSystem, Content: "系统提示"},
	}
	m.input = "draft text"

	view := viewContent(m)
	for _, want := range []string{
		"coding-agent",
		"user:",
		"你好",
		"assistant:",
		"你好，我是助手",
		"system:",
		"系统提示",
		"> draft text",
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
	m.messages = []Message{
		{Role: RoleUser, Content: "first-visible-marker"},
		{Role: RoleUser, Content: "line-2"},
		{Role: RoleUser, Content: "line-3"},
		{Role: RoleUser, Content: "line-4"},
		{Role: RoleUser, Content: "line-5"},
		{Role: RoleUser, Content: "line-6"},
		{Role: RoleUser, Content: "line-7"},
		{Role: RoleUser, Content: "line-8"},
		{Role: RoleUser, Content: "last-visible-marker"},
	}
	m = m.clampScrollToBottom()

	bottomView := viewContent(m)
	if !strings.Contains(bottomView, "last-visible-marker") {
		t.Fatalf("bottom view should show latest message:\n%s", bottomView)
	}

	m.scrollOffset = 0
	topView := viewContent(m)
	if strings.Contains(topView, "last-visible-marker") {
		t.Fatalf("top view should hide latest message when scrolled up:\n%s", topView)
	}
	if !strings.Contains(topView, "first-visible-marker") {
		t.Fatalf("top view should show earliest message:\n%s", topView)
	}
}

func longMessageHistory() []Message {
	msgs := make([]Message, 0, 20)
	for i := 1; i <= 20; i++ {
		msgs = append(msgs, Message{Role: RoleUser, Content: strings.Repeat("x", i*3)})
	}
	return msgs
}
