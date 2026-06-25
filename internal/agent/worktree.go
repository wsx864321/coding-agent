package agent

import (
	"os/exec"
	"strings"

	"github.com/wsx864321/coding-agent/internal/provider"
)

// WorktreeInfo 描述当前 git worktree 的状态
type WorktreeInfo struct {
	IsWorktree bool   // 是否在 linked worktree 中
	IsGitRepo  bool   // 是否在 git 仓库中
	Path       string // worktree 路径（IsWorktree 为 true 时有值）
	Branch     string // 当前分支名；detached HEAD 时为空
	GitDir     string // .git 目录路径
	GitCommon  string // 共享的 .git 目录路径
}

// DetectWorktree 检测 workdir 下的 git worktree 状态
//
// 对应 using-git-worktrees Skill 的 Step 0 检测逻辑。
func DetectWorktree(workdir string) WorktreeInfo {
	info := WorktreeInfo{}

	gitDir := runGit(workdir, "rev-parse", "--git-dir")
	if gitDir == "" {
		return info // 非 git 仓库
	}
	info.IsGitRepo = true
	info.GitDir = gitDir

	gitCommon := runGit(workdir, "rev-parse", "--git-common-dir")
	info.GitCommon = gitCommon

	// 检查是否在 submodule 中（submodule 的 GIT_DIR != GIT_COMMON 但不是 worktree）
	superProject := runGit(workdir, "rev-parse", "--show-superproject-working-tree")

	if gitDir != gitCommon && gitCommon != "" && superProject == "" {
		info.IsWorktree = true
	}

	// 获取分支名
	branch := runGit(workdir, "branch", "--show-current")
	info.Branch = strings.TrimSpace(branch)

	// 获取 worktree 路径
	if info.IsWorktree {
		info.Path = runGit(workdir, "rev-parse", "--show-toplevel")
	}

	return info
}

// runGit 在指定目录下执行 git 命令，返回去除尾部换行的 stdout；失败返回空字符串
func runGit(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// SystemPromptContext 为 system prompt 生成 worktree 上下文提示
func (w WorktreeInfo) SystemPromptContext() string {
	if !w.IsWorktree {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n## Git Worktree 上下文\n\n")
	b.WriteString("你当前在隔离的 git worktree 中工作：\n")

	if w.Path != "" {
		b.WriteString("- 路径: `" + w.Path + "`\n")
	}
	if w.Branch != "" {
		b.WriteString("- 分支: `" + w.Branch + "`\n")
	} else {
		b.WriteString("- HEAD: detached（无分支，修改无法直接 push）\n")
	}
	b.WriteString("\n你的文件操作应在此 worktree 内进行，不要修改主仓库或其他 worktree 的文件。\n")

	return b.String()
}

// SetWorktreeContext 将 worktree 信息注入到第一个 system message 中
func (a *Agent) SetWorktreeContext(info WorktreeInfo) {
	ctx := info.SystemPromptContext()
	if ctx == "" || len(a.messages) == 0 {
		return
	}

	// 修改第一个 system message
	if a.messages[0].Role == provider.RoleSystem {
		a.messages[0].Content += ctx
		// 同步更新 cfg 中的 SystemPrompt 以便保存 session 时一致
		a.cfg.SystemPrompt += ctx
	}
}
