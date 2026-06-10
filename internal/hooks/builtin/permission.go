// Package builtin 提供 coding-agent 默认注册的内置 hook。
//
// 4 个 hook 各自负责一件事，组合起来覆盖"日志 / 权限 / 输出告警 / 收尾"四个关注点。
// 装配入口是 Default(workdir, asker, out)，会一次性把 4 个 hook 注入给定 Registry。
package builtin

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	openai "github.com/sashabaranov/go-openai"

	"github.com/wsx864321/coding-agent/internal/permission"
)

// =====================================================================
// 通用输出目标
// =====================================================================

// Sink 是各 hook 的可选输出目标；nil 时默认走 os.Stderr
//
// 之所以是独立类型而不是直接用 io.Writer，是为了未来支持彩色 / 标签前缀
// 等更复杂的输出策略时不影响调用方代码
type Sink struct {
	W io.Writer
}

// Fprintf 写入一行
func (s *Sink) Fprintf(format string, a ...any) {
	w := s.W
	if w == nil {
		w = os.Stderr
	}
	fmt.Fprintf(w, format, a...)
}

// =====================================================================
// PreToolUse: 权限检查 hook（替代循环内硬编码的 check_permission）
// =====================================================================

// PermissionHook 是 PreToolUse 阶段的内置权限检查 hook。
//
// 它本身不是"系统级硬约束"——系统级 Deny/Ask 仍由 agent.invokeTool 内的
// permission.Checker 负责；本 hook 处于更上层，承担"工作区边界 / 破坏性关键字"
// 这类需要 hooks 编排能力的检查。两层串联形成完整的权限防御：
//
//	hook（先放行 / 先阻断）
//	  → 仍要走 system permission.Checker（system deny 不可被 hook 覆盖）
//
// 实现要点：
//   - bash 工具的硬拒绝关键字：rm -rf /, sudo, shutdown, mkfs, dd if=
//   - bash 工具的破坏性关键字（命中后转 Ask）：rm, rmdir, > /etc/, chmod 777, del, rd /s, format
//   - 写入工具的越界检查：解析后路径必须在 workdir 内
//   - asker 为 nil 时，Ask 视为 Allow（与 permission.Pipeline 保持一致）
type PermissionHook struct {
	Workdir string
	Asker   permission.Asker
	Sink    *Sink
}

// NewPermissionHook 构造一个 PermissionHook
//
// workdir：写入工具越界检查的基准目录（空字符串 → 不启用越界规则）
// asker：Ask 阶段的用户裁决；nil 时 Ask 视为 Allow
func NewPermissionHook(workdir string, asker permission.Asker) *PermissionHook {
	return &PermissionHook{Workdir: workdir, Asker: asker}
}

// Handle 实现 hooks.PreToolUseHook
func (h *PermissionHook) Handle(_ context.Context, call permission.ToolCall) (string, string) {
	if call.Name == "bash" {
		if block, reason := h.checkBashDeny(call); block != "" {
			return block, reason
		}
		if block, reason := h.checkBashDestructive(call); block != "" {
			return block, reason
		}
	}
	if call.Name == "write_file" || call.Name == "edit_file" {
		if block, reason := h.checkWriteOutsideWorkdir(call); block != "" {
			return block, reason
		}
	}
	return "", ""
}

func (h *PermissionHook) checkBashDeny(call permission.ToolCall) (string, string) {
	cmd, _ := call.Args["command"].(string)
	if cmd == "" {
		return "", ""
	}
	denyKws := []string{
		"rm -rf /", "sudo", "shutdown", "reboot", "mkfs", "dd if=",
	}
	for _, kw := range denyKws {
		if strings.Contains(cmd, kw) {
			return "Permission denied: 硬拒绝关键字 '" + kw + "'", "命中硬拒绝列表"
		}
	}
	return "", ""
}

func (h *PermissionHook) checkBashDestructive(call permission.ToolCall) (string, string) {
	cmd, _ := call.Args["command"].(string)
	if cmd == "" {
		return "", ""
	}
	kws := []string{
		"rm ", "rmdir",
		"> /etc/", "chmod 777",
		"del ", "rd /s", "rd /q",
		"format ",
	}
	for _, kw := range kws {
		if strings.Contains(cmd, kw) {
			reason := "潜在破坏性 bash 命令（包含 '" + kw + "'）"
			if h.Asker == nil {
				// 无 Asker：放行（与 permission.Pipeline 行为对齐）
				return "", ""
			}
			if h.Asker.Ask(context.Background(), call, reason) {
				return "", "" // 用户批准
			}
			return "Permission denied: 用户拒绝执行 '" + kw + "'", reason
		}
	}
	return "", ""
}

