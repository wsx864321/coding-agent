package agent

import (
	"fmt"
	"strings"

	"github.com/wsx864321/coding-agent/internal/tools"
)

// buildSystemPrompt 根据已注册工具自动生成 system prompt
//
// 输出格式（示例）：
//
//	你是一个编码助手，可以使用以下工具完成任务：
//
//	1. bash
//	   描述: 在本地终端执行...
//	   参数 schema: {"type":"object","properties":{...}}
//
//	2. read_file
//	   ...
//
//	请按用户意图选择合适的工具，按需连续调用多个工具以完成任务。
func buildSystemPrompt(registry *tools.Registry) string {
	toolList := registry.List()
	if len(toolList) == 0 {
		return "你是一个编码助手。当前未注册任何工具，请直接回答用户问题。"
	}

	var b strings.Builder
	b.WriteString("你是一个编码助手，可以使用以下工具完成任务。\n\n")

	for i, t := range toolList {
		fmt.Fprintf(&b, "%d. %s\n", i+1, t.Name())
		fmt.Fprintf(&b, "   描述: %s\n", t.Description())
		// Schema 是 json.RawMessage，直接 stringify
		schema := t.Schema()
		if len(schema) > 0 {
			fmt.Fprintf(&b, "   参数 schema: %s\n", string(schema))
		}
		b.WriteString("\n")
	}

	b.WriteString("请按用户意图选择合适的工具，按需连续调用多个工具以完成任务。\n")

	if hasTodoTools(registry) {
		b.WriteString(todoGuidance)
	}

	return b.String()
}

// hasTodoTools 检查 registry 中是否注册了 todo_write 和 complete_step
func hasTodoTools(registry *tools.Registry) bool {
	return registry.Get("todo_write") != nil && registry.Get("complete_step") != nil
}

const todoGuidance = `
对于多步骤任务（3 步以上），使用 todo_write 和 complete_step 工具跟踪进度：
- 动手之前先调 todo_write 列出所有步骤，第一步设为 in_progress，其余 pending
- 每次只保持一个 in_progress
- 完成一个步骤的实际工作后，先调 complete_step 提供完成证据，再调 todo_write 更新状态
- 不要跳过 complete_step 直接将步骤标记为 completed，否则 todo_write 会拒绝
- 简单任务（少于 3 步）无需使用 todo_write
`
