package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestCompletionStructEmpty(t *testing.T) {
	c := completion{}
	if c.active {
		t.Fatal("new completion should not be active")
	}
	if c.selected != 0 {
		t.Fatalf("selected = %d, want 0", c.selected)
	}
	if len(c.items) != 0 {
		t.Fatalf("items = %v, want empty", c.items)
	}
}

func TestFilterCommandsEmpty(t *testing.T) {
	cmds := []string{"/help", "/skills", "/model", "/clear", "/diff-fold"}
	result := filterCommands(cmds, "")
	if len(result) != len(cmds) {
		t.Fatalf("filter with empty prefix: got %d items, want %d", len(result), len(cmds))
	}
}

func TestFilterCommandsPrefix(t *testing.T) {
	cmds := []string{"/help", "/skills", "/model", "/clear", "/diff-fold"}
	result := filterCommands(cmds, "/s")
	if len(result) != 1 {
		t.Fatalf("filter /s: got %d items, want 1", len(result))
	}
	if result[0] != "/skills" {
		t.Fatalf("filter /s: got %q, want /skills", result[0])
	}
}

func TestFilterCommandsNoMatch(t *testing.T) {
	cmds := []string{"/help", "/skills"}
	result := filterCommands(cmds, "/x")
	if len(result) != 0 {
		t.Fatalf("filter /x: got %d items, want 0", len(result))
	}
}

func TestRenderCompletionActive(t *testing.T) {
	c := completion{
		active:   true,
		items:    []string{"/help", "/skills", "/model"},
		selected: 1,
	}
	out := renderCompletion(c, 40)
	if out == "" {
		t.Fatal("renderCompletion with active=true should produce output")
	}
	if !strings.Contains(out, "/help") {
		t.Fatal("renderCompletion should contain /help")
	}
	if !strings.Contains(out, "/skills") {
		t.Fatal("renderCompletion should contain /skills")
	}
	if !strings.Contains(out, "/model") {
		t.Fatal("renderCompletion should contain /model")
	}
}

func TestRenderCompletionInactive(t *testing.T) {
	c := completion{
		active: false,
		items:  []string{"/help", "/skills"},
	}
	out := renderCompletion(c, 40)
	if out != "" {
		t.Fatalf("renderCompletion with active=false: got %q, want empty", out)
	}
}

func TestRenderCompletionEmptyItems(t *testing.T) {
	c := completion{
		active: true,
		items:  nil,
	}
	out := renderCompletion(c, 40)
	if out != "" {
		t.Fatalf("renderCompletion with no items: got %q, want empty", out)
	}
}

func TestCheckSlashCompletion(t *testing.T) {
	m := New()
	m.slashCommands = []string{"/help", "/skills", "/model", "/clear", "/diff-fold"}

	// 输入 / 时激活补全
	m.textarea.SetValue("/")
	m = m.checkSlashCompletion()
	if !m.completion.active {
		t.Fatal("/ should activate completion")
	}
	if len(m.completion.items) != 5 {
		t.Fatalf("/: got %d items, want 5", len(m.completion.items))
	}

	// 输入 /s 时过滤
	m.textarea.SetValue("/s")
	m = m.checkSlashCompletion()
	if !m.completion.active {
		t.Fatal("/s should keep completion active")
	}
	if len(m.completion.items) != 1 || m.completion.items[0] != "/skills" {
		t.Fatalf("/s: got %v, want [/skills]", m.completion.items)
	}

	// 输入 /x 无匹配时关闭
	m.textarea.SetValue("/x")
	m = m.checkSlashCompletion()
	if m.completion.active {
		t.Fatal("/x should deactivate completion (no matches)")
	}

	// 带空格输入不激活补全
	m.textarea.SetValue("/help me")
	m = m.checkSlashCompletion()
	if m.completion.active {
		t.Fatal("'/help me' should not activate completion (contains space)")
	}

	// 非 / 开头不激活
	m.textarea.SetValue("hello")
	m = m.checkSlashCompletion()
	if m.completion.active {
		t.Fatal("'hello' should not activate completion")
	}
}

func TestCompletionUpDown(t *testing.T) {
	m := New()
	m.slashCommands = []string{"/help", "/skills", "/model"}
	m.textarea.SetValue("/")
	m = m.checkSlashCompletion()

	// 按 ↓ 导航
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m2 := next.(Model)
	if m2.completion.selected != 1 {
		t.Fatalf("after down: selected = %d, want 1", m2.completion.selected)
	}

	// 按 ↑ 导航
	next, _ = m2.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m3 := next.(Model)
	if m3.completion.selected != 0 {
		t.Fatalf("after up: selected = %d, want 0", m3.completion.selected)
	}

	// ↑ 在顶部时不越界
	next, _ = m3.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m4 := next.(Model)
	if m4.completion.selected != 0 {
		t.Fatalf("up at top: selected = %d, want 0", m4.completion.selected)
	}
}

func TestCompletionEnterAccept(t *testing.T) {
	m := New()
	m.slashCommands = []string{"/help", "/skills"}
	m.textarea.SetValue("/h")
	m = m.checkSlashCompletion()

	if !m.completion.active {
		t.Fatal("completion should be active for /h")
	}
	if m.completion.items[0] != "/help" {
		t.Fatalf("first item: got %q, want /help", m.completion.items[0])
	}

	// 按 Enter 接受补全
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m2 := next.(Model)
	if m2.completion.active {
		t.Fatal("enter should deactivate completion")
	}
	if !strings.HasPrefix(m2.textarea.Value(), "/help") {
		t.Fatalf("textarea after enter: got %q, want prefix /help", m2.textarea.Value())
	}
}

func TestCompletionEscClose(t *testing.T) {
	m := New()
	m.slashCommands = []string{"/help"}
	m.textarea.SetValue("/")
	m = m.checkSlashCompletion()

	if !m.completion.active {
		t.Fatal("completion should be active for /")
	}

	// 按 Esc 关闭
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m2 := next.(Model)
	if m2.completion.active {
		t.Fatal("esc should deactivate completion")
	}
}

func TestCompletionTabAccept(t *testing.T) {
	m := New()
	m.slashCommands = []string{"/help", "/skills"}
	m.textarea.SetValue("/he")
	m = m.checkSlashCompletion()

	// 按 Tab 接受补全
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m2 := next.(Model)
	if m2.completion.active {
		t.Fatal("tab should deactivate completion")
	}
	if !strings.HasPrefix(m2.textarea.Value(), "/help") {
		t.Fatalf("textarea after tab: got %q, want prefix /help", m2.textarea.Value())
	}
}
