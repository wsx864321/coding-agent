package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wsx864321/coding-agent/internal/memory"
)

func TestForgetTool(t *testing.T) {
	tmpDir := t.TempDir()
	store := memory.NewStore(memory.StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})
	queue := memory.NewQueue()

	tool := NewForgetTool(store, queue)
	if tool.Name() != "forget" {
		t.Errorf("Name = %q, want forget", tool.Name())
	}
	if tool.ReadOnly() {
		t.Error("forget should not be read-only")
	}
}

func TestForgetToolExecute(t *testing.T) {
	tmpDir := t.TempDir()
	store := memory.NewStore(memory.StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})
	queue := memory.NewQueue()

	// 先保存一条记忆
	store.Save(memory.Memory{
		Name:        "to-delete",
		Title:       "Delete Me",
		Description: "desc",
		Type:        memory.TypeUser,
		Body:        "body",
	})

	tool := NewForgetTool(store, queue)

	args := map[string]any{
		"name": "to-delete",
	}

	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(result, "to-delete") {
		t.Errorf("result should contain name: %s", result)
	}

	// 原文件应被删除
	if _, err := os.Stat(filepath.Join(store.GlobalDir, "to-delete.md")); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}

	// 归档目录应有文件
	archiveDir := filepath.Join(store.GlobalDir, ".archive")
	entries, _ := os.ReadDir(archiveDir)
	found := false
	for _, e := range entries {
		if strings.Contains(e.Name(), "to-delete.md") {
			found = true
		}
	}
	if !found {
		t.Error("archived file not found")
	}

	// queue 应有通知
	if !queue.Pending() {
		t.Error("queue should have pending notification after forget")
	}
}

func TestForgetToolExecuteNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := memory.NewStore(memory.StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})
	tool := NewForgetTool(store, nil)

	args := map[string]any{
		"name": "nonexistent",
	}

	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("Execute with nonexistent memory should return error")
	}
}

func TestForgetToolExecuteEmptyName(t *testing.T) {
	tmpDir := t.TempDir()
	store := memory.NewStore(memory.StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})
	tool := NewForgetTool(store, nil)

	args := map[string]any{
		"name": "",
	}

	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("Execute with empty name should return error")
	}
}
