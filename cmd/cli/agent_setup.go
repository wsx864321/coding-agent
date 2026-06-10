package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/wsx864321/coding-agent/internal/agent"
)

// resolveWorkdir 从 --workdir flag 取值；空则用 cwd
//
// 小型 cobra 工具函数，仅在 cmd/cli 内用
func resolveWorkdir(cmd *cobra.Command) string {
	workdir, _ := cmd.Flags().GetString("workdir")
	if workdir == "" {
		workdir, _ = os.Getwd()
	}
	return workdir
}

// buildConfig 从 cmd flags 构造 agent.Config
//
// 注意：permission.Checker / hooks / client / registry 这类"装配期可选、运行期可替换"
// 的依赖已迁出 Config，由各子命令在 NewAgent 时通过 agent.WithXxx 注入
func buildConfig(cmd *cobra.Command) agent.Config {
	model, _ := cmd.Flags().GetString("model")
	baseURL, _ := cmd.Flags().GetString("base-url")
	maxTurns, _ := cmd.Flags().GetInt("max-turns")
	system, _ := cmd.Flags().GetString("system")

	return agent.Config{
		Model:        model,
		BaseURL:      baseURL,
		MaxTurns:     maxTurns,
		SystemPrompt: system,
	}
}
