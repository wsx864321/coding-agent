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
	return b.String()
}
