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
  chat  交互式 REPL
  tui   Bubble Tea 交互式聊天`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	// 公共 flag 注册到 rootCmd，子命令会自动继承
	rootCmd.PersistentFlags().StringP("provider", "P", "", "provider 类型：openai 或 anthropic（默认 openai，可通过 PROVIDER_KIND 环境变量设置）")
	rootCmd.PersistentFlags().StringP("model", "M", "", "模型名（默认按 provider 类型选择）")
	rootCmd.PersistentFlags().StringP("base-url", "u", "", "API base URL（默认从环境变量读取）")
	rootCmd.PersistentFlags().IntP("max-turns", "t", 0, "Agent loop 最大轮数（默认 20）")
	rootCmd.PersistentFlags().StringP("system", "s", "", "自定义 system prompt（留空则按工具列表自动生成）")
	rootCmd.PersistentFlags().StringP("workdir", "w", "", "file 工具的白名单基准目录（默认当前工作目录）")
	rootCmd.PersistentFlags().Int("context-window", 0, "上下文窗口 token 上限；<=0 关闭 context compact")
	rootCmd.PersistentFlags().Float64("soft-compact-ratio", 0.50, "软阈值（仅提示，不触发摘要）")
	rootCmd.PersistentFlags().Float64("compact-ratio", 0.80, "自动摘要压缩触发阈值")
	rootCmd.PersistentFlags().Float64("compact-force-ratio", 0.90, "强制压缩阈值（跳过低价值折叠判断）")
	rootCmd.PersistentFlags().Int("recent-keep", 3, "压缩时最少保留的最近消息下限")
	rootCmd.PersistentFlags().Int("max-messages-snip", 80, "snip_compact 消息数阈值；<=0 关闭")
	rootCmd.PersistentFlags().String("archive-dir", "", "压缩归档目录（默认 .transcripts）")
}

// Execute 是 CLI 入口，由 main() 调用
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
