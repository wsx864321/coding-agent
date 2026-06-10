package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wsx864321/coding-agent/internal/agent"
	"github.com/wsx864321/coding-agent/internal/hooks/builtin"
	"github.com/wsx864321/coding-agent/internal/permission"
	"github.com/wsx864321/coding-agent/internal/tools"
)

// onceCmd 一次性对话子命令
//
// 用法：
//   coding-agent once -m "请总结 main.go"
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

	// 系统级权限：仅保留硬拒绝（bash deny 列表），保证 hook "放水" 也不能绕过
	// Ask 阶段在 once 模式下不装配（无 TTY、无 Asker）
	checker := &permission.Pipeline{
		Deny: []permission.Checker{
			&permission.DenyListChecker{Patterns: permission.DefaultBashDenyList()},
		},
	}

	// asker=nil：once 模式无 TTY，Ask 视为 Allow
	a, err := agent.NewAgent(buildConfig(cmd),
		agent.WithRegistry(registry),
		agent.WithChecker(checker),
		agent.WithHooks(builtin.NewDefault(workdir, nil, os.Stderr)),
	)
	if err != nil {
		return err
	}

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
