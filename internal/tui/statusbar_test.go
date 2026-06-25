package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestRenderWorkingLineIdle(t *testing.T) {
	m := New()
	got := renderWorkingLine(m)
	if got != "" {
		t.Fatalf("renderWorkingLine(idle) = %q, want empty", got)
	}
}

func TestRenderWorkingLineBusy(t *testing.T) {
	m := New()
	m.busy = true
	m.runStart = time.Now().Add(-3 * time.Second)
	m.statusLabel = "running bash..."

	got := renderWorkingLine(m)
	for _, want := range []string{"running bash...", "3s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderWorkingLine(busy) = %q, missing %q", got, want)
		}
	}
}

func TestRenderWorkingLineDefaultLabel(t *testing.T) {
	m := New()
	m.busy = true
	m.runStart = time.Now()

	got := renderWorkingLine(m)
	if !strings.Contains(got, "thinking") {
		t.Fatalf("renderWorkingLine(busy) = %q, missing thinking label", got)
	}
}

func TestRenderModeLine(t *testing.T) {
	m := New()
	m.modelName = "deepseek-flash"
	m.gitStatus = gitStatus{branch: "main", ahead: 3}

	got := renderModeLine(m)
	for _, want := range []string{"Plan", "deepseek-flash", "main ↑3"} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderModeLine = %q, missing %q", got, want)
		}
	}
}

func TestRenderModeLineDirty(t *testing.T) {
	m := New()
	m.modelName = "deepseek-flash"
	m.gitStatus = gitStatus{branch: "main", dirty: true}

	got := renderModeLine(m)
	if !strings.Contains(got, "main *") {
		t.Fatalf("renderModeLine(dirty) = %q, missing dirty marker", got)
	}
}

func TestRenderDataLine(t *testing.T) {
	m := New()
	m.contextUsed = 45000
	m.contextWindow = 128000
	m.cacheHitRate = 85
	m.balance = "¥110.00"

	got := renderDataLine(m)
	for _, want := range []string{"ctx", "45K", "128K", "35%", "cache 85%", "¥110.00"} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderDataLine = %q, missing %q", got, want)
		}
	}
}

func TestRenderContextGauge(t *testing.T) {
	tests := []struct {
		used, window int
		want         string
	}{
		{0, 0, ""},
		{45000, 128000, "ctx 45K/128K (35%)"},
		{0, 128000, "ctx 0/128K (0%)"},
		{128000, 128000, "ctx 128K/128K (100%)"},
	}
	for _, tc := range tests {
		got := renderContextGauge(tc.used, tc.window)
		if got != tc.want {
			t.Fatalf("renderContextGauge(%d, %d) = %q, want %q", tc.used, tc.window, got, tc.want)
		}
	}
}

func TestGaugeColorThresholds(t *testing.T) {
	green := gaugeColor(30)
	yellow := gaugeColor(60)
	red := gaugeColor(85)

	// 验证三者返回不同样式
	gs := green.Render("x")
	ys := yellow.Render("x")
	rs := red.Render("x")

	if gs == ys || ys == rs || gs == rs {
		t.Fatalf("gaugeColor thresholds should produce distinct styles: green=%q yellow=%q red=%q", gs, ys, rs)
	}
}

