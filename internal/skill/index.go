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

	// 按 scope 分组
	var project, global, builtin []Skill
	for _, sk := range skills {
		switch sk.Scope {
		case ScopeProject:
			project = append(project, sk)
		case ScopeGlobal:
			global = append(global, sk)
		default:
			builtin = append(builtin, sk)
		}
	}

	var b strings.Builder
	b.WriteString("──────────────────────────────────────────────────────────────────────────────\n")
	b.WriteString("  Skills\n")
	fmt.Fprintf(&b, "  %d skills\n\n", len(skills))

	writeGroup := func(label string, group []Skill) {
		if len(group) == 0 {
			return
		}
		fmt.Fprintf(&b, "  %s\n", label)
		for _, sk := range group {
			tag := ""
			if sk.RunAs == RunSubagent {
				tag = "[subagent] "
			}
			name := sk.Name
			if len(name) > 25 {
				name = name[:22] + "..."
			}
			desc := sk.Description
			if len(desc) > 50 {
				desc = desc[:47] + "..."
			}
			fmt.Fprintf(&b, "  %s%s — %s\n", tag, name, desc)
		}
		b.WriteByte('\n')
	}

	writeGroup("Project skills (.coding-agent/skills)", project)
	writeGroup("Global skills (~/.coding-agent/skills)", global)
	writeGroup("Builtin skills", builtin)
	return b.String()
}
