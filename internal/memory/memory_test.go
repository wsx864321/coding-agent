package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveUserDir(t *testing.T) {
	// 显式传入路径应原样返回
	if got := ResolveUserDir("/custom/path"); got != "/custom/path" {
		t.Errorf("ResolveUserDir explicit = %q, want %q", got, "/custom/path")
	}

	// 空字符串应返回默认路径
	got := ResolveUserDir("")
	if !strings.Contains(got, ".coding-agent") {
		t.Errorf("ResolveUserDir empty should contain .coding-agent, got %q", got)
	}
}

func TestDefaultMemoryDir(t *testing.T) {
	dir := DefaultMemoryDir()
	if !strings.Contains(dir, ".coding-agent") || !strings.Contains(dir, "memory") {
		t.Errorf("DefaultMemoryDir = %q, expected .coding-agent/memory in path", dir)
	}
}

func TestLoad(t *testing.T) {
	tmpDir := t.TempDir()
	userDir := filepath.Join(tmpDir, ".coding-agent")
	workDir := filepath.Join(tmpDir, "project")

	// 创建项目目录
	os.MkdirAll(workDir, 0o755)

	// 创建 AGENTS.md
	os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte("# Project Rules\nUse tabs."), 0o644)

	// 创建全局 AGENTS.md
	os.MkdirAll(userDir, 0o755)
	os.WriteFile(filepath.Join(userDir, "AGENTS.md"), []byte("# User Preferences\nPrefer Go."), 0o644)

	set := Load(Options{
		CWD:     workDir,
		UserDir: userDir,
		Workdir: workDir,
	})

	if set == nil {
		t.Fatal("Load returned nil")
	}
	if set.CWD != workDir {
		t.Errorf("CWD = %q, want %q", set.CWD, workDir)
	}
	// 应该有 User 级和 Project 级文档
	if len(set.Docs) < 1 {
		t.Errorf("expected at least 1 doc, got %d", len(set.Docs))
	}
	// Store 应已初始化
	if set.Store == nil {
		t.Fatal("Store is nil")
	}
}

func TestCompose(t *testing.T) {
	s := &Set{
		Docs: []Source{
			{Path: "/tmp/AGENTS.md", Scope: ScopeProject, Body: "# Rules"},
		},
		Index: "- [test](test.md) — test memory",
	}

	result := Compose("BASE PROMPT", s)
	if !strings.Contains(result, "BASE PROMPT") {
		t.Error("Compose missing base prompt")
	}
	if !strings.Contains(result, "# Rules") {
		t.Error("Compose missing doc body")
	}
	if !strings.Contains(result, "test memory") {
		t.Error("Compose missing memory index")
	}
	if !strings.Contains(result, "<doc scope=") {
		t.Error("Compose missing doc tag")
	}
	if !strings.Contains(result, "<memory-index>") {
		t.Error("Compose missing memory-index tag")
	}
}

func TestComposeEmpty(t *testing.T) {
	s := &Set{}
	result := Compose("BASE", s)
	if !strings.HasPrefix(result, "BASE") {
		t.Errorf("Compose with empty Set = %q", result)
	}
	if strings.Contains(result, "<doc") || strings.Contains(result, "<memory-index") {
		t.Error("Compose with empty Set should not have doc/memory tags")
	}
}

func TestBlockOnly(t *testing.T) {
	// nil set
	if got := BlockOnly(nil); got != "" {
		t.Errorf("BlockOnly nil = %q, want empty", got)
	}

	s := &Set{
		Index: "- [a](a.md) — desc",
	}
	got := BlockOnly(s)
	if !strings.Contains(got, "<memory-index>") {
		t.Error("BlockOnly missing memory-index")
	}
	if strings.Contains(got, "<doc") {
		t.Error("BlockOnly should not have doc tags when no docs")
	}
}