func TestShortTokens(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{500, "500"},
		{1500, "1.5K"},
		{45000, "45K"},
		{128000, "128K"},
		{1500000, "1.5M"},
	}
	for _, tc := range tests {
		got := shortTokens(tc.n)
		if got != tc.want {
			t.Fatalf("shortTokens(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestRenderGitStatusStr(t *testing.T) {
	tests := []struct {
		gs   gitStatus
		want string
	}{
		{gitStatus{branch: "main"}, "main"},
		{gitStatus{branch: "main", dirty: true}, "main *"},
		{gitStatus{branch: "main", ahead: 3}, "main ↑3"},
		{gitStatus{branch: "feat/x", ahead: 2, behind: 1}, "feat/x ↑2 ↓1"},
	}
	for _, tc := range tests {
		got := renderGitStatusStr(tc.gs)
		if got != tc.want {
			t.Fatalf("renderGitStatusStr(%+v) = %q, want %q", tc.gs, got, tc.want)
		}
	}
}

func TestBottomHeightBase(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m = m.syncLayout()

	// 2 (mode + data) + 1 (help) + textarea
	want := 2 + 1 + m.textarea.Height()
	if got := m.bottomHeight(); got != want {
		t.Fatalf("bottomHeight() = %d, want %d", got, want)
	}
}

func TestBottomHeightBusy(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.busy = true
	m = m.syncLayout()

	// 1 (working) + 2 (mode + data) + 1 (help) + textarea
	want := 1 + 2 + 1 + m.textarea.Height()
	if got := m.bottomHeight(); got != want {
		t.Fatalf("bottomHeight(busy) = %d, want %d", got, want)
	}
}

func TestBottomHeightWithTodo(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.todoArgs = `[{"content":"task1","status":"pending"}]`
	m = m.syncLayout()

	// 2 (mode + data) + 1 (todo) + 1 (help) + textarea
	want := 2 + 1 + 1 + m.textarea.Height()
	if got := m.bottomHeight(); got != want {
		t.Fatalf("bottomHeight(todo) = %d, want %d", got, want)
	}
}

func TestBottomHeightWithApprovalAndError(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.approval = &pendingApproval{toolName: "write_file", respond: func(bool) {}}
	m.lastError = "network down"
	m = m.syncLayout()

	want := 2 + 1 + m.textarea.Height() + 2 + 1
	if got := m.bottomHeight(); got != want {
		t.Fatalf("bottomHeight() = %d, want %d", got, want)
	}
}

func TestViewThreePanelLayout(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.modelName = "coding-agent"
	m = m.syncLayout()
	m = m.syncViewportContent()

	view := viewContent(m)
	for _, want := range []string{
		"> ",
		"Plan",
		"coding-agent",
		"Shift+Enter",
		"Enter 发送",
		"Ctrl+C",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("View missing %q:\n%s", want, view)
		}
	}
}

func TestViewShowsApprovalBannerAboveStatusBar(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.approval = &pendingApproval{
		toolName: "write_file",
		args:     map[string]any{"path": "config.yaml"},
		respond:  func(bool) {},
	}
	m = m.syncLayout()

	view := viewContent(m)
	statusIdx := strings.Index(view, "Plan")
	bannerIdx := strings.Index(view, "Allow")
	if bannerIdx < 0 {
		t.Fatalf("View missing approval banner:\n%s", view)
	}
	if statusIdx < 0 {
		t.Fatalf("View missing status bar:\n%s", view)
	}
	if bannerIdx > statusIdx {
		t.Fatalf("approval banner should appear above status bar:\n%s", view)
	}
}

func TestViewBusyShowsWorkingLine(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.busy = true
	m.runStart = time.Now()
	m = m.syncLayout()

	view := viewContent(m)
	if !strings.Contains(view, "thinking") {
		t.Fatalf("View(busy) missing thinking:\n%s", view)
	}
}

func TestViewWithTodoPanel(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.todoArgs = `[{"content":"task1","status":"in_progress","activeForm":"testing"}]`
	m.todoItems = []todoItem{
		{Content: "task1", Status: "in_progress", ActiveForm: "testing"},
	}
	m = m.syncLayout()

	view := viewContent(m)
	if !strings.Contains(view, "task1") {
		t.Fatalf("View(todo) missing task:\n%s", view)
	}
}

func TestWindowSizeSetsViewportHeightFromBottom(t *testing.T) {
	m := New()
	m.textarea.SetValue("line1\nline2")
	m = m.syncLayout()

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated := next.(Model)

	bottom := updated.bottomHeight()
	wantVP := 30 - bottom
	if wantVP < 1 {
		wantVP = 1
	}
	if updated.viewport.Height() != wantVP {
		t.Fatalf("viewport height = %d, want %d (term=%d bottom=%d)", updated.viewport.Height(), wantVP, 30, bottom)
	}
}
