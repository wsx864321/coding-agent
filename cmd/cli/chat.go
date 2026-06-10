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
	"github.com/wsx864321/coding-agent/internal/hooks/builtin"
	"github.com/wsx864321/coding-agent/internal/permission"
	"github.com/wsx864321/coding-agent/internal/tools"
)

// chatCmd 交互式 REPL 子命令
//
// 内建指令（以 / 开头）：
//   /help     查看帮助
//   /reset    清空除 system 外的对话历史
//   /history  查看当前消息条数
//   /tools    查看已注册工具
//   /hooks    查看已注册 hook 数量（按事件分组）
//   /exit     退出
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
  /exit     退出`,
	RunE: runChat,
}

func init() {
	rootCmd.AddCommand(chatCmd)
}

func runChat(cmd *cobra.Command, args []string) error {
	workdir := resolveWorkdir(cmd)
	registry := tools.DefaultRegistry(workdir)

	fmt.Printf("[coding-agent] REPL 已启动，workdir=%s\n", workdir)
	fmt.Printf("[coding-agent] 已注册工具: %s\n", joinToolNames(registry))

	// 共用 asker：PermissionHook（业务级 Ask）和 system Pipeline（兜底 Deny）都不装 Ask，
	// 避免同一命令被询问两次
	asker := &permission.StdinAsker{Reader: os.Stdin, Writer: os.Stderr}

	// 系统级硬约束：仅装 Deny 列表，Ask 由 PermissionHook 承担。
	// 这样 hook "放水" 也不能绕过 system deny（安全不变式），但用户不会被问两次
	checker := &permission.Pipeline{
		Deny: []permission.Checker{
			&permission.DenyListChecker{Patterns: permission.DefaultBashDenyList()},
		},
	}

	// 业务级 hooks：PermissionHook 承担 Ask + 警告 + 日志（用户友好层）
	hooks := builtin.NewDefault(workdir, asker, os.Stderr)

	a, err := agent.NewAgent(buildConfig(cmd),
		agent.WithRegistry(registry),
		agent.WithChecker(checker),
		agent.WithHooks(hooks),
	)
	if err != nil {
		return err
	}

	if c := a.Hooks(); c != nil {
		fmt.Printf("[coding-agent] 已注册 hooks: %s\n", formatHookCounts(c.Count()))
	}
	fmt.Println("[coding-agent] 输入 /help 查看可用命令，Ctrl+C 中断当前轮")

	// Ctrl+C / SIGTERM 取消当前轮
	ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

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

		switch line {
		case "/exit", "/quit":
			return nil
		case "/help":
			printChatHelp()
			continue
		case "/reset":
			a.Reset()
			fmt.Println("[coding-agent] 历史已清空")
			continue
		case "/history":
			fmt.Printf("[coding-agent] 当前消息条数: %d\n", len(a.Messages()))
			continue
		case "/tools":
			fmt.Printf("[coding-agent] 已注册工具: %s\n", joinToolNames(registry))
			continue
		case "/hooks":
			if c := a.Hooks(); c != nil {
				fmt.Printf("[coding-agent] 已注册 hooks: %s\n", formatHookCounts(c.Count()))
			} else {
				fmt.Println("[coding-agent] 未配置 hooks")
			}
			continue
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
	out, err := a.Run(ctx, prompt)
	if err != nil {
		if errors.Is(err, agent.ErrMaxTurnsExceeded) {
			return fmt.Errorf("超过最大轮数: %w", err)
		}
		return err
	}
	fmt.Println(out)
	return nil
}

func joinToolNames(r *tools.Registry) string {
	names := make([]string, 0, len(r.List()))
	for _, t := range r.List() {
		names = append(names, t.Name())
	}
	return strings.Join(names, ", ")
}

// formatHookCounts 把 {Event: count} 拍平成可读字符串
func formatHookCounts(m map[hooks.Event]int) string {
	// 固定事件顺序，避免不同会话下顺序不同
	order := []hooks.Event{hooks.EventUserPromptSubmit, hooks.EventPreToolUse, hooks.EventPostToolUse, hooks.EventStop}
	parts := make([]string, 0, len(order))
	for _, ev := range order {
		parts = append(parts, fmt.Sprintf("%s=%d", ev, m[ev]))
	}
	return strings.Join(parts, ", ")
}

func printChatHelp() {
	fmt.Println("可用命令:")
	fmt.Println("  /help     查看帮助")
	fmt.Println("  /reset    清空对话历史")
	fmt.Println("  /history  查看当前消息条数")
	fmt.Println("  /tools    查看已注册工具")
	fmt.Println("  /hooks    查看已注册 hook 数量")
	fmt.Println("  /exit     退出")
}
