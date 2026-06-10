package permission

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// StdinAsker 是 permission.Asker 的 stdin/stdout 交互实现
type StdinAsker struct {
	Reader io.Reader
	Writer io.Writer
	// AutoAnswer 非空时跳过交互，按 AutoAnswer 返回
	AutoAnswer string
}

// Ask 实现 permission.Asker
func (s *StdinAsker) Ask(_ context.Context, call ToolCall, reason string) bool {
	if s.AutoAnswer == "allow" {
		return true
	}
	if s.AutoAnswer == "deny" {
		return false
	}

	w := s.Writer
	if w == nil {
		w = os.Stderr
	}
	fmt.Fprintf(w, "\n\033[33m⚠  %s\033[0m\n", reason)
	fmt.Fprintf(w, "   Tool: %s(%s)\n", call.Name, formatArgs(call.Args))
	fmt.Fprintf(w, "   Allow? [y/N] ")

	r := s.Reader
	if r == nil {
		r = os.Stdin
	}
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return false
	}
	ans := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return ans == "y" || ans == "yes"
}

// formatArgs 紧凑地打印 args（json.Marshal 保证 key 顺序稳定）
func formatArgs(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	raw, _ := json.Marshal(args)
	s := string(raw)
	const max = 200
	if len(s) > max {
		s = s[:max] + "...(truncated)"
	}
	return s
}
