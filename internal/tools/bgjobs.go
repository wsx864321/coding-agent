package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/wsx864321/coding-agent/internal/jobs"
)

// bash_output / kill_shell / wait 操作由 bash(run_in_background) 和
// task(run_in_background) 注册的后台任务。它们通过 call context
// （jobs.FromContext）访问 session 的 job manager——agent 在每次工具调用
// 的 context 上印入——manager 不存在时降级为清晰错误（headless 测试、
// run loop 外的调用）。三者分别：轮询 job 新输出、终止 job、阻塞等待完成。

// --- bash_output: 轮询后台 job 的新输出（非阻塞）---

// BashOutputTool 读取后台任务自上次读取以来的增量输出及当前状态。
type BashOutputTool struct{}

func NewBashOutputTool() *BashOutputTool { return &BashOutputTool{} }

func (BashOutputTool) ReadOnly() bool { return true }

func (BashOutputTool) Name() string { return "bash_output" }

func (BashOutputTool) Description() string {
	return "读取后台任务（bash run_in_background=true 或 task run_in_background=true）自上次调用以来的新输出，" +
		"并返回其状态（running/done/failed/killed）。非阻塞。" +
		"适合分批查看长命令（install/build/test）的进度。"
}

func (BashOutputTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"job_id": map[string]any{
				"type":        "string",
				"description": "后台任务 ID（如 \"bash-1\"），由启动时返回。",
			},
			"filter": map[string]any{
				"type":        "string",
				"description": "可选正则表达式，只返回匹配的新输出行。",
			},
		},
		"required": []string{"job_id"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

func (BashOutputTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	var p struct {
		JobID  string `json:"job_id"`
		Filter string `json:"filter"`
	}
	if err := decodeArgs(args, &p); err != nil {
		return "", err
	}
	if strings.TrimSpace(p.JobID) == "" {
		return "", fmt.Errorf("job_id 不能为空")
	}
	jm, ok := jobs.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf("当前上下文不支持后台任务")
	}
	text, status, found := jm.OutputForSession(jobs.SessionFromContext(ctx), p.JobID)
	if !found {
		return "", fmt.Errorf("未找到后台任务 %q", p.JobID)
	}
	if p.Filter != "" && text != "" {
		filtered, err := filterLines(text, p.Filter)
		if err != nil {
			return "", err
		}
		text = filtered
	}
	header := fmt.Sprintf("[%s] %s", p.JobID, status)
	if strings.TrimSpace(text) == "" {
		return header + "\n(无新输出)", nil
	}
	return header + "\n" + text, nil
}

// filterLines 只保留 s 中匹配正则 re 的行。
func filterLines(s, re string) (string, error) {
	rx, err := regexp.Compile(re)
	if err != nil {
		return "", fmt.Errorf("无效的 filter 正则: %w", err)
	}
	var keep []string
	for _, line := range strings.Split(s, "\n") {
		if rx.MatchString(line) {
			keep = append(keep, line)
		}
	}
	return strings.Join(keep, "\n"), nil
}

// --- kill_shell: 终止运行中的后台任务 ---

// KillShellTool 终止一个后台任务（bash 或 task）。若已结束或 id 未知则为 no-op。
type KillShellTool struct{}

func NewKillShellTool() *KillShellTool { return &KillShellTool{} }

func (KillShellTool) ReadOnly() bool { return false }

func (KillShellTool) Name() string { return "kill_shell" }

func (KillShellTool) Description() string {
	return "终止一个运行中的后台任务（bash 或 task，由 run_in_background 启动）。" +
		"若任务已结束或 id 未知则为 no-op。"
}

func (KillShellTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"job_id": map[string]any{
				"type":        "string",
				"description": "要终止的后台任务 ID（如 \"bash-1\"）。",
			},
		},
		"required": []string{"job_id"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

func (KillShellTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	var p struct {
		JobID string `json:"job_id"`
	}
	if err := decodeArgs(args, &p); err != nil {
		return "", err
	}
	if strings.TrimSpace(p.JobID) == "" {
		return "", fmt.Errorf("job_id 不能为空")
	}
	jm, ok := jobs.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf("当前上下文不支持后台任务")
	}
	if jm.KillForSession(jobs.SessionFromContext(ctx), p.JobID) {
		return fmt.Sprintf("已终止后台任务 %q。", p.JobID), nil
	}
	return fmt.Sprintf("后台任务 %q 未在运行（已结束或未知）。", p.JobID), nil
}

// --- wait: 阻塞等待后台任务完成 ---

// WaitTool 阻塞直到后台任务完成，返回每个任务的状态与最终输出/回答。
type WaitTool struct{}

func NewWaitTool() *WaitTool { return &WaitTool{} }

func (WaitTool) ReadOnly() bool { return true }

func (WaitTool) Name() string { return "wait" }

func (WaitTool) Description() string {
	return "阻塞直到后台任务完成，返回每个任务的状态与最终输出/回答。" +
		"用于在继续前收集 task(run_in_background) 或 bash(run_in_background) 的结果。" +
		"省略 job_ids 则等待所有运行中的任务。"
}

func (WaitTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"job_ids": map[string]any{
				"type":        "array",
				"items":        map[string]any{"type": "string"},
				"description": "要等待的后台任务 ID 列表。省略则等待所有运行中的任务。",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"description": "可选的最大阻塞秒数，到期后返回当前进度。省略则等到任务完成。",
			},
		},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

func (WaitTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	var p struct {
		JobIDs         []string `json:"job_ids"`
		TimeoutSeconds int      `json:"timeout_seconds"`
	}
	// args 可能为 nil（省略所有参数）
	if args != nil {
		if err := decodeArgs(args, &p); err != nil {
			return "", err
		}
	}
	jm, ok := jobs.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf("当前上下文不支持后台任务")
	}
	results := jm.WaitForSession(ctx, jobs.SessionFromContext(ctx), p.JobIDs, p.TimeoutSeconds)
	if len(results) == 0 {
		return "没有可等待的后台任务。", nil
	}
	var b strings.Builder
	for i, r := range results {
		if i > 0 {
			b.WriteString("\n\n")
		}
		label := r.ID
		if r.Label != "" {
			label = fmt.Sprintf("%s (%s)", r.ID, r.Label)
		}
		fmt.Fprintf(&b, "[%s] %s", label, r.Status)
		if strings.TrimSpace(r.Output) != "" {
			b.WriteString("\n" + r.Output)
		}
	}
	return b.String(), nil
}
