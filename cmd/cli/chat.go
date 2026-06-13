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
	"github.com/wsx864321/coding-agent/internal/skill"
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
  /<skill>  触发对应 skill
  /exit     退出`,
	RunE: runChat,
}

func init() {
	rootCmd.AddCommand(chatCmd)
}

func runChat(cmd *cobra.Command, args []string) error {
	workdir := resolveWorkdir(cmd)
	registry := tools.DefaultRegistry(workdir)

	// 初始化 skill store：扫描 project / global / builtin 技能
	skillStore := skill.NewStore(skill.StoreOptions{Workdir: workdir})

	// 注册 skill 工具到 registry
	registry.Register(skill.NewRunSkillTool(skillStore, nil))
	registry.Register(skill.NewInstallSkillTool(skillStore))

	fmt.Printf("[coding-agent] REPL 已启动，workdir=%s\n", workdir)
	fmt.Printf("[coding-agent] 已注册工具: %s\n", joinToolNames(registry))

	skills := skillStore.List()
	if len(skills) > 0 {
		names := make([]string, len(skills))
		for i, sk := range skills {
			names[i] = sk.Name
		}
		fmt.Printf("[coding-agent] 已加载 skills: %s\n", strings.Join(names, ", "))
	}

	asker := &permission.StdinAsker{Reader: os.Stdin, Writer: os.Stderr}

	checker := &permission.Pipeline{
		Deny: []permission.Checker{
			permission.NewDenyListChecker(),
			permission.NewBashAskChecker(asker),
			permission.NewWorkdirChecker(workdir, asker),
		},
	}

	a, err := agent.NewAgent(buildConfig(cmd),
		agent.WithRegistry(registry),
		agent.WithChecker(checker),
		agent.WithHooks(builtin.NewDefault(os.Stderr, workdir)),
		agent.WithSkillStore(skillStore),
	)
	if err != nil {
		return err
	}
	a.WireTaskTool()
	a.WireSkillTools()

	if c := a.Hooks(); c != nil {
		fmt.Printf("[coding-agent] 已注册 hooks: %s\n", formatHookCounts(c.Count()))
	}
	fmt.Println("[coding-agent] 输入 /help 查看可用命令，Ctrl+C 中断当前轮")

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

		// 内建命令分发
		if strings.HasPrefix(line, "/") {
			handled, err := handleSlashCommand(ctx, a, skillStore, registry, line)
			if err != nil {
				fmt.Fprintf(os.Stderr, "[coding-agent] %v\n", err)
			}
			if handled {
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

// handleSlashCommand 处理 / 开头的命令，返回 (是否已处理, 错误)
func handleSlashCommand(ctx context.Context, a *agent.Agent, store *skill.Store, registry *tools.Registry, line string) (bool, error) {
	parts := strings.SplitN(line, " ", 2)
	cmd := strings.ToLower(parts[0])
	slashArgs := ""
	if len(parts) > 1 {
		slashArgs = strings.TrimSpace(parts[1])
	}

	switch cmd {
	case "/exit", "/quit":
		fmt.Println("[coding-agent] 再见！")
		os.Exit(0)
		return true, nil

	case "/help":
		printChatHelp(store)
		return true, nil

	case "/reset":
		a.Reset()
		fmt.Println("[coding-agent] 历史已清空")
		return true, nil

	case "/history":
		fmt.Printf("[coding-agent] 当前消息条数: %d\n", len(a.Messages()))
		return true, nil

	case "/tools":
		fmt.Printf("[coding-agent] 已注册工具: %s\n", joinToolNames(registry))
		return true, nil

	case "/hooks":
		if c := a.Hooks(); c != nil {
			fmt.Printf("[coding-agent] 已注册 hooks: %s\n", formatHookCounts(c.Count()))
		} else {
			fmt.Println("[coding-agent] 未配置 hooks")
		}
		return true, nil

	case "/skills":
		skills := store.List()
		fmt.Println(skill.Catalog(skills))
		return true, nil

	case "/compact":
		if err := a.CompactNow(ctx, slashArgs); err != nil {
			return true, fmt.Errorf("/compact 失败: %w", err)
		}
		fmt.Printf("[coding-agent] context compact 完成 (%s)\n", a.ContextStats())
		return true, nil
	}

	// 尝试匹配 skill slash 命令：/<skill_name> [args]
	skillName := strings.TrimPrefix(cmd, "/")
	sk := store.Get(skillName)
	if sk != nil {
		prompt := fmt.Sprintf("[skill: %s] %s\n\n%s", sk.Name, sk.Description, sk.Body)
		if slashArgs != "" {
			prompt += fmt.Sprintf("\n\n用户参数: %s", slashArgs)
		}
		if err := runOneTurn(ctx, a, prompt); err != nil {
			return true, err
		}
		return true, nil
	}

	return false, nil
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
	order := []hooks.Event{hooks.EventUserPromptSubmit, hooks.EventPreToolUse, hooks.EventPostToolUse, hooks.EventStop}
	parts := make([]string, 0, len(order))
	for _, ev := range order {
		parts = append(parts, fmt.Sprintf("%s=%d", ev, m[ev]))
	}
	return strings.Join(parts, ", ")
}

func printChatHelp(store *skill.Store) {
	fmt.Println("可用命令:")
	fmt.Println("  /help     查看帮助")
	fmt.Println("  /reset    清空对话历史")
	fmt.Println("  /history  查看当前消息条数")
	fmt.Println("  /tools    查看已注册工具")
	fmt.Println("  /hooks    查看已注册 hook 数量")
	fmt.Println("  /skills   查看已加载的 skill 列表")
	fmt.Println("  /compact  手动触发一次上下文压缩（可附 focus）")
	fmt.Println("  /exit     退出")

	if store != nil {
		skills := store.List()
		if len(skills) > 0 {
			fmt.Println()
			fmt.Println("Skill 快捷命令:")
			for _, sk := range skills {
				fmt.Printf("  /%-18s %s\n", sk.Name, sk.Description)
			}
		}
	}
}
