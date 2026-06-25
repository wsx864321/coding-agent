package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/wsx864321/coding-agent/internal/agent"
	"github.com/wsx864321/coding-agent/internal/hooks"
	"github.com/wsx864321/coding-agent/internal/provider"
	"github.com/wsx864321/coding-agent/internal/tools"
)

// chatCmd 交互式 REPL 子命令
//
// 内建指令（以 / 开头）：
//
//	/help     查看帮助
//	/reset    清空除 system 外的对话历史
//	/history  查看当前消息条数
//	/tools    查看已注册工具
//	/hooks    查看已注册 hook 数量（按事件分组）
//	/skills   查看已加载的 skill 列表
//	/compact  手动触发一次上下文压缩（可选 focus 文本）
//	/jobs     查看运行中的后台任务
//	/<skill>  触发对应 skill（等效于向 agent 发送 run_skill 的结果）
//	/exit     退出
var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "交互式 REPL",
	Long: `启动一个交互式 REPL，逐行接收用户输入并调用 Agent。

除普通 prompt 外，还支持以下内建命令（以 / 开头）：
  /help     查看帮助
  /reset    清空对话历史（保留 system message）
  /history  查看当前消息条数
  /tools    查看已注册工具
  /hooks    查看已注册 hook 数量
  /skills   查看已加载的 skill 列表
  /compact  手动触发一次上下文压缩
  /jobs     查看运行中的后台任务
  /<skill>  触发对应 skill
  /exit     退出`,
	RunE: runChat,
}

func init() {
	chatCmd.Flags().String("resume", "", "恢复指定会话（latest=最近, 或 ID 前缀匹配）")
	chatCmd.Flags().Bool("list", false, "列出当前项目的所有会话")
	rootCmd.AddCommand(chatCmd)
}

// runChat 交互式 REPL 主循环
func runChat(cmd *cobra.Command, args []string) error {
	workdir := resolveWorkdir(cmd)
	cfg := buildConfig(cmd)
	// SessionDir 需要在 NewAgent(resolve) 之前确定，以便 --list / --resume 使用
	sessionBucket := agent.SessionBucket(agent.ResolveSessionDir(cfg.SessionDir), workdir)

	list, _ := cmd.Flags().GetBool("list")
	if list {
		return listAndPrintSessions(sessionBucket)
	}

	setup, err := setupChatAgent(cmd)
	if err != nil {
		return err
	}
	defer setup.cleanup()

	a := setup.Agent
	skillStore := setup.SkillStore
	registry := setup.Registry

	resume, _ := cmd.Flags().GetString("resume")

	fmt.Printf("[coding-agent] REPL 已启动，workdir=%s\n", workdir)
	fmt.Printf("[coding-agent] 已注册工具: %s\n", joinToolNames(registry))

	if memSet := a.MemorySet(); memSet != nil && len(memSet.Docs) > 0 {
		fmt.Printf("[coding-agent] 已加载 %d 个层级文档\n", len(memSet.Docs))
	}

	skills := skillStore.List()
	if len(skills) > 0 {
		names := make([]string, len(skills))
		for i, sk := range skills {
			names[i] = sk.Name
		}
		fmt.Printf("[coding-agent] 已加载 skills: %s\n", strings.Join(names, ", "))
	}

	// 绑定 session 路径（--resume 恢复已有 session，否则新建）
	if resume != "" {
		if err := resumeSession(a, sessionBucket, resume); err != nil {
			return err
		}
	} else {
		a.SetSessionPath(agent.NewSessionPath(sessionBucket, cfg.Model))
	}

	if info := hookCountInfo(a.Hooks()); info != "" {
		fmt.Printf("[coding-agent] 已注册 hooks: %s\n", info)
	}
	fmt.Println("[coding-agent] 输入 /help 查看可用命令，Ctrl+C 中断当前轮")

	ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	slashHandler := &SlashHandler{Agent: a, Registry: registry, Skills: skillStore}

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			fmt.Println()
			return nil
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// 内建命令分发
		if strings.HasPrefix(line, "/") {
			result := slashHandler.Handle(ctx, line)
			if result.Handled {
				if result.Status != "" {
					fmt.Println(result.Status)
				}
				if result.Quit {
					return nil
				}
				if result.Prompt != "" {
					// skill 触发：将构造好的 prompt 发送给 agent
					if err := runOneTurn(ctx, a, result.Prompt); err != nil {
						fmt.Fprintf(os.Stderr, "[coding-agent] %v\n", err)
					}
				}
				continue
			}
			// 未匹配任何命令/skill，当做普通输入
		}

		if err := runOneTurn(ctx, a, line); err != nil {
			if errors.Is(err, context.Canceled) {
				fmt.Fprintln(os.Stderr, "[coding-agent] 本轮被中断")
				continue
			}
			fmt.Fprintf(os.Stderr, "[coding-agent] 调用失败: %v\n", err)
		}
	}
}