func (h *PermissionHook) checkWriteOutsideWorkdir(call permission.ToolCall) (string, string) {
	if h.Workdir == "" {
		return "", ""
	}
	pathVal, _ := call.Args["path"].(string)
	if pathVal == "" {
		return "", ""
	}
	abs, err := filepath.Abs(pathVal)
	if err != nil {
		return "", ""
	}
	abs = filepath.Clean(abs)
	root, err := filepath.Abs(h.Workdir)
	if err != nil {
		return "", ""
	}
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		reason := "写入工作区外（无法解析相对路径）"
		if h.Asker == nil {
			return "", ""
		}
		if h.Asker.Ask(context.Background(), call, reason) {
			return "", ""
		}
		return "Permission denied: " + reason, reason
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		reason := "写入工作区外：" + pathVal
		if h.Asker == nil {
			return "", ""
		}
		if h.Asker.Ask(context.Background(), call, reason) {
			return "", ""
		}
		return "Permission denied: " + reason, reason
	}
	return "", ""
}

// =====================================================================
// PreToolUse: 日志 hook
// =====================================================================

// LogHook 打印每次工具调用的摘要（用于 debug / 审计）
type LogHook struct {
	Sink *Sink
}

// NewLogHook 构造一个 LogHook
func NewLogHook() *LogHook {
	return &LogHook{}
}

// Handle 实现 hooks.PreToolUseHook（仅打日志，不阻断）
func (h *LogHook) Handle(_ context.Context, call permission.ToolCall) (string, string) {
	preview := argPreview(call.Args, 60)
	h.sink().Fprintf("[HOOK] %s(%s)\n", call.Name, preview)
	return "", ""
}

func (h *LogHook) sink() *Sink {
	if h.Sink != nil {
		return h.Sink
	}
	return &Sink{}
}

// =====================================================================
// PostToolUse: 大输出告警 hook
// =====================================================================

// LargeOutputThreshold 默认 50KB
const LargeOutputThreshold = 50000

// LargeOutputHook 在工具输出过大时打警告
type LargeOutputHook struct {
	Threshold int
	Sink      *Sink
}

// NewLargeOutputHook 构造一个 LargeOutputHook
func NewLargeOutputHook() *LargeOutputHook {
	return &LargeOutputHook{Threshold: LargeOutputThreshold}
}

// Handle 实现 hooks.PostToolUseHook
func (h *LargeOutputHook) Handle(_ context.Context, call permission.ToolCall, output string) {
	threshold := h.Threshold
	if threshold <= 0 {
		threshold = LargeOutputThreshold
	}
	if len(output) > threshold {
		h.sink().Fprintf("[HOOK] ⚠ Large output from %s: %d chars (threshold=%d)\n",
			call.Name, len(output), threshold)
	}
}

func (h *LargeOutputHook) sink() *Sink {
	if h.Sink != nil {
		return h.Sink
	}
	return &Sink{}
}

// =====================================================================
// UserPromptSubmit: 上下文注入 hook
// =====================================================================

// ContextInjectHook 在 user 消息追加前打印 workdir 等上下文
type ContextInjectHook struct {
	Workdir string
	Sink    *Sink
}

// NewContextInjectHook 构造一个 ContextInjectHook
func NewContextInjectHook(workdir string) *ContextInjectHook {
	return &ContextInjectHook{Workdir: workdir}
}

// Handle 实现 hooks.UserPromptSubmitHook
func (h *ContextInjectHook) Handle(_ context.Context, _ string) error {
	h.sink().Fprintf("[HOOK] UserPromptSubmit: workdir=%s\n", h.Workdir)
	return nil
}

func (h *ContextInjectHook) sink() *Sink {
	if h.Sink != nil {
		return h.Sink
	}
	return &Sink{}
}

// =====================================================================
// Stop: 收尾 hook（统计工具调用次数）
// =====================================================================

// SummaryHook 在主循环结束时打印本次会话的工具调用统计
type SummaryHook struct {
	Sink *Sink
}

// NewSummaryHook 构造一个 SummaryHook
func NewSummaryHook() *SummaryHook {
	return &SummaryHook{}
}

// Handle 实现 hooks.StopHook
//
// 统计 messages 中 Role=tool 的条数；不阻断（不返回 force）
func (h *SummaryHook) Handle(_ context.Context, messages []openai.ChatCompletionMessage) (string, bool) {
	count := 0
	for _, m := range messages {
		if m.Role == openai.ChatMessageRoleTool {
			count++
		}
	}
	h.sink().Fprintf("[HOOK] Stop: session used %d tool calls\n", count)
	return "", false
}

func (h *SummaryHook) sink() *Sink {
	if h.Sink != nil {
		return h.Sink
	}
	return &Sink{}
}

// =====================================================================
// 内部辅助
// =====================================================================

// argPreview 把 args 拍平成 "<key1>=<val1>, <key2>=<val2>" 形式并截断
func argPreview(args map[string]any, max int) string {
	if len(args) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(args))
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	s := strings.Join(parts, ", ")
	if len(s) > max {
		s = s[:max] + "..."
	}
	return s
}
