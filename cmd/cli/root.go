// Package cli 提供 coding-agent 的 cobra 子命令注册
//
// 拆分原因：
//   - cobra 习惯把 rootCmd + 子命令 + flags 分到多个 .go 文件里
//   - 这里把 rootCmd 的 flag 注册和 Execute() 入口放到 root.go
//   - 各子命令（once / chat）单独成文件
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// rootCmd 是所有子命令的父命令
var rootCmd = &cobra.Command{
	Use:   "coding-agent",
	Short: "一个基于 OpenAI 兼容服务的 Coding Agent",
	Long: `coding-agent 是一个轻量级的 Coding Agent，集成 bash / read_file /
write_file / edit_file / glob_file 五个工具。

子命令：
  once  一次性对话
  chat  交互式 REPL`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	// 公共 flag 注册到 rootCmd，子命令会自动继承
	rootCmd.PersistentFlags().StringP("model", "M", "", "模型名（默认 gpt-4o-mini）")
	rootCmd.PersistentFlags().StringP("base-url", "u", "", "OpenAI 兼容服务 base URL（默认从环境变量 OPEN_BASE_URL）")
	rootCmd.PersistentFlags().IntP("max-turns", "t", 0, "Agent loop 最大轮数（默认 20）")
	rootCmd.PersistentFlags().StringP("system", "s", "", "自定义 system prompt（留空则按工具列表自动生成）")
	rootCmd.PersistentFlags().StringP("workdir", "w", "", "file 工具的白名单基准目录（默认当前工作目录）")
}

// Execute 是 CLI 入口，由 main() 调用
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
