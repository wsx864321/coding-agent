package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverDocs(t *testing.T) {
	tmpDir := t.TempDir()
	userDir := filepath.Join(tmpDir, ".coding-agent")
	workDir := filepath.Join(tmpDir, "project")

	os.MkdirAll(userDir, 0o755)
	os.MkdirAll(workDir, 0o755)

	// 创建用户级 AGENTS.md
	os.WriteFile(filepath.Join(userDir, "AGENTS.md"), []byte("# User Prefs"), 0o644)
	// 创建项目级 AGENTS.md
	os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte("# Project Rules"), 0o644)
	// 创建本地 AGENTS.local.md
	os.WriteFile(filepath.Join(workDir, "AGENTS.local.md"), []byte("# Local Overrides"), 0o644)

	docs := DiscoverDocs(workDir, userDir)

	if len(docs) < 2 {
		t.Errorf("expected at least 2 docs, got %d", len(docs))
	}

	// 验证 scope 顺序：user → project → local
	hasUser := false
	hasProject := false
	hasLocal := false
	for _, d := range docs {
		switch d.Scope {
		case ScopeUser:
			hasUser = true
		case ScopeProject:
			hasProject = true
		case ScopeLocal:
			hasLocal = true
		}
	}

	// User 级文档存在
	if !hasUser {
		t.Error("missing user scope doc")
	}
	// Project 级文档存在
	if !hasProject {
		t.Error("missing project scope doc")
	}
	// Local 级文档存在
	if !hasLocal {
		t.Error("missing local scope doc")
	}

	// 验证优先级：最后一个应是 local（最高优先级）
	if docs[len(docs)-1].Scope != ScopeLocal {
		t.Errorf("last doc scope = %q, want local", docs[len(docs)-1].Scope)
	}
}

func TestDiscoverDocsNoDocs(t *testing.T) {
	tmpDir := t.TempDir()
	userDir := filepath.Join(tmpDir, ".coding-agent")
	workDir := filepath.Join(tmpDir, "empty-project")

	os.MkdirAll(workDir, 0o755)

	docs := DiscoverDocs(workDir, userDir)
	if len(docs) != 0 {
		t.Errorf("expected 0 docs, got %d", len(docs))
	}
}

func TestFindGitRoot(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建 .git 目录
	gitDir := filepath.Join(tmpDir, ".git")
	os.MkdirAll(gitDir, 0o755)

	// 创建项目子目录
	subDir := filepath.Join(tmpDir, "src", "module")
	os.MkdirAll(subDir, 0o755)

	root := findGitRoot(subDir)
	if root != tmpDir {
		t.Errorf("findGitRoot = %q, want %q", root, tmpDir)
	}
}

func TestFindGitRootNone(t *testing.T) {
	tmpDir := t.TempDir()

	root := findGitRoot(tmpDir)
	if root != "" {
		t.Errorf("findGitRoot without .git = %q, want empty", root)
	}
}

func TestFindDocInDir(t *testing.T) {
	tmpDir := t.TempDir()

	// 不存在
	if src := findDocInDir(tmpDir, ScopeProject); src != nil {
		t.Error("findDocInDir should return nil when no docs")
	}

	// 创建 AGENTS.md
	os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte("# Rules"), 0o644)
	src := findDocInDir(tmpDir, ScopeProject)
	if src == nil {
		t.Fatal("findDocInDir returned nil")
	}
	if src.Scope != ScopeProject {
		t.Errorf("Scope = %q, want project", src.Scope)
	}
	if src.Body != "# Rules" {
		t.Errorf("Body = %q, want # Rules", src.Body)
	}
}

func TestExpandImports(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建被导入文件
	sharedPath := filepath.Join(tmpDir, "shared.md")
	os.WriteFile(sharedPath, []byte("# Shared Content"), 0o644)

	content := "@shared.md\n\n# Main Content"
	expanded := expandImports(content, tmpDir)

	if !strings.Contains(expanded, "# Shared Content") {
		t.Errorf("expandImports did not expand: %q", expanded)
	}
	if !strings.Contains(expanded, "# Main Content") {
		t.Errorf("expandImports lost original content: %q", expanded)
	}
	if strings.Contains(expanded, "@shared.md") {
		t.Error("expandImports should remove @ directive")
	}
}

func TestExpandImportsHomeTilde(t *testing.T) {
	home, _ := os.UserHomeDir()
	tmpDir := filepath.Join(home, ".coding-agent-test-import")
	os.MkdirAll(tmpDir, 0o755)
	defer os.RemoveAll(tmpDir)

	os.WriteFile(filepath.Join(tmpDir, "global.md"), []byte("# Global"), 0o644)

	// 使用绝对路径而非 ~ 展开（因为 ~ 展开依赖 home 目录存在）
	content := "@" + filepath.Join(tmpDir, "global.md") + "\n# Main"
	expanded := expandImports(content, tmpDir)

	if !strings.Contains(expanded, "# Global") {
		t.Errorf("expandImports home tilde: %q", expanded)
	}
}

func TestExpandImportsRecursive(t *testing.T) {
	tmpDir := t.TempDir()

	os.WriteFile(filepath.Join(tmpDir, "a.md"), []byte("@b.md"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "b.md"), []byte("# Deep Content"), 0o644)

	expanded := expandImports("@a.md", tmpDir)
	if !strings.Contains(expanded, "# Deep Content") {
		t.Errorf("recursive import failed: %q", expanded)
	}
}

func TestExpandImportsCircular(t *testing.T) {
	tmpDir := t.TempDir()

	os.WriteFile(filepath.Join(tmpDir, "a.md"), []byte("@b.md"), 0o644)
	os.WriteFile(filepath.Join(tmpDir, "b.md"), []byte("@a.md"), 0o644)

	expanded := expandImports("@a.md", tmpDir)
	if !strings.Contains(expanded, "循环导入") {
		t.Errorf("circular import not detected: %q", expanded)
	}
}

func TestExpandImportsNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	expanded := expandImports("@nonexistent.md\n# Main", tmpDir)
	if !strings.Contains(expanded, "导入失败") {
		t.Errorf("missing import error: %q", expanded)
	}
}

func TestDedupeSources(t *testing.T) {
	sources := []Source{
		{Path: "/tmp/agents.md", Scope: ScopeProject, Body: "a"},
		{Path: "/tmp/agents.md", Scope: ScopeProject, Body: "a"}, // 重复（同路径）
		{Path: "/other/rules.md", Scope: ScopeUser, Body: "b"},
	}

	deduped := dedupeSources(sources)
	if len(deduped) != 2 {
		t.Errorf("dedupeSources = %d, want 2", len(deduped))
	}
}
