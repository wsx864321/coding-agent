package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/wsx864321/coding-agent/internal/evidence"
)

// CompleteStepTool 是证据签收工具，用于正式签收一个已完成的 todo 步骤。
//
// 设计要点（对标 Reasonix complete_step）：
//   - 必须提供 evidence（至少 1 条）
//   - 证据类型：verification（验证命令）、diff（文件变更）、files（文件操作）、manual（手动确认）
//   - 宿主验证：verification 需要本轮有成功的 bash 凭证，diff/files 需要有成功的写操作凭证
//   - manual 接受但不做宿主验证
//   - 签收成功后记录到 Ledger，todo_write 可据此将对应 step 标记为 completed
type CompleteStepTool struct{}

// NewCompleteStepTool 创建 CompleteStepTool
func NewCompleteStepTool() *CompleteStepTool {
	return &CompleteStepTool{}
}

func (t *CompleteStepTool) Name() string { return "complete_step" }

func (t *CompleteStepTool) Description() string {
	return "为一个已完成的 todo 步骤提供完成证据。" +
		"调用此工具前应已完成实际工作（运行命令、编辑文件等），" +
		"签收通过后再用 todo_write 将该步骤标记为 completed。" +
		"每条 evidence 必须有 type 和 detail：" +
		"type=verification 表示运行了验证命令（需本轮有成功的 bash 调用），" +
		"type=diff 表示修改了文件（需本轮有成功的 write/edit 调用），" +
		"type=files 表示操作了文件，type=manual 表示人工确认。"
}

type completeStepArgs struct {
	Step     string         `json:"step"`
	Result   string         `json:"result"`
	Evidence []evidenceItem `json:"evidence"`
}

type evidenceItem struct {
	Type   string `json:"type"`
	Detail string `json:"detail"`
}

var validEvidenceTypes = map[string]bool{
	"verification": true,
	"diff":         true,
	"files":        true,
	"manual":       true,
}

func (t *CompleteStepTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"step": map[string]any{
				"type":        "string",
				"description": "要签收的步骤内容，必须与 todo_write 中某个 in_progress 或 pending 步骤的 content 完全匹配。",
			},
			"result": map[string]any{
				"type":        "string",
				"description": "完成结果的简要描述。",
			},
			"evidence": map[string]any{
				"type":        "array",
				"minItems":    1,
				"description": "完成证据列表（至少 1 条）。",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"type": map[string]any{
							"type":        "string",
							"enum":        []string{"verification", "diff", "files", "manual"},
							"description": "证据类型：verification=验证命令, diff=文件变更, files=文件操作, manual=手动确认。",
						},
						"detail": map[string]any{
							"type":        "string",
							"description": "证据的具体描述（如运行了什么命令、修改了哪个文件）。",
						},
					},
					"required": []string{"type", "detail"},
				},
			},
		},
		"required": []string{"step", "result", "evidence"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

func (t *CompleteStepTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	var p completeStepArgs
	if err := decodeArgs(args, &p); err != nil {
		return "", err
	}

	if strings.TrimSpace(p.Step) == "" {
		return "", fmt.Errorf("step 不能为空")
	}
	if strings.TrimSpace(p.Result) == "" {
		return "", fmt.Errorf("result 不能为空")
	}
	if len(p.Evidence) == 0 {
		return "", fmt.Errorf("evidence 至少需要 1 条")
	}

	// 验证证据格式
	for i, e := range p.Evidence {
		if !validEvidenceTypes[e.Type] {
			return "", fmt.Errorf("evidence[%d].type %q 无效，必须为 verification/diff/files/manual", i, e.Type)
		}
		if strings.TrimSpace(e.Detail) == "" {
			return "", fmt.Errorf("evidence[%d].detail 不能为空", i)
		}
	}

	ledger, ok := evidence.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf("evidence ledger 不可用")
	}

	// 检查 step 是否匹配当前 todo 列表中的某个条目
	todos := ledger.CurrentTodos()
	if len(todos) > 0 {
		found := false
		for _, todo := range todos {
			if todo.Content == p.Step {
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("step %q 不匹配当前 todo 列表中的任何条目", p.Step)
		}
	}

	// 宿主验证：检查证据是否有对应的工具调用凭证
	var unverified []string
	for _, e := range p.Evidence {
		switch e.Type {
		case "verification":
			if !ledger.HasSuccessfulReceipt("bash") {
				unverified = append(unverified, fmt.Sprintf(
					"verification 证据 %q 无法验证：本轮没有成功的 bash 调用", e.Detail))
			}
		case "diff":
			if !ledger.HasAnyWriteReceipt() {
				unverified = append(unverified, fmt.Sprintf(
					"diff 证据 %q 无法验证：本轮没有成功的 write_file/edit_file 调用", e.Detail))
			}
		case "files":
			if !ledger.HasAnyWriteReceipt() && !ledger.HasSuccessfulReceipt("read_file") {
				unverified = append(unverified, fmt.Sprintf(
					"files 证据 %q 无法验证：本轮没有成功的文件操作调用", e.Detail))
			}
		case "manual":
			// manual 类型接受但不做宿主验证
		}
	}

	if len(unverified) > 0 {
		return "", fmt.Errorf("证据验证失败:\n%s", strings.Join(unverified, "\n"))
	}

	// 签收成功
	ledger.MarkStepCompleted(p.Step)

	return fmt.Sprintf("Step signed off: %s\nResult: %s\n"+
		"Move the next step to in_progress with todo_write.",
		p.Step, p.Result), nil
}
