package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wsx864321/coding-agent/internal/agent"
	"github.com/wsx864321/coding-agent/internal/hooks"
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

	hookRunner := hooks.NewRunner(
		hooks.Load(hooks.LoadOptions{ProjectRoot: workdir}),
		workdir,
		hooks.DefaultSpawner,
	)

	a, err := agent.NewAgent(buildConfig(cmd),
		agent.WithRegistry(registry),
		agent.WithChecker(checker),
		agent.WithHooks(hookRunner),
		agent.WithSkillStore(skillStore),
	)
	if err != nil {
		return err
	}
	a.WireTaskTool()
	a.WireSkillTools()

	if !onceQuiet {
		fmt.Fprintf(os.Stderr, "[coding-agent] running once, message=%q\n", truncate(onceMessage, 60))
	}

	out, err := a.Run(cmd.Context(), onceMessage)
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
