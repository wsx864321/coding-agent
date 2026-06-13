package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// SkillRunner 是 subagent 模式下 skill 的执行器。
// 由 agent 包实现并注入，避免 skill → agent 的循环依赖。
type SkillRunner func(ctx context.Context, sk Skill, task string) (string, error)

// ---------- run_skill ----------

// RunSkillTool 加载并执行一个已注册的 skill
type RunSkillTool struct {
	store  *Store
	runner SkillRunner // subagent 执行器（延迟注入）
}

// NewRunSkillTool 创建 run_skill 工具
func NewRunSkillTool(store *Store, runner SkillRunner) *RunSkillTool {
	return &RunSkillTool{store: store, runner: runner}
}

// SetRunner 注入 SkillRunner（延迟连线）
func (t *RunSkillTool) SetRunner(runner SkillRunner) {
	t.runner = runner
}

func (t *RunSkillTool) ReadOnly() bool { return false }

func (t *RunSkillTool) Name() string { return "run_skill" }

func (t *RunSkillTool) Description() string {
	return "加载并执行一个已注册的 skill。inline 模式返回 skill 完整内容供你遵循；" +
		"subagent 模式在独立 session 中执行并返回结果。"
}

func (t *RunSkillTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "要执行的 skill 名称（从 system prompt 中的 Skills 列表获取）",
			},
			"arguments": map[string]any{
				"type":        "string",
				"description": "传递给 skill 的额外参数或任务描述（可选）",
			},
		},
		"required": []string{"name"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

func (t *RunSkillTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	raw, _ := json.Marshal(args)
	var p struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", fmt.Errorf("解析参数失败: %w", err)
	}

	if strings.TrimSpace(p.Name) == "" {
		return "", fmt.Errorf("skill name 不能为空")
	}

	sk := t.store.Get(p.Name)
	if sk == nil {
		available := t.store.List()
		names := make([]string, len(available))
		for i, s := range available {
			names[i] = s.Name
		}
		return "", fmt.Errorf("skill %q 未找到，可用的 skill: %s", p.Name, strings.Join(names, ", "))
	}

	if sk.RunAs == RunSubagent {
		if t.runner == nil {
			return "", fmt.Errorf("subagent runner 未配置，无法执行 subagent skill %q", sk.Name)
		}
		task := p.Arguments
		if task == "" {
			task = "执行 skill 中定义的任务"
		}
		return t.runner(ctx, *sk, task)
	}

	// inline 模式：返回 skill body 供 LLM 遵循
	return renderInline(*sk, p.Arguments), nil
}

// renderInline 将 skill 内容包装为 inline 注入格式
func renderInline(sk Skill, args string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Skill: %s\n\n", sk.Name))
	if args != "" {
		b.WriteString(fmt.Sprintf("**参数**: %s\n\n", args))
	}
	b.WriteString("---\n\n")
	b.WriteString(sk.Body)
	b.WriteString("\n\n---\n请严格按照以上 skill 指令执行。")
	return b.String()
}

// ---------- install_skill ----------

// InstallSkillTool 在运行时创建或更新一个 skill
type InstallSkillTool struct {
	store *Store
}

// NewInstallSkillTool 创建 install_skill 工具
func NewInstallSkillTool(store *Store) *InstallSkillTool {
	return &InstallSkillTool{store: store}
}

func (t *InstallSkillTool) ReadOnly() bool { return false }

func (t *InstallSkillTool) Name() string { return "install_skill" }

func (t *InstallSkillTool) Description() string {
	return "创建或更新一个 skill，保存到项目的 .coding-agent/skills/ 目录。" +
		"content 必须是完整的 SKILL.md 格式（包含 YAML frontmatter）。" +
		"保存后 skill 立即可通过 run_skill 调用。"
}

func (t *InstallSkillTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "skill 名称（只能包含字母、数字、连字符、下划线，以字母开头）",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "完整的 SKILL.md 内容，包含 YAML frontmatter（---\\nname: ...\\ndescription: ...\\n---）和 markdown body",
			},
		},
		"required": []string{"name", "content"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

func (t *InstallSkillTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	raw, _ := json.Marshal(args)
	var p struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", fmt.Errorf("解析参数失败: %w", err)
	}

	if strings.TrimSpace(p.Name) == "" {
		return "", fmt.Errorf("skill name 不能为空")
	}
	if strings.TrimSpace(p.Content) == "" {
		return "", fmt.Errorf("skill content 不能为空")
	}

	path, err := t.store.Install(p.Name, p.Content)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Skill %q 已保存到 %s\n可通过 run_skill 或 /%s 调用。", p.Name, path, p.Name), nil
}
