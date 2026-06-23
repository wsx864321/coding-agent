package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

const approvalArgMaxLen = 200

var sensitiveArgKeys = []string{"password", "token", "secret", "api_key", "apikey", "auth"}

// pendingApproval 表示等待用户 y/n 响应的审批模态状态。
type pendingApproval struct {
	toolName string
	args     map[string]any
	respond  func(bool)
}

func (m Model) handleApprovalKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.approval == nil {
		return m, nil
	}
	switch msg.String() {
	case "y", "Y":
		m.approval.respond(true)
		m.approval = nil
	case "n", "N":
		m.approval.respond(false)
		m.approval = nil
	}
	return m, nil
}

func renderApprovalBanner(a pendingApproval, _ int) string {
	label := toolDisplayName(a.toolName)
	summary := approvalArgSummary(a.toolName, a.args)
	if summary != "" {
		return fmt.Sprintf(`Allow %s("%s")? [y]es [n]o`, label, summary)
	}
	return fmt.Sprintf(`Allow %s? [y]es [n]o`, label)
}

func approvalArgSummary(name string, args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	raw, err := json.Marshal(redactSensitiveArgs(args))
	if err != nil {
		return ""
	}
	summary := toolArg(name, string(raw))
	if len(summary) > approvalArgMaxLen {
		return summary[:approvalArgMaxLen] + "..."
	}
	return summary
}

func redactSensitiveArgs(args map[string]any) map[string]any {
	out := make(map[string]any, len(args))
	for k, v := range args {
		if isSensitiveArgKey(k) {
			out[k] = "***"
		} else {
			out[k] = v
		}
	}
	return out
}

func isSensitiveArgKey(key string) bool {
	lower := strings.ToLower(key)
	for _, sensitive := range sensitiveArgKeys {
		if lower == sensitive {
			return true
		}
	}
	return false
}
