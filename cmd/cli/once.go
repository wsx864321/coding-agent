package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/wsx864321/coding-agent/internal/agent"
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
	a, _, err := buildAgent(cmd)
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

// buildAgent 根据 cmd 的 flags 构造 Agent + 工具注册表
func buildAgent(cmd *cobra.Command) (*agent.Agent, *tools.Registry, error) {
	registry := tools.NewRegistry()

	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, fmt.Errorf("获取当前工作目录失败: %w", err)
	}

	bash := tools.NewBashTool()
	bash.AllowedDirs = []string{cwd}
	registry.Register(bash)

	registry.Register(tools.NewReadFileTool())
	registry.Register(tools.NewWriteFileTool())
	registry.Register(tools.NewEditFileTool())
	registry.Register(tools.NewGlobFileTool())

	model, _ := cmd.Flags().GetString("model")
	baseURL, _ := cmd.Flags().GetString("base-url")
	maxTurns, _ := cmd.Flags().GetInt("max-turns")
	system, _ := cmd.Flags().GetString("system")

	cfg := agent.Config{
		Model:        model,
		BaseURL:      baseURL,
		MaxTurns:     maxTurns,
		SystemPrompt: system,
	}
	a, err := agent.NewAgent(cfg, registry)
	if err != nil {
		return nil, nil, err
	}
	return a, registry, nil
}
