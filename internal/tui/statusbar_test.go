package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestBottomHeightBase(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m = m.syncLayout()

	want := 1 + 1 + m.textarea.Height() // status + help + textarea
	if got := m.bottomHeight(); got != want {
		t.Fatalf("bottomHeight() = %d, want %d", got, want)
	}
}

func TestBottomHeightWithApprovalAndError(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.approval = &pendingApproval{toolName: "write_file", respond: func(bool) {}}
	m.lastError = "network down"
	m = m.syncLayout()

	want := 1 + 1 + m.textarea.Height() + 2 + 1
	if got := m.bottomHeight(); got != want {
		t.Fatalf("bottomHeight() = %d, want %d", got, want)
	}
}

func TestRenderStatusBarIdle(t *testing.T) {
	m := New()
	m.modelName = "deepseek-v3"
	got := renderStatusBar(m)
	if got != "deepseek-v3 │ idle" {
		t.Fatalf("renderStatusBar(idle) = %q, want %q", got, "deepseek-v3 │ idle")
	}
}

func TestRenderStatusBarBusy(t *testing.T) {
	m := New()
	m.busy = true
	m.runStart = time.Now().Add(-3 * time.Second)
	m.statusLabel = "running bash..."

	got := renderStatusBar(m)
	for _, want := range []string{"running bash...", "(3s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderStatusBar(busy) = %q, missing %q", got, want)
		}
	}
}

func TestRenderStatusBarBusyDefaultLabel(t *testing.T) {
	m := New()
	m.busy = true
	m.runStart = time.Now()

	got := renderStatusBar(m)
	if !strings.Contains(got, "thinking") {
		t.Fatalf("renderStatusBar(busy) = %q, want default thinking label", got)
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
	if updated.width != 100 || updated.height != 30 {
		t.Fatalf("size = %dx%d, want 100x30", updated.width, updated.height)
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
		"coding-agent │ idle",
		"Shift+Enter",
		"Enter 发送",
		"Ctrl+C",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("View missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "↑↓/jk") {
		t.Fatalf("View should not mention j/k scroll help:\n%s", view)
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
	statusIdx := strings.Index(view, "coding-agent │ idle")
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
