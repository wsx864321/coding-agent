package cli

import (
	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"github.com/wsx864321/coding-agent/internal/agent"
	"github.com/wsx864321/coding-agent/internal/tui"
)

// tuiCmd Bubble Tea 交互式聊天子命令
var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Bubble Tea 交互式聊天",
	Long: `启动 Bubble Tea 全屏 TUI，提供消息流与输入区交互。

继承 root 持久 flags（--provider、--model、--workdir 等），与 chat 共用 agent 装配逻辑。`,
	RunE: runTui,
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}

func runTui(cmd *cobra.Command, args []string) error {
	setup, err := setupTuiAgent(cmd)
	if err != nil {
		return err
	}
	defer setup.cleanup()

	cfg := buildConfig(cmd)
	workdir := resolveWorkdir(cmd)
	sessionBucket := agent.SessionBucket(agent.ResolveSessionDir(cfg.SessionDir), workdir)
	setup.Agent.SetSessionPath(agent.NewSessionPath(sessionBucket, cfg.Model))

	m := tui.NewWithRunner(newAgentRunner(setup.Agent), setup.TuiSink)
	m.SetSlashCommands(defaultSlashCommands())
	m.SetModelName(cfg.Model)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}

// defaultSlashCommands 返回 TUI 中可用的斜杠命令列表。
func defaultSlashCommands() []string {
	return []string{
		"/help", "/skills", "/model", "/clear", "/reset",
		"/exit", "/quit", "/history", "/tools", "/hooks",
		"/compact", "/jobs", "/diff-fold",
	}
}
