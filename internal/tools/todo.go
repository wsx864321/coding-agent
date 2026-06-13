package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wsx864321/coding-agent/internal/evidence"
)

// TodoWriteTool 是规划工具，让 Agent 在动手之前先列出步骤并跟踪进度。
//
// 设计要点（对标 Reasonix todo_write）：
//   - 全量替换模型：每次调用发送完整列表，覆盖上一次
//   - 3 种状态：pending / in_progress / completed
//   - activeForm 字段：进行时态描述，用于日志展示
//   - 证据耦合：标记 completed 前必须有 complete_step 凭证（依赖 evidence.Ledger）
//   - 本身不执行任何操作，只提供规划能力
type TodoWriteTool struct{}

// NewTodoWriteTool 创建 TodoWriteTool
func NewTodoWriteTool() *TodoWriteTool {
	return &TodoWriteTool{}
}

func (t *TodoWriteTool) ReadOnly() bool { return false }

func (t *TodoWriteTool) Name() string { return "todo_write" }

func (t *TodoWriteTool) Description() string {
	return "创建和管理当前任务的执行清单。每次调用发送完整列表（全量替换）。" +
		"对于多步骤任务，先用此工具列出所有步骤（pending），将第一步设为 in_progress。" +
		"每次只保持一个 in_progress。完成一步后先调 complete_step 提供证据，再调此工具更新状态。" +
		"简单任务（少于 3 步）无需使用此工具。"
}

// todoWriteArgs 是 todo_write 的入参
type todoWriteArgs struct {
	Todos []todoItemArg `json:"todos"`
}

type todoItemArg struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"activeForm,omitempty"`
}

var validStatuses = map[string]bool{
	"pending":     true,
	"in_progress": true,
	"completed":   true,
}

func (t *TodoWriteTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"todos": map[string]any{
				"type":        "array",
				"description": "完整的任务列表（全量替换）。按执行顺序排列。",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content": map[string]any{
							"type":        "string",
							"description": "任务的祈使句描述。",
						},
						"status": map[string]any{
							"type":        "string",
							"enum":        []string{"pending", "in_progress", "completed"},
							"description": "任务状态。同时只保持一个 in_progress。",
						},
						"activeForm": map[string]any{
							"type":        "string",
							"description": "进行时态描述，在 in_progress 时展示（如「正在添加类型提示」）。",
						},
					},
					"required": []string{"content", "status"},
				},
			},
		},
		"required": []string{"todos"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

func (t *TodoWriteTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	var p todoWriteArgs
	if err := decodeArgs(args, &p); err != nil {
		return "", err
	}

	if len(p.Todos) == 0 {
		return "", fmt.Errorf("todos 不能为空")
	}

	// 验证每个条目
	var done, active, pending int
	for i, item := range p.Todos {
		if strings.TrimSpace(item.Content) == "" {
			return "", fmt.Errorf("todos[%d].content 不能为空", i)
		}
		if !validStatuses[item.Status] {
			return "", fmt.Errorf("todos[%d].status %q 无效，必须为 pending/in_progress/completed", i, item.Status)
		}
		switch item.Status {
		case "completed":
			done++
		case "in_progress":
			active++
		case "pending":
			pending++
		}
	}

	// 转换为 evidence.TodoItem
	todos := make([]evidence.TodoItem, len(p.Todos))
	for i, item := range p.Todos {
		todos[i] = evidence.TodoItem{
			Content:    item.Content,
			Status:     item.Status,
			ActiveForm: item.ActiveForm,
		}
	}

	// 证据耦合：检查 completed 转换是否有 complete_step 凭证
	ledger, hasLedger := evidence.FromContext(ctx)
	if hasLedger {
		missing, hasBaseline := ledger.UnverifiedCompletedTodos(todos)
		if hasBaseline && len(missing) > 0 {
			return "", fmt.Errorf(
				"以下步骤缺少 complete_step 凭证，不能直接标记为 completed: %s。"+
					"请先对每个完成的步骤调用 complete_step 提供证据",
				strings.Join(missing, "; "))
		}
		ledger.SetTodos(todos)
	}

	// 格式化终端输出
	output := formatTodoOutput(todos, done, active, pending)
	return output, nil
}

// formatTodoOutput 生成 todo 列表的可读输出
func formatTodoOutput(todos []evidence.TodoItem, done, active, pending int) string {
	var b strings.Builder
	b.WriteString("## Current Tasks\n")
	for _, t := range todos {
		switch t.Status {
		case "completed":
			fmt.Fprintf(&b, "  [✓] %s\n", t.Content)
		case "in_progress":
			label := t.Content
			if t.ActiveForm != "" {
				label = t.ActiveForm
			}
			fmt.Fprintf(&b, "  [▸] %s\n", label)
		default:
			fmt.Fprintf(&b, "  [ ] %s\n", t.Content)
		}
	}
	fmt.Fprintf(&b, "\nTodos updated: %d total — %d completed, %d in progress, %d pending.",
		len(todos), done, active, pending)
	return b.String()
}
