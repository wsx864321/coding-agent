package tui

import (
	"strings"
	"testing"
)

func layoutIndex(view, section string) int {
	return strings.Index(view, section)
}

func TestViewLayoutOrderMessageBeforeInputBeforeErrorBeforeStatus(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.messages = []Message{{Role: RoleUser, Content: "MSG_MARKER"}}
	m.input = "draft"
	m.lastError = "ERR_MARKER"
	m.statusMsg = "STAT_MARKER"

	view := m.View()
	msgIdx := layoutIndex(view, "MSG_MARKER")
	inputIdx := layoutIndex(view, "> draft")
	errIdx := layoutIndex(view, "ERR_MARKER")
	statIdx := layoutIndex(view, "STAT_MARKER")
	helpIdx := layoutIndex(view, "Enter")

	if msgIdx < 0 || inputIdx < 0 || errIdx < 0 || statIdx < 0 || helpIdx < 0 {
		t.Fatalf("missing section markers in view:\n%s", view)
	}
	if !(msgIdx < inputIdx && inputIdx < errIdx && errIdx < statIdx && statIdx < helpIdx) {
		t.Fatalf("layout order wrong (msg=%d input=%d err=%d stat=%d help=%d):\n%s",
			msgIdx, inputIdx, errIdx, statIdx, helpIdx, view)
	}
}

func TestViewBusyInputPaneShowsProcessingHint(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.busy = true
	m.input = "should-not-show"

	view := m.View()
	if strings.Contains(view, "> should-not-show") {
		t.Fatalf("busy view must not show draft input:\n%s", view)
	}
	if !strings.Contains(view, "处理中") {
		t.Fatalf("busy view must show processing hint:\n%s", view)
	}
}

func TestViewStatusAndErrorNotInMessagePane(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.messages = []Message{{Role: RoleAssistant, Content: "body-only"}}
	m.lastError = "network down"
	m.statusMsg = "已中断"

	lines := m.renderMessageLines()
	joined := strings.Join(lines, "\n")
	for _, forbidden := range []string{"network down", "已中断", "错误:"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("message pane polluted with %q:\n%s", forbidden, joined)
		}
	}
}
