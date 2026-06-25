package cli

import (
	"io"
	"log"
	"os"

	"github.com/spf13/cobra"

	"github.com/wsx864321/coding-agent/internal/agent"
	"github.com/wsx864321/coding-agent/internal/event"
	"github.com/wsx864321/coding-agent/internal/hooks"
	"github.com/wsx864321/coding-agent/internal/jobs"
	"github.com/wsx864321/coding-agent/internal/lsp"
	"github.com/wsx864321/coding-agent/internal/mcp"
	"github.com/wsx864321/coding-agent/internal/memory"
	"github.com/wsx864321/coding-agent/internal/permission"
	"github.com/wsx864321/coding-agent/internal/skill"
	"github.com/wsx864321/coding-agent/internal/tools"
	"github.com/wsx864321/coding-agent/internal/tui"
)

type chatSetup struct {
	Agent      *agent.Agent
	SkillStore *skill.Store
	Registry   *tools.Registry
	TuiSink    *tui.TuiSink
	MCPManager *mcp.Manager
	LSPManager *lsp.Manager
	cleanup    func()
}

// setupChatAgent 构造 chat/tui 共用的 agent 装配链；调用方须 defer setup.cleanup()。
func setupChatAgent(cmd *cobra.Command) (*chatSetup, error) {
	asker := &permission.StdinAsker{Reader: os.Stdin, Writer: os.Stderr}
	return setupAgentWithAsker(cmd, asker, nil, log.Default())
}

// setupTuiAgent 为 TUI 构造 agent：通过 TuiSink 在 TUI 横幅中请求用户审批。
func setupTuiAgent(cmd *cobra.Command) (*chatSetup, error) {
	tuiSink := &tui.TuiSink{}
	asker := agent.SinkAsker{Sink: tuiSink}
	return setupAgentWithAsker(cmd, asker, tuiSink, log.New(io.Discard, "", 0))
}

func setupAgentWithAsker(cmd *cobra.Command, asker permission.Asker, tuiSink *tui.TuiSink, logger *log.Logger) (*chatSetup, error) {
	workdir := resolveWorkdir(cmd)
	registry := tools.DefaultRegistry(workdir)

	skillStore := skill.NewStore(skill.StoreOptions{Workdir: workdir})
	registry.Register(skill.NewRunSkillTool(skillStore, nil))
	registry.Register(skill.NewInstallSkillTool(skillStore))

	memSet := memory.Load(memory.Options{CWD: workdir, Workdir: workdir})

	checker := &permission.Pipeline{
		Deny: []permission.Checker{
			permission.NewDenyListChecker(),
			permission.NewBashAskChecker(asker),
			permission.NewWorkdirChecker(workdir, asker),
		},
	}

	jobMgr := jobs.NewManager()

	var sink event.Sink
	if tuiSink != nil {
		sink = tuiSink
	} else {
		sink = &event.TextSink{Out: os.Stdout, Err: os.Stderr}
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
	mcpManager.SetLogger(logger)
	mcpManager.Start()

	// 注册 MCP 安装/卸载工具
	registry.Register(mcp.NewInstallSourceTool(mcpManager, workdir))

	// 加载并启动 LSP server（多语言自动检测）
	lspManager := lsp.NewManager(workdir)
	lspManager.SetLogger(logger)
	lspManager.Start()

	// 注册 LSP 工具
	registry.Register(tools.NewLSPDefinitionTool(lspManager))
	registry.Register(tools.NewLSPReferencesTool(lspManager))
	registry.Register(tools.NewLSPHoverTool(lspManager))
	registry.Register(tools.NewLSPDiagnosticsTool(lspManager))
	registry.Register(tools.NewCodeIndexTool(lspManager))

	a, err := agent.NewAgent(buildConfig(cmd),
		agent.WithRegistry(registry),
		agent.WithChecker(checker),
		agent.WithHooks(hookRunner),
		agent.WithSkillStore(skillStore),
		agent.WithMemory(memSet),
		agent.WithJobManager(jobMgr),
		agent.WithSink(sink),
	)
	if err != nil {
		jobMgr.Close()
		return nil, err
	}
	a.WireTaskTool()
	a.WireSkillTools()
	a.WireMemoryTools()

	// 检测 worktree 状态并注入 system prompt 上下文
	wtInfo := agent.DetectWorktree(workdir)
	a.SetWorktreeContext(wtInfo)

	// 确保 .worktrees/ 在 .gitignore 中（安全守护）
	agent.EnsureWorktreeGitignore(workdir)

	cleanup := func() {
		lspManager.Stop()
		mcpManager.Stop()
		jobMgr.Close()
	}

	return &chatSetup{
		Agent:      a,
		SkillStore: skillStore,
		Registry:   registry,
		TuiSink:    tuiSink,
		MCPManager: mcpManager,
		LSPManager: lspManager,
		cleanup:    cleanup,
	}, nil
}
