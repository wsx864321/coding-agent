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
	"github.com/wsx864321/coding-agent/internal/tools"
)

// chatCmd 交互式 REPL 子命令
//
// 内建指令（以 / 开头）：
//   /help     查看帮助
//   /reset    清空除 system 外的对话历史
//   /history  查看当前消息条数
//   /tools    查看已注册工具
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
  /exit     退出`,
	RunE: runChat,
}

func init() {
	rootCmd.AddCommand(chatCmd)
}

func runChat(cmd *cobra.Command, args []string) error {
	a, registry, err := buildAgent(cmd)
	if err != nil {
		return err
	}

	cwd, _ := os.Getwd()
	fmt.Printf("[coding-agent] REPL 已启动，cwd=%s\n", cwd)
	fmt.Printf("[coding-agent] 已注册工具: %s\n", joinToolNames(registry))
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

func printChatHelp() {
	fmt.Println("可用命令:")
	fmt.Println("  /help     查看帮助")
	fmt.Println("  /reset    清空对话历史")
	fmt.Println("  /history  查看当前消息条数")
	fmt.Println("  /tools    查看已注册工具")
	fmt.Println("  /exit     退出")
}
