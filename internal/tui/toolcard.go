package tui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"
)

const toolOutputCollapseLines = 8

const toolCardRawSep = "\x1e"

const toolCardErrorMarker = "!"

const connector = "  ⎿  "

var (
	toolReadStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("80"))
	toolWriteStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	toolExecStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	toolProcStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("176"))
	toolErrorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	toolDefaultDot = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	toolNameStyle  = lipgloss.NewStyle().Bold(true)
	toolArgStyle   = lipgloss.NewStyle().Faint(true)
	toolGutterStyle = lipgloss.NewStyle().Faint(true)
)

var toolVerb = map[string]string{
	"bash":        "Bash",
	"read_file":   "Read",
	"write_file":  "Write",
	"edit_file":   "Update",
	"glob_file":   "Glob",
	"glob":        "Glob",
	"grep":        "Search",
	"ls":          "List",
	"bash_output": "Output",
	"kill_shell":  "Kill",
	"wait":        "Wait",
	"task":        "Task",
}

var toolArgKey = map[string]string{
	"bash":        "command",
	"read_file":   "path",
	"write_file":  "path",
	"edit_file":   "path",
	"glob_file":   "pattern",
	"glob":        "pattern",
	"grep":        "pattern",
	"ls":          "path",
	"bash_output": "job_id",
	"kill_shell":  "job_id",
	"task":        "description",
}

var toolCategory = map[string]string{
	"read_file": "read", "glob_file": "read", "glob": "read", "grep": "read", "ls": "read",
	"bash_output": "read",
	"write_file": "write", "edit_file": "write",
	"bash": "exec",
	"wait": "proc", "kill_shell": "proc",
}

func renderToolCard(name, args string, width int) string {
	label := toolDisplayName(name)
	dot := toolDot(name, false)
	head := toolNameStyle.Render(label)
	if arg := toolArg(name, args); arg != "" {
		avail := width - 4 - runewidth.StringWidth(label) - 2
		if avail < 4 {
			avail = 4
		}
		head += toolArgStyle.Render("(" + clampPlain(arg, avail) + ")")
	}
	return "  " + dot + " " + head
}

func renderToolCardError(name, errMsg string, width int) string {
	dot := toolDot(name, true)
	label := toolDisplayName(name)
	msg := clampPlain(errMsg, width-6-runewidth.StringWidth(label))
	return "  " + dot + " " + toolNameStyle.Render(label) + " ⊘ " + toolErrorStyle.Render(msg)
}

func renderToolOutput(result string, maxLines int) string {
	lines := splitLines(result)
	if len(lines) == 0 {
		return ""
	}
	if len(lines) <= maxLines {
		return connectorBlock(lines)
	}
	visible := lines[:maxLines]
	summary := fmt.Sprintf("%s(%d lines, collapsed)", connector, len(lines))
	return connectorBlock(visible) + "\n" + toolGutterStyle.Render(summary)
}

func connectorBlock(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	indent := strings.Repeat(" ", runewidth.StringWidth(connector))
	out := toolGutterStyle.Render(connector) + lines[0]
	for _, ln := range lines[1:] {
		out += "\n" + indent + ln
	}
	return out
}

func toolDot(name string, isError bool) string {
	if isError {
		return toolErrorStyle.Render("●")
	}
	switch toolCategory[name] {
	case "read":
		return toolReadStyle.Render("●")
	case "write":
		return toolWriteStyle.Render("●")
	case "exec":
		return toolExecStyle.Render("●")
	case "proc":
		return toolProcStyle.Render("●")
	default:
		return toolDefaultDot.Render("●")
	}
}

func toolDisplayName(name string) string {
	if v, ok := toolVerb[name]; ok {
		return v
	}
	if strings.HasPrefix(name, "mcp__") {
		parts := strings.Split(name, "__")
		if len(parts) >= 3 {
			return parts[len(parts)-1]
		}
	}
	return name
}

func toolArg(name, args string) string {
	var m map[string]any
	if json.Unmarshal([]byte(args), &m) != nil {
		if strings.TrimSpace(args) != "" {
			return clampPlain(strings.TrimSpace(args), 60)
		}
		return ""
	}
	key, ok := toolArgKey[name]
	if !ok {
		for _, k := range []string{"path", "command", "pattern", "query", "url", "name"} {
			if v, exists := m[k]; exists {
				return formatArgValue(v)
			}
		}
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	return formatArgValue(v)
}

func formatArgValue(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strconv.Itoa(int(x))
	case []any:
		parts := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	default:
		return ""
	}
}

func clampPlain(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= maxWidth {
		return s
	}
	ellipsis := "…"
	target := maxWidth - runewidth.StringWidth(ellipsis)
	if target < 1 {
		return ellipsis
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if w+rw > target {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String() + ellipsis
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func encodeToolCardRaw(name, args string, isError bool) string {
	prefix := ""
	if isError {
		prefix = toolCardErrorMarker
	}
	return prefix + name + toolCardRawSep + args
}

func decodeToolCardRaw(raw string) (name, args string, isError bool) {
	if strings.HasPrefix(raw, toolCardErrorMarker) {
		isError = true
		raw = raw[len(toolCardErrorMarker):]
	}
	parts := strings.SplitN(raw, toolCardRawSep, 2)
	name = parts[0]
	if len(parts) > 1 {
		args = parts[1]
	}
	return name, args, isError
}

const toolOutputRawSep = "\x1e"

// encodeToolOutputRaw encodes a toolCallID and output string into a single
// Raw value. The format is toolCallID + \x1e + output.
func encodeToolOutputRaw(toolCallID, output string) string {
	return toolCallID + toolOutputRawSep + output
}

// decodeToolOutputRaw decodes a Raw value back into toolCallID and output.
// If the raw does not contain the separator, it is treated as a legacy
// plain-output entry and the entire string is returned as output.
func decodeToolOutputRaw(raw string) (toolCallID, output string) {
	parts := strings.SplitN(raw, toolOutputRawSep, 2)
	if len(parts) < 2 {
		return "", raw
	}
	return parts[0], parts[1]
}

// isDiffOutput reports whether a tool output string looks like a unified diff.
// It requires at least one hunk header (@@ ... @@) and at least one line
// starting with '+' or '-' to avoid false positives on plain text.
func isDiffOutput(output string) bool {
	if output == "" {
		return false
	}
	hasHunk := false
	hasChange := false
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "@@") {
			hasHunk = true
		}
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			hasChange = true
		}
		if hasHunk && hasChange {
			return true
		}
	}
	return false
}

// renderDiffOutput renders diff output with optional collapse at maxLines.
// When maxLines is 0, all lines are shown without collapse.
// When maxLines > 0 and the diff has more lines, it shows the first maxLines
// lines and appends a collapsed summary.
func renderDiffOutput(result string, maxLines int) string {
	lines := splitLines(result)
	if len(lines) == 0 {
		return ""
	}
	if maxLines <= 0 || len(lines) <= maxLines {
		return connectorBlock(lines)
	}
	visible := lines[:maxLines]
	summary := fmt.Sprintf("%sDiff: %d lines (%d visible, collapsed)", connector, len(lines), maxLines)
	return connectorBlock(visible) + "\n" + toolGutterStyle.Render(summary)
}
