package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/wsx864321/coding-agent/internal/hooks"
	"github.com/wsx864321/coding-agent/internal/permission"
	"github.com/wsx864321/coding-agent/internal/provider"
	"github.com/wsx864321/coding-agent/internal/tools"
)

// SubagentMetaTools 返回 subagent 不应继承的 meta 工具名称列表。
func SubagentMetaTools() []string {
	return []string{
		"task",          // 防止递归 spawn（只允许一层嵌套）
		"todo_write",    // 子 agent 的 todo 状态不应影响父 agent
		"complete_step", // 同上
		"run_skill",     // skill 执行是父 agent 的职责
		"install_skill", // 同上
		"bash_output",   // 后台任务操作是父 agent 的职责（子 agent 无 jobMgr）
		"kill_shell",    // 同上
		"wait",          // 同上
	}
}

const DefaultSubagentSystemPrompt = `你是一个由父 agent 派生的子 agent，负责完成一个聚焦的子任务。
使用提供的工具进行调查或操作。返回一个简洁、自包含的最终回答——
父 agent 只能看到这个回答，看不到你的工具调用和推理过程。
如果需要澄清，请以明确的问题失败，而不是猜测。`

type SubagentOptions struct {
	SystemPrompt string
	MaxTurns     int
	Registry     *tools.Registry
	Hooks        *hooks.Registry
	Checker      permission.Checker
}

// RunSubAgent 在一个全新的 session 中运行 prompt 直到完成。
// 共享 Provider 实例（连接池复用）。
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
		ProviderKind:      parent.cfg.ProviderKind,
		APIKey:            parent.cfg.APIKey,
		BaseURL:           parent.cfg.BaseURL,
		Model:             parent.cfg.Model,
		MaxTokens:         parent.cfg.MaxTokens,
		MaxTurns:          maxTurns,
		SystemPrompt:      sysPrompt,
		Temperature:       parent.cfg.Temperature,
		ContextWindow:     parent.cfg.ContextWindow,
		SoftCompactRatio:  parent.cfg.SoftCompactRatio,
		CompactRatio:      parent.cfg.CompactRatio,
		CompactForceRatio: parent.cfg.CompactForceRatio,
		RecentKeep:        parent.cfg.RecentKeep,
		MaxMessagesSnip:   parent.cfg.MaxMessagesSnip,
		ArchiveDir:        parent.cfg.ArchiveDir,
	}

	subOpts := []Option{
		WithRegistry(reg),
		WithProvider(parent.prov), // 复用父 agent 的 Provider 实例
	}
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

	subCtx := hooks.WithSubagentFlag(ctx)
	answer, err := sub.Run(subCtx, prompt)
	if err != nil {
		return "", fmt.Errorf("subagent 执行失败: %w", err)
	}

	if strings.TrimSpace(answer) != "" {
		return answer, nil
	}
	for i := len(sub.messages) - 1; i >= 0; i-- {
		m := sub.messages[i]
		if m.Role == provider.RoleAssistant && strings.TrimSpace(m.Content) != "" {
			return m.Content, nil
		}
	}
	return "", fmt.Errorf("subagent 结束但未产生最终回答")
}
