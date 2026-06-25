package agent

import (
	"os"
	"path/filepath"
	"strings"
)

// EnsureWorktreeGitignore 检查并确保 .worktrees/ 在项目的 .gitignore 中。
// 仅在 git 仓库中生效。若 .gitignore 已包含 .worktrees/ 则跳过。
func EnsureWorktreeGitignore(workdir string) {
	// 非 git 仓库跳过
	if !isGitRepo(workdir) {
		return
	}

	giPath := filepath.Join(workdir, ".gitignore")
	data, err := os.ReadFile(giPath)
	if err != nil && !os.IsNotExist(err) {
		return // 读取失败静默跳过
	}

	content := string(data)
	if strings.Contains(content, ".worktrees") {
		return // 已存在
	}

	// 追加
	f, err := os.OpenFile(giPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	if len(data) > 0 && !strings.HasSuffix(content, "\n") {
		f.WriteString("\n")
	}
	f.WriteString("# git worktree 工作目录\n.worktrees/\n")
}

func isGitRepo(dir string) bool {
	return runGit(dir, "rev-parse", "--git-dir") != ""
}
