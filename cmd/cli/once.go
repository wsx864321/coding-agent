package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/wsx864321/coding-agent/internal/agent"
	"github.com/wsx864321/coding-agent/internal/event"
	"github.com/wsx864321/coding-agent/internal/hooks"
	"github.com/wsx864321/coding-agent/internal/lsp"
	"github.com/wsx864321/coding-agent/internal/mcp"
	"github.com/wsx864321/coding-agent/internal/permission"
	"github.com/wsx864321/coding-agent/internal/skill"
	"github.com/wsx864321/coding-agent/internal/tools"
)

// onceCmd 一次性对话子命令
//
// 用法：
//
//	coding-agent once -m "请总结 main.go"
var onceCmd = &cobra.Command{
	Use:   "once",
	Short: "一次性对话：执行一次后退出",
	Example: `  coding-agent once -m "列出当前目录所有 go 文件"
  coding-agent once -m "读取 main.go 并总结" -M gpt-4o`,
	RunE: runOnce,
}

var (
	onceMessage string
	onceQuiet   bool
)

func init() {
	rootCmd.AddCommand(onceCmd)
	onceCmd.Flags().StringVarP(&onceMessage, "message", "m", "", "用户输入（必填）")
	onceCmd.Flags().BoolVarP(&onceQuiet, "quiet", "q", false, "仅打印最终回答（不打印提示信息）")
	_ = onceCmd.MarkFlagRequired("message")
}

func runOnce(cmd *cobra.Command, args []string) error {
	workdir := resolveWorkdir(cmd)
	registry := tools.DefaultRegistry(workdir)

	// 初始化 skill store
	skillStore := skill.NewStore(skill.StoreOptions{Workdir: workdir})
	registry.Register(skill.NewRunSkillTool(skillStore, nil))
	registry.Register(skill.NewInstallSkillTool(skillStore))

	checker := &permission.Pipeline{
		Deny: []permission.Checker{
			permission.NewDenyListChecker(),
			permission.NewBashAskChecker(nil),
			permission.NewWorkdirChecker(workdir, nil),
		},
	}

	sink := &event.TextSink{Out: os.Stdout, Err: os.Stderr}
	if onceQuiet {
		sink = &event.TextSink{Out: os.Stdout, Err: io.Discard}
	}
	notify := func(msg string) {
		sink.Emit(event.Event{Kind: event.Notice, Level: event.LevelWarn, Text: msg})
	}

	hookRunner := hooks.NewRunner(
		hooks.Load(hooks.LoadOptions{ProjectRoot: workdir}),
		workdir,
		hooks.DefaultSpawner,
		notify,
	)

	// 加载并启动 MCP server
	mcpConfigs := mcp.Load(mcp.LoadOptions{ProjectRoot: workdir})
	mcpManager := mcp.NewManager(mcpConfigs, registry)
	mcpManager.Start()
	defer mcpManager.Stop()

	// 注册 MCP 安装/卸载工具
	registry.Register(mcp.NewInstallSourceTool(mcpManager, workdir))

	// 加载并启动 LSP server（多语言自动检测）
	lspManager := lsp.NewManager(workdir)
	lspManager.Start()
	defer lspManager.Stop()

	// 注册 LSP 工具
	registry.Register(tools.NewLSPDefinitionTool(lspManager, []string{workdir}))
	registry.Register(tools.NewLSPReferencesTool(lspManager, []string{workdir}))
	registry.Register(tools.NewLSPHoverTool(lspManager, []string{workdir}))
	registry.Register(tools.NewLSPDiagnosticsTool(lspManager, []string{workdir}))
	registry.Register(tools.NewCodeIndexTool(lspManager, []string{workdir}))

	a, err := agent.NewAgent(buildConfig(cmd),
		agent.WithRegistry(registry),
		agent.WithChecker(checker),
		agent.WithHooks(hookRunner),
		agent.WithSkillStore(skillStore),
		agent.WithSink(sink),
	)
	if err != nil {
		return err
	}
	a.WireTaskTool()
	a.WireSkillTools()

	// 检测 worktree 状态并注入 system prompt 上下文
	wtInfo := agent.DetectWorktree(workdir)
	a.SetWorktreeContext(wtInfo)

	// 确保 .worktrees/ 在 .gitignore 中（安全守护）
	agent.EnsureWorktreeGitignore(workdir)

	if !onceQuiet {
		fmt.Fprintf(os.Stderr, "[coding-agent] running once, message=%q\n", truncate(onceMessage, 60))
	}

	_, err = a.Run(cmd.Context(), onceMessage)
	return err
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
