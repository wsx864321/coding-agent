package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/wsx864321/coding-agent/internal/agent"
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

	// 基准目录：从 flag 读取，留空走当前目录
	workdir, _ := cmd.Flags().GetString("workdir")
	if workdir == "" {
		workdir, _ = os.Getwd()
	}

	// bash 不默认限制 workdir（按设计）；调用方按需设置 AllowedDirs
	registry.Register(tools.NewBashTool(workdir))

	// file 系列工具以 workdir 作为白名单基准
	registry.Register(tools.NewReadFileTool(workdir))
	registry.Register(tools.NewWriteFileTool(workdir))
	registry.Register(tools.NewEditFileTool(workdir))
	registry.Register(tools.NewGlobFileTool(workdir))

	model, _ := cmd.Flags().GetString("model")
	baseURL, _ := cmd.Flags().GetString("base-url")
	maxTurns, _ := cmd.Flags().GetInt("max-turns")
	system, _ := cmd.Flags().GetString("system")

	// s03: 构造三道闸门权限管线
	//   Gate 1: bash 硬拒绝列表
	//   Gate 2: bash 破坏性关键字 / 写入工作区外
	//   Gate 3: once 模式无 TTY，按 auto-deny：只走 deny 与 ask-no-op，规则命中会被忽略
	//           后续若需要 once 也走交互，可加 --interactive flag
	checker := &permission.Pipeline{
		Deny: []permission.Checker{
			&permission.DenyListChecker{Patterns: permission.DefaultBashDenyList()},
		},
		Ask: []permission.Checker{
			&permission.AskRuleChecker{Rules: []permission.AskRule{
				permission.DefaultBashAskRules(),
				permission.WriteOutsideWorkdirRule(workdir),
			}},
		},
		// Asker 留 nil：once 模式下 Ask 视作 Allow（见 Pipeline.Check 注释）
	}

	cfg := agent.Config{
		Model:        model,
		BaseURL:      baseURL,
		MaxTurns:     maxTurns,
		SystemPrompt: system,
		Checker:      checker,
	}
	a, err := agent.NewAgent(cfg, registry)
	if err != nil {
		return nil, nil, err
	}
	return a, registry, nil
}
