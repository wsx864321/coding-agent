package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wsx864321/coding-agent/internal/memory"
)

func TestRememberTool(t *testing.T) {
	tmpDir := t.TempDir()
	store := memory.NewStore(memory.StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})
	queue := memory.NewQueue()

	tool := NewRememberTool(store, queue)
	if tool.Name() != "remember" {
		t.Errorf("Name = %q, want remember", tool.Name())
	}
	if tool.ReadOnly() {
		t.Error("remember should not be read-only")
	}
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}
	if len(tool.Schema()) == 0 {
		t.Error("Schema should not be empty")
	}
}

func TestRememberToolExecute(t *testing.T) {
	tmpDir := t.TempDir()
	store := memory.NewStore(memory.StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})
	queue := memory.NewQueue()

	tool := NewRememberTool(store, queue)

	args := map[string]any{
		"name":        "prefers-tabs",
		"title":       "Prefers tabs",
		"description": "User prefers tabs over spaces",
		"type":        "user",
		"body":        "Always use tabs.\n**Why:** Consistency.",
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(result, "prefers-tabs") {
		t.Errorf("result should contain name: %s", result)
	}

	// 验证文件写入
	path := filepath.Join(store.GlobalDir, "prefers-tabs.md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("memory file not created at %s", path)
	}

	// 验证 queue 有通知
	if !queue.Pending() {
		t.Error("queue should have pending notification")
	}

	// 验证 Load
	loaded, err := store.Load("prefers-tabs")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Type != memory.TypeUser {
		t.Errorf("Type = %q, want user", loaded.Type)
	}
}

func TestRememberToolExecuteInvalidType(t *testing.T) {
	tmpDir := t.TempDir()
	store := memory.NewStore(memory.StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})
	tool := NewRememberTool(store, nil)

	args := map[string]any{
		"name":        "test",
		"title":       "Test",
		"description": "desc",
		"type":        "invalid-type",
		"body":        "body",
	}

	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("Execute with invalid type should return error")
	}
}

func TestRememberToolExecuteEmptyName(t *testing.T) {
	tmpDir := t.TempDir()
	store := memory.NewStore(memory.StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})
	tool := NewRememberTool(store, nil)

	args := map[string]any{
		"name":        "",
		"title":       "Test",
		"description": "desc",
		"type":        "user",
		"body":        "body",
	}

	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("Execute with empty name should return error")
	}
}

func TestRememberToolExecuteNilQueue(t *testing.T) {
	tmpDir := t.TempDir()
	store := memory.NewStore(memory.StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})
	tool := NewRememberTool(store, nil) // nil queue

	args := map[string]any{
		"name":        "nil-queue-test",
		"title":       "Test",
		"description": "desc",
		"type":        "project",
		"body":        "body",
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute with nil queue failed: %v", err)
	}
	if !strings.Contains(result, "nil-queue-test") {
		t.Errorf("unexpected result: %s", result)
	}
}

func TestToSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Prefers Tabs", "prefers-tabs"},
		{"Build Command!!!", "build-command"},
		{"  Spaces  ", "spaces"},
		{"", "memory"}, // 空输入回退
		{"already-slug", "already-slug"},
		{"中文记忆", "中文记忆"}, // CJK 不转 slug（只有 ASCII 转）
	}

	for _, tt := range tests {
		got := toSlug(tt.input)
		if got != tt.want {
			t.Errorf("toSlug(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
