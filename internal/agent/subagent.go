package agent

import (
	"context"
	"fmt"
	"strings"

	openai "github.com/sashabaranov/go-openai"

	"github.com/wsx864321/coding-agent/internal/hooks"
	"github.com/wsx864321/coding-agent/internal/permission"
	"github.com/wsx864321/coding-agent/internal/tools"
)


// SubagentMetaTools 返回 subagent 不应继承的 meta 工具名称列表。
//
// 排除理由：
//   - task: 防止递归 spawn（delegation 只允许一层）
//   - todo_write / complete_step: 子 agent 的 todo 状态不应影响父 agent
//   - run_skill / install_skill: skill 管理属于父 agent 职责
func SubagentMetaTools() []string {
	return []string{"task", "todo_write", "complete_step", "run_skill", "install_skill"}
}

// DefaultSubagentSystemPrompt 是通用 subagent 的默认 system prompt
const DefaultSubagentSystemPrompt = `你是一个由父 agent 派生的子 agent，负责完成一个聚焦的子任务。
使用提供的工具进行调查或操作。返回一个简洁、自包含的最终回答——
父 agent 只能看到这个回答，看不到你的工具调用和推理过程。
如果需要澄清，请以明确的问题失败，而不是猜测。`

// SubagentOptions 控制 subagent 的运行参数
type SubagentOptions struct {
	// SystemPrompt 子 agent 的系统提示；空字符串使用 DefaultSubagentSystemPrompt
	SystemPrompt string
	// MaxTurns 子 agent 的最大轮数；0 表示自动计算（父 MaxTurns / 2，最少 5）
	MaxTurns int
	// Registry 子 agent 的工具注册表；nil 时从父 registry 过滤生成
	Registry *tools.Registry
	// Hooks 子 agent 的 hook 注册表；nil 表示不使用 hooks
	Hooks *hooks.Registry
	// Checker 子 agent 的权限检查器；nil 表示放行
	Checker permission.Checker
}

// RunSubAgent 在一个全新的 session 中运行 prompt 直到完成，返回子 agent 的最终回答。
//
// 这是 task 工具及未来技能工具的共享核心：调用方提供 system prompt、工具注册表和运行参数，
// RunSubAgent 构造一个独立的 Agent 实例，执行 prompt，提取最终的 assistant 文本。
//
// 隔离边界：
//   - 全新的 messages 历史（不继承父 agent 的对话）
//   - 独立的 evidence ledger（不影响父 agent 的 todo 状态）
//   - 共享文件系统（写操作会持久化）
//   - 共享 OpenAI client（同一个 API 配置）
func RunSubAgent(ctx context.Context, parent *Agent, prompt string, opts SubagentOptions) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("subagent prompt 不能为空")
	}

	sysPrompt := opts.SystemPrompt
	if sysPrompt == "" {
		sysPrompt = DefaultSubagentSystemPrompt
	}

	maxTurns := opts.MaxTurns
	if maxTurns <= 0 {
		maxTurns = parent.cfg.MaxTurns / 2
		if maxTurns < 5 {
			maxTurns = 5
		}
	}

	reg := opts.Registry
	if reg == nil {
		reg = tools.FilterRegistry(parent.registry, SubagentMetaTools()...)
	}

	subCfg := Config{
		APIKey:       parent.cfg.APIKey,
		BaseURL:      parent.cfg.BaseURL,
		Model:        parent.cfg.Model,
		MaxTokens:    parent.cfg.MaxTokens,
		MaxTurns:     maxTurns,
		SystemPrompt: sysPrompt,
		Temperature:  parent.cfg.Temperature,
	}

	subOpts := []Option{WithRegistry(reg)}
	if opts.Hooks != nil {
		subOpts = append(subOpts, WithHooks(opts.Hooks))
	}
	if opts.Checker != nil {
		subOpts = append(subOpts, WithChecker(opts.Checker))
	}

	sub, err := NewAgent(subCfg, subOpts...)
	if err != nil {
		return "", fmt.Errorf("构造 subagent 失败: %w", err)
	}
	// 复用父 agent 的 client（共享连接池，避免重复构造）
	sub.client = parent.client

	// 在 context 中标记 subagent，让继承的 hooks（如 LogHook）能区分日志来源
	subCtx := hooks.WithSubagentFlag(ctx)
	answer, err := sub.Run(subCtx, prompt)
	if err != nil {
		return "", fmt.Errorf("subagent 执行失败: %w", err)
	}

	// 反向遍历取最后一个非空 assistant 文本作为最终回答
	if strings.TrimSpace(answer) != "" {
		return answer, nil
	}
	for i := len(sub.messages) - 1; i >= 0; i-- {
		m := sub.messages[i]
		if m.Role == openai.ChatMessageRoleAssistant && strings.TrimSpace(m.Content) != "" {
			return m.Content, nil
		}
	}
	return "", fmt.Errorf("subagent 结束但未产生最终回答")
}
