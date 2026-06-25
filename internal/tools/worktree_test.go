package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, dir, "init", "-b", "main")
	mustGit(t, dir, "config", "user.email", "test@test.com")
	mustGit(t, dir, "config", "user.name", "Test")
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0o644)
	mustGit(t, dir, "add", "README.md")
	mustGit(t, dir, "commit", "-m", "init")
	return dir
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git %v skipped (git may not be available): %v\n%s", args, err, out)
	}
}

func TestWorktreeTool_Name(t *testing.T) {
	tool := NewWorktreeTool("")
	if tool.Name() != "worktree" {
		t.Errorf("Name: got %q", tool.Name())
	}
}

func TestWorktreeTool_Schema(t *testing.T) {
	tool := NewWorktreeTool("")
	schema := string(tool.Schema())
	if !strings.Contains(schema, "create") {
		t.Error("schema should contain 'create'")
	}
	if !strings.Contains(schema, "list") {
		t.Error("schema should contain 'list'")
	}
	if !strings.Contains(schema, "remove") {
		t.Error("schema should contain 'remove'")
	}
}

func TestWorktreeTool_InvalidOp(t *testing.T) {
	tool := NewWorktreeTool("")
	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{"op": "invalid"})
	if err == nil {
		t.Error("expected error for invalid op")
	}
}

func TestWorktreeTool_List(t *testing.T) {
	dir := setupGitRepo(t)
	tool := NewWorktreeTool(dir)

	ctx := context.Background()
	result, err := tool.Execute(ctx, map[string]any{"op": "list"})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(result, "worktree") {
		t.Errorf("list result should mention 'worktree': %q", result)
	}
}

func TestWorktreeTool_CreateAndRemove(t *testing.T) {
	dir := setupGitRepo(t)
	tool := NewWorktreeTool(dir)

	ctx := context.Background()

	// 创建 worktree
	result, err := tool.Execute(ctx, map[string]any{
		"op":     "create",
		"branch": "feature/worktree-test",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if !strings.Contains(result, "创建成功") {
		t.Errorf("create result should say 创建成功: %q", result)
	}

	// 验证 .worktrees/ 目录存在
	wtDir := filepath.Join(dir, ".worktrees", "feature", "worktree-test")
	if _, err := os.Stat(wtDir); os.IsNotExist(err) {
		t.Errorf("worktree directory not created at %s", wtDir)
	}

	// 验证 .gitignore 已更新
	giPath := filepath.Join(dir, ".gitignore")
	data, _ := os.ReadFile(giPath)
	if !strings.Contains(string(data), ".worktrees") {
		t.Error(".gitignore should contain .worktrees/")
	}

	// 列出 worktree
	listResult, err := tool.Execute(ctx, map[string]any{"op": "list"})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(listResult, "feature/worktree-test") {
		t.Errorf("list should include the new worktree: %q", listResult)
	}

	// 删除 worktree
	removeResult, err := tool.Execute(ctx, map[string]any{
		"op":   "remove",
		"path": wtDir,
	})
	if err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if !strings.Contains(removeResult, "已删除") {
		t.Errorf("remove result should say 已删除: %q", removeResult)
	}
}

func TestWorktreeTool_RemoveMissingPath(t *testing.T) {
	dir := setupGitRepo(t)
	tool := NewWorktreeTool(dir)

	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{"op": "remove"})
	if err == nil {
		t.Error("expected error for missing path")
	}
}

func TestWorktreeTool_NonGitRepo(t *testing.T) {
	dir := t.TempDir()
	tool := NewWorktreeTool(dir)

	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{
		"op":     "create",
		"branch": "test",
	})
	if err == nil {
		t.Error("expected error for non-git directory")
	}
}

func TestParsePorcelain(t *testing.T) {
	input := `worktree /path/to/main
bare

worktree /path/to/feature
HEAD abc123
branch refs/heads/feature-x

worktree /path/to/detached
HEAD def456
`

	wts := parsePorcelain(input)
	if len(wts) != 3 {
		t.Fatalf("expected 3 worktrees, got %d", len(wts))
	}

	if !wts[0].Bare {
		t.Error("first worktree should be bare")
	}
	if wts[1].Branch != "feature-x" {
		t.Errorf("branch: got %q", wts[1].Branch)
	}
	if wts[1].Path != "/path/to/feature" {
		t.Errorf("path: got %q", wts[1].Path)
	}
	if !wts[2].HEAD {
		t.Error("third worktree should have HEAD")
	}
	if wts[2].Branch != "" {
		t.Errorf("third worktree should have empty branch (detached)")
	}
}

func TestEnsureGitignore(t *testing.T) {
	dir := t.TempDir()
	tool := NewWorktreeTool(dir)

	if err := tool.ensureGitignore(); err != nil {
		t.Fatal(err)
	}

	// 检查文件已创建
	giPath := filepath.Join(dir, ".gitignore")
	data, err := os.ReadFile(giPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), ".worktrees") {
		t.Error(".gitignore should contain .worktrees/")
	}

	// 再次调用应该是幂等的
	if err := tool.ensureGitignore(); err != nil {
		t.Fatal(err)
	}
	data2, _ := os.ReadFile(giPath)
	if strings.Count(string(data2), ".worktrees") > 1 {
		t.Error(".worktrees/ should not appear twice")
	}
}

func TestIsGitRepo(t *testing.T) {
	if isGitRepo(t.TempDir()) {
		t.Error("temp dir should not be a git repo")
	}
	dir := setupGitRepo(t)
	if !isGitRepo(dir) {
		t.Error("should detect git repo")
	}
}

func TestWorktreeTool_ReadOnly(t *testing.T) {
	tool := NewWorktreeTool("")
	if tool.ReadOnly() {
		t.Error("worktree tool should not be read-only")
	}
}