func runOneTurn(ctx context.Context, a *agent.Agent, prompt string) error {
	_, err := a.Run(ctx, prompt)
	if err != nil {
		if errors.Is(err, agent.ErrMaxTurnsExceeded) {
			return fmt.Errorf("超过最大轮数: %w", err)
		}
		return err
	}
	fmt.Println()
	return nil
}

func joinToolNames(r *tools.Registry) string {
	names := make([]string, 0, len(r.List()))
	for _, t := range r.List() {
		names = append(names, t.Name())
	}
	return strings.Join(names, ", ")
}

// hookCountInfo 从 ToolHooks 提取 hook 数量摘要（仅 *hooks.Runner 支持 Count）。
func hookCountInfo(h agent.ToolHooks) string {
	if h == nil {
		return ""
	}
	r, ok := h.(*hooks.Runner)
	if !ok {
		return "configured"
	}
	return formatHookCounts(r.Count())
}

// formatHookCounts 把 {Event: count} 拍平成可读字符串
func formatHookCounts(m map[hooks.Event]int) string {
	order := []hooks.Event{hooks.EventUserPromptSubmit, hooks.EventPreToolUse, hooks.EventPostToolUse, hooks.EventStop}
	parts := make([]string, 0, len(order))
	for _, ev := range order {
		parts = append(parts, fmt.Sprintf("%s=%d", ev, m[ev]))
	}
	return strings.Join(parts, ", ")
}

func listAndPrintSessions(dir string) error {
	sessions, err := agent.ListSessions(dir)
	if err != nil {
		return fmt.Errorf("列出 session 失败: %w", err)
	}
	if len(sessions) == 0 {
		fmt.Println("暂无保存的会话。")
		return nil
	}
	fmt.Printf("共 %d 个会话（按时间倒序）：\n\n", len(sessions))
	for i, s := range sessions {
		fmt.Printf("  %2d. [%s] %s\n", i+1, s.ID, s.Preview)
		fmt.Printf("      轮次: %d  最后活跃: %s\n", s.Turns, s.UpdatedAt.Format("2006-01-02 15:04"))
	}
	fmt.Println("\n使用 --resume <id> 恢复会话，或 --resume latest 恢复最近会话。")
	return nil
}

// resumeSession 根据模式恢复 session：
//   - "latest"：恢复最近 session
//   - 其他：按 ID 前缀匹配（例如 --resume 202506 匹配 202506...-xxx）
func resumeSession(a *agent.Agent, dir, mode string) error {
	sessions, err := agent.ListSessions(dir)
	if err != nil {
		return fmt.Errorf("列出 session 失败: %w", err)
	}
	if len(sessions) == 0 {
		return fmt.Errorf("没有可恢复的会话——请先运行 chat 创建新会话")
	}

	var target *agent.SessionInfo
	if mode == "latest" {
		target = &sessions[0]
	} else {
		for i := range sessions {
			if strings.HasPrefix(sessions[i].ID, mode) {
				t := sessions[i] // 避免取循环变量地址
				target = &t
				break
			}
		}
	}
	if target == nil {
		return fmt.Errorf("未找到匹配 %q 的会话，使用 --list 查看可用会话", mode)
	}

	messages, err := agent.LoadSession(target.Path)
	if err != nil {
		return fmt.Errorf("加载 session 失败: %w", err)
	}

	// 替换 agent 当前的 messages（保留已存在的 system message 不动，追加其他消息）
	if len(messages) > 0 && len(a.Messages()) > 0 {
		// system message 已由 NewAgent 设置，跳过恢复的首条 system 消息
		start := 0
		if messages[0].Role == provider.RoleSystem {
			start = 1
		}
		// 逐个追加非 system 消息（绕过 append 的并发问题，直接使用内部 messages）
		a.Reset()
		for i := start; i < len(messages); i++ {
			a.AppendMessage(messages[i])
		}
	} else {
		for _, m := range messages {
			a.AppendMessage(m)
		}
	}

	a.SetSessionPath(target.Path)
	fmt.Printf("[coding-agent] 已恢复会话: %s（%d 轮, %d 条消息）\n",
		target.ID, target.Turns, len(messages))
	return nil
}
