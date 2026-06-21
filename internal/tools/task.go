package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/wsx864321/coding-agent/internal/jobs"
)

// SubagentRunner 是 task 工具调用 subagent 的抽象接口。
//
// 由 agent 包实现并注入，避免 tools → agent 的循环依赖。
// prompt 是子 agent 的唯一输入，返回子 agent 的最终回答。
type SubagentRunner func(ctx context.Context, prompt string) (string, error)

// TaskTool 派生一个子 agent 执行聚焦的子任务。
//
// 子 agent 在独立的 session 中运行（全新 messages 历史），只有最终回答
// 返回给父 agent。用于：
//   - 把冗长的探索过程隔离在子 agent 中，不污染父 agent 上下文
//   - 委派自包含的工作（如「找出所有调用 Foo 的地方并总结规律」）
//
// 设计参考：
//   - Reasonix: task 工具 + FilterRegistry + SubagentMetaTools
//   - Claude Code: Agent 工具 + subagent_type
//   - learn-claude-code s06: spawn_subagent() 教学版
type TaskTool struct {
	runner SubagentRunner
}

// NewTaskTool 创建 TaskTool
//
// runner 可以为 nil，后续通过 SetRunner 注入（agent 构造完成后再连线）
func NewTaskTool(runner SubagentRunner) *TaskTool {
	return &TaskTool{runner: runner}
}

// SetRunner 注入 SubagentRunner（延迟连线，解决构造顺序问题）
func (t *TaskTool) SetRunner(runner SubagentRunner) {
	t.runner = runner
}

func (t *TaskTool) ReadOnly() bool { return false }

func (t *TaskTool) Name() string { return "task" }

func (t *TaskTool) Description() string {
	return "派生一个子 agent 执行聚焦的子任务。子 agent 在独立的 session 中运行，" +
		"拥有与父 agent 相同的工具集（不含 task/todo_write/complete_step），" +
		"只有最终回答返回。适用于探索性工作（如搜索调用链、分析代码模式）" +
		"或自包含的修改任务，避免中间过程污染父 agent 上下文。"
}

type taskArgs struct {
	Prompt          string `json:"prompt"`
	Description     string `json:"description,omitempty"`
	RunInBackground bool   `json:"run_in_background,omitempty"`
}

func (t *TaskTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "子 agent 需要完成的任务。描述要具体，包含明确的交付物——子 agent 看不到父 agent 的对话上下文。",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "子任务的简短标签（3-7 个词），用于日志展示。",
			},
			"run_in_background": map[string]any{
				"type":        "boolean",
				"description": "后台执行：立即返回 job id，子 agent 跨 turn 持续运行。用 bash_output(job_id=...) 读取最终回答，wait 等待完成，kill_shell 终止。适合耗时的子任务。",
			},
		},
		"required": []string{"prompt"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

func (t *TaskTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	var p taskArgs
	if err := decodeArgs(args, &p); err != nil {
		return "", err
	}

	if strings.TrimSpace(p.Prompt) == "" {
		return "", fmt.Errorf("prompt 不能为空")
	}

	if t.runner == nil {
		return "", fmt.Errorf("subagent runner 未配置")
	}

	// 后台执行分支：通过 jobs.Manager 启动，立即返回 job id
	if p.RunInBackground {
		return t.runBackground(ctx, p)
	}

	answer, err := t.runner(ctx, p.Prompt)
	if err != nil {
		return "", fmt.Errorf("子 agent 执行失败: %w", err)
	}

	return answer, nil
}

// runBackground 在后台 goroutine 中运行子 agent，立即返回 job id。
// 子 agent 的最终回答作为 job 的 result，模型用 bash_output 读取。
func (t *TaskTool) runBackground(ctx context.Context, p taskArgs) (string, error) {
	jm, ok := jobs.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf("后台执行不可用：当前上下文未配置 jobs.Manager")
	}

	label := p.Description
	if label == "" {
		label = "task"
	}
	sessionID := jobs.SessionFromContext(ctx)
	runner := t.runner

	job := jm.StartForSession(sessionID, "task", label, func(jobCtx context.Context, out io.Writer) (string, error) {
		answer, err := runner(jobCtx, p.Prompt)
		if err != nil {
			return "", fmt.Errorf("子 agent 执行失败: %w", err)
		}
		return answer, nil
	})

	return fmt.Sprintf("已启动后台子任务 %q。它跨 turn 持续运行；用 bash_output(job_id=%q) 读取最终回答，wait 等待完成，kill_shell(job_id=%q) 终止。",
		job.ID, job.ID, job.ID), nil
}
