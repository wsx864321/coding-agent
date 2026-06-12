package skill

import (
	"fmt"
	"strings"
)

// IndexMaxChars 是 skill catalog 注入 system prompt 的最大字符数
const IndexMaxChars = 4000

// ApplyIndex 将 skill catalog 追加到 system prompt 末尾
//
// 仅注入名称和描述（不含 body），保持 prompt-cache 友好。
// 超过 IndexMaxChars 时截断，确保不会撑大 system prompt。
func ApplyIndex(basePrompt string, skills []Skill) string {
	if len(skills) == 0 {
		return basePrompt
	}

	var b strings.Builder
	b.WriteString("\n\n# Skills — 可调用的技能\n\n")
	b.WriteString("你可以通过 run_skill 工具加载并执行以下技能，或者用户可以通过 /<name> 直接触发。\n")
	b.WriteString("列表只包含名称和描述；调用 run_skill 时完整指令会注入。\n\n")

	for _, sk := range skills {
		tag := ""
		if sk.RunAs == RunSubagent {
			tag = " [subagent]"
		}
		line := fmt.Sprintf("- **%s**%s — %s\n", sk.Name, tag, sk.Description)

		if b.Len()+len(line) > IndexMaxChars {
			b.WriteString("- ...（更多技能已省略）\n")
			break
		}
		b.WriteString(line)
	}

	return basePrompt + b.String()
}

// Catalog 返回技能列表的纯文本格式（用于 /skills 命令展示）
func Catalog(skills []Skill) string {
	if len(skills) == 0 {
		return "当前未加载任何 skill。"
	}

	var b strings.Builder
	b.WriteString("已加载的 skills:\n")
	for _, sk := range skills {
		mode := "inline"
		if sk.RunAs == RunSubagent {
			mode = "subagent"
		}
		fmt.Fprintf(&b, "  %-20s [%s] %s  (%s)\n", sk.Name, mode, sk.Description, sk.Scope)
	}
	return b.String()
}
