package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/wsx864321/coding-agent/internal/provider"
)

// setupTestGitRepo 创建一个临时 git 仓库并返回路径
func setupTestGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// 初始化 git 仓库
	runGitOrSkip(t, dir, "init", "-b", "main")
	runGitOrSkip(t, dir, "config", "user.email", "test@test.com")
	runGitOrSkip(t, dir, "config", "user.name", "Test")

	// 创建一个初始提交（worktree add 需要）
	path := filepath.Join(dir, "README.md")
	if err := os.WriteFile(path, []byte("# test"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitOrSkip(t, dir, "add", "README.md")
	runGitOrSkip(t, dir, "commit", "-m", "init")

	return dir
}

func runGitOrSkip(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git %v failed (git not installed?): %v\n%s", args, err, out)
	}
}

func TestDetectWorktree_NonGit(t *testing.T) {
	dir := t.TempDir()
	info := DetectWorktree(dir)
	if info.IsGitRepo {
		t.Error("non-git directory should have IsGitRepo=false")
	}
	if info.IsWorktree {
		t.Error("non-git directory should have IsWorktree=false")
	}
}

func TestDetectWorktree_NormalRepo(t *testing.T) {
	dir := setupTestGitRepo(t)
	info := DetectWorktree(dir)

	if !info.IsGitRepo {
		t.Error("git repo should have IsGitRepo=true")
	}
	if info.IsWorktree {
		t.Error("normal repo should not be detected as worktree")
	}
	if info.Branch != "main" {
		t.Errorf("branch: got %q, want main", info.Branch)
	}
}

func TestDetectWorktree_LinkedWorktree(t *testing.T) {
	dir := setupTestGitRepo(t)

	// 创建一个 linked worktree
	wtDir := filepath.Join(t.TempDir(), "wt")
	runGitOrSkip(t, dir, "worktree", "add", "-b", "feature/test", wtDir)

	info := DetectWorktree(wtDir)
	if !info.IsGitRepo {
		t.Error("worktree should have IsGitRepo=true")
	}
	if !info.IsWorktree {
		t.Error("linked worktree should be detected")
	}
	if info.Branch != "feature/test" {
		t.Errorf("branch: got %q, want feature/test", info.Branch)
	}
	// 规范化路径比较（git 和 TempDir 可能使用不同的路径分隔符）
	if filepath.Clean(info.Path) != filepath.Clean(wtDir) {
		t.Errorf("path: got %q, want %q", info.Path, wtDir)
	}
}

func TestDetectWorktree_DetachedHEAD(t *testing.T) {
	dir := setupTestGitRepo(t)

	// 创建 detached HEAD worktree
	wtDir := filepath.Join(t.TempDir(), "wt-detached")
	runGitOrSkip(t, dir, "worktree", "add", "--detach", wtDir)

	info := DetectWorktree(wtDir)
	if !info.IsWorktree {
		t.Error("detached worktree should be detected")
	}
	if info.Branch != "" {
		t.Errorf("branch should be empty for detached HEAD, got %q", info.Branch)
	}
}

func TestSystemPromptContext_NotWorktree(t *testing.T) {
	info := WorktreeInfo{IsWorktree: false}
	if ctx := info.SystemPromptContext(); ctx != "" {
		t.Errorf("non-worktree should return empty context, got: %q", ctx)
	}
}

func TestSystemPromptContext_Worktree(t *testing.T) {
	info := WorktreeInfo{
		IsWorktree: true,
		Path:       "/tmp/.worktrees/feature-x",
		Branch:     "feature/x",
	}
	ctx := info.SystemPromptContext()
	if ctx == "" {
		t.Error("worktree should return non-empty context")
	}
	// 关键信息应在上下文中
	for _, keyword := range []string{"隔离", "worktree", "/tmp/.worktrees/feature-x", "feature/x"} {
		if !contains(ctx, keyword) {
			t.Errorf("context should contain %q: %q", keyword, ctx)
		}
	}
}

func TestSystemPromptContext_Detached(t *testing.T) {
	info := WorktreeInfo{
		IsWorktree: true,
		Path:       "/tmp/.worktrees/detached",
		Branch:     "",
	}
	ctx := info.SystemPromptContext()
	if !contains(ctx, "detached") {
		t.Errorf("detached HEAD context should mention detached: %q", ctx)
	}
}

func TestSetWorktreeContext(t *testing.T) {
	// 构造一个最小化的 Agent 来测试 SetWorktreeContext
	a := &Agent{
		messages: []provider.Message{
			{Role: provider.RoleSystem, Content: "base system prompt"},
		},
	}

	info := WorktreeInfo{
		IsWorktree: true,
		Path:       "/tmp/.worktrees/test",
		Branch:     "test-branch",
	}
	a.SetWorktreeContext(info)

	if !contains(a.messages[0].Content, "隔离") {
		t.Errorf("system message should contain worktree context after SetWorktreeContext")
	}
	if !contains(a.messages[0].Content, "base system prompt") {
		t.Error("original content should be preserved")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
