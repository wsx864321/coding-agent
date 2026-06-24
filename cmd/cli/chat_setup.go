package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/wsx864321/coding-agent/internal/agent"
	"github.com/wsx864321/coding-agent/internal/event"
	"github.com/wsx864321/coding-agent/internal/hooks"
	"github.com/wsx864321/coding-agent/internal/jobs"
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
	cleanup    func()
}

// setupChatAgent 构造 chat/tui 共用的 agent 装配链；调用方须 defer setup.cleanup()。
func setupChatAgent(cmd *cobra.Command) (*chatSetup, error) {
	asker := &permission.StdinAsker{Reader: os.Stdin, Writer: os.Stderr}
	return setupAgentWithAsker(cmd, asker, nil)
}

// setupTuiAgent 为 TUI 构造 agent：通过 TuiSink 在 TUI 横幅中请求用户审批。
func setupTuiAgent(cmd *cobra.Command) (*chatSetup, error) {
	tuiSink := &tui.TuiSink{}
	asker := agent.SinkAsker{Sink: tuiSink}
	return setupAgentWithAsker(cmd, asker, tuiSink)
}

func setupAgentWithAsker(cmd *cobra.Command, asker permission.Asker, tuiSink *tui.TuiSink) (*chatSetup, error) {
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

	return &chatSetup{
		Agent:      a,
		SkillStore: skillStore,
		Registry:   registry,
		TuiSink:    tuiSink,
		cleanup:    jobMgr.Close,
	}, nil
}
