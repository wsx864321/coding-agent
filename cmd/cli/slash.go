package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/wsx864321/coding-agent/internal/agent"
	"github.com/wsx864321/coding-agent/internal/skill"
	"github.com/wsx864321/coding-agent/internal/tools"
)

// SlashHandler 是斜杠命令的共享处理器，供 chat REPL 和 TUI 共用。
type SlashHandler struct {
	Agent    *agent.Agent
	Registry *tools.Registry
	Skills   *skill.Store
}

// HandleResult 是 Handle 的返回结果
type HandleResult struct {
	Handled bool   // 是否已识别命令
	Status  string // 反馈消息（状态文本）
	Prompt  string // 若非空，调用方应将此文本作为 agent prompt 发送
	Quit    bool   // 是否应退出程序
}

// Handle 处理一条以 / 开头的命令。
func (h *SlashHandler) Handle(ctx context.Context, line string) HandleResult {
	parts := strings.SplitN(line, " ", 2)
	cmd := strings.ToLower(parts[0])
	arg := ""
	if len(parts) > 1 {
		arg = strings.TrimSpace(parts[1])
	}

	switch cmd {
	case "/exit", "/quit":
		return HandleResult{Handled: true, Status: "再见！", Quit: true}

	case "/help":
		return HandleResult{Handled: true, Status: h.helpText()}

	case "/reset":
		if h.Agent != nil {
			h.Agent.Reset()
		}
		return HandleResult{Handled: true, Status: "对话历史已清空"}

	case "/history":
		if h.Agent != nil {
			return HandleResult{Handled: true, Status: fmt.Sprintf("当前消息条数: %d", len(h.Agent.Messages()))}
		}
		return HandleResult{Handled: true, Status: "Agent 未初始化"}

	case "/tools":
		if h.Registry != nil {
			return HandleResult{Handled: true, Status: fmt.Sprintf("已注册工具: %s", joinToolNames(h.Registry))}
		}
		return HandleResult{Handled: true, Status: "工具注册表未初始化"}

	case "/skills":
		if h.Skills == nil {
			return HandleResult{Handled: true, Status: "Skill store 未初始化"}
		}
		skills := h.Skills.List()
		if len(skills) == 0 {
			return HandleResult{Handled: true, Status: "未加载任何 skill"}
		}
		return HandleResult{Handled: true, Status: skill.Catalog(skills)}

	case "/compact":
		if h.Agent != nil {
			if err := h.Agent.CompactNow(ctx, arg); err != nil {
				return HandleResult{Handled: true, Status: fmt.Sprintf("/compact 失败: %v", err)}
			}
			return HandleResult{Handled: true, Status: fmt.Sprintf("上下文压缩完成 (%s)", h.Agent.ContextStats())}
		}
		return HandleResult{Handled: true, Status: "Agent 未初始化"}

	case "/jobs":
		if h.Agent == nil {
			return HandleResult{Handled: true, Status: "Agent 未初始化"}
		}
		mgr := h.Agent.JobManager()
		if mgr == nil {
			return HandleResult{Handled: true, Status: "后台任务未启用"}
		}
		running := mgr.Running()
		if len(running) == 0 {
			return HandleResult{Handled: true, Status: "无运行中的后台任务"}
		}
		var b strings.Builder
		fmt.Fprintf(&b, "运行中的后台任务 (%d):\n", len(running))
		for _, v := range running {
			label := v.ID
			if v.Label != "" {
				label = fmt.Sprintf("%s (%s)", v.ID, v.Label)
			}
			fmt.Fprintf(&b, "  %s\n", label)
		}
		return HandleResult{Handled: true, Status: b.String()}
	}

	// 尝试匹配 skill 快捷命令：/<skill_name> [args]
	if h.Skills != nil {
		skillName := strings.TrimPrefix(cmd, "/")
		sk := h.Skills.Get(skillName)
		if sk != nil {
			prompt := fmt.Sprintf("[skill: %s] %s\n\n%s", sk.Name, sk.Description, sk.Body)
			if arg != "" {
				prompt += fmt.Sprintf("\n\n用户参数: %s", arg)
			}
			return HandleResult{Handled: true, Prompt: prompt}
		}
	}

	return HandleResult{}
}

func (h *SlashHandler) helpText() string {
	var b strings.Builder
	b.WriteString("可用命令:\n")
	b.WriteString("  /help     查看帮助\n")
	b.WriteString("  /reset    清空对话历史\n")
	b.WriteString("  /history  查看当前消息条数\n")
	b.WriteString("  /tools    查看已注册工具\n")
	b.WriteString("  /skills   查看已加载的 skill 列表\n")
	b.WriteString("  /compact  手动触发上下文压缩（可附 focus）\n")
	b.WriteString("  /jobs     查看运行中的后台任务\n")
	b.WriteString("  /exit     退出\n")

	if h.Skills != nil {
		skills := h.Skills.List()
		if len(skills) > 0 {
			b.WriteString("\nSkill 快捷命令:\n")
			for _, sk := range skills {
				fmt.Fprintf(&b, "  /%-18s %s\n", sk.Name, sk.Description)
			}
		}
	}
	return b.String()
}
