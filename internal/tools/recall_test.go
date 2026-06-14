package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/wsx864321/coding-agent/internal/memory"
)

func TestRecallTool(t *testing.T) {
	tmpDir := t.TempDir()
	store := memory.NewStore(memory.StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})
	tool := NewRecallTool(store)

	if tool.Name() != "recall" {
		t.Errorf("Name = %q, want recall", tool.Name())
	}
	if !tool.ReadOnly() {
		t.Error("recall should be read-only")
	}
}

func TestRecallToolList(t *testing.T) {
	tmpDir := t.TempDir()
	store := memory.NewStore(memory.StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})

	// 准备数据
	store.Save(memory.Memory{
		Name: "prefers-tabs", Title: "Tabs", Description: "prefers tabs",
		Type: memory.TypeUser, Body: "use tabs",
	})
	store.Save(memory.Memory{
		Name: "build-cmd", Title: "Build", Description: "build command",
		Type: memory.TypeProject, Body: "go build",
	})

	tool := NewRecallTool(store)

	// list all
	args := map[string]any{"action": "list"}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(result, "Tabs") {
		t.Error("list missing title")
	}
	if !strings.Contains(result, "Build") {
		t.Error("list missing second memory")
	}

	// list filtered
	args["type"] = "user"
	result, err = tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("list filtered failed: %v", err)
	}
	if !strings.Contains(result, "Tabs") {
		t.Error("filtered list should contain user memory")
	}
}

func TestRecallToolRead(t *testing.T) {
	tmpDir := t.TempDir()
	store := memory.NewStore(memory.StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})

	store.Save(memory.Memory{
		Name: "test-mem", Title: "Test", Description: "desc",
		Type: memory.TypeUser, Body: "Full body content here.",
	})

	tool := NewRecallTool(store)

	args := map[string]any{
		"action": "read",
		"query":  "test-mem",
	}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !strings.Contains(result, "Full body content here.") {
		t.Errorf("read missing body: %s", result)
	}
}

func TestRecallToolReadNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := memory.NewStore(memory.StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})
	tool := NewRecallTool(store)

	args := map[string]any{
		"action": "read",
		"query":  "nonexistent",
	}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("read nonexistent should return error")
	}
}

func TestRecallToolSearch(t *testing.T) {
	tmpDir := t.TempDir()
	store := memory.NewStore(memory.StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})

	store.Save(memory.Memory{
		Name: "tabs-pref", Title: "Tabs", Description: "tab indentation",
		Type: memory.TypeUser, Body: "User prefers using tabs for indentation in all projects.",
	})
	store.Save(memory.Memory{
		Name: "build-cmd", Title: "Build", Description: "go build command",
		Type: memory.TypeProject, Body: "Use go build ./cmd/server to compile.",
	})

	tool := NewRecallTool(store)

	args := map[string]any{
		"action": "search",
		"query":  "tabs indentation",
	}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	// 搜索 "tabs" 应该首先命中 tabs-pref
	if !strings.Contains(result, "Tabs") {
		t.Errorf("search result should contain Tabs: %s", result)
	}
}

func TestRecallToolSearchEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	store := memory.NewStore(memory.StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})
	tool := NewRecallTool(store)

	args := map[string]any{
		"action": "search",
		"query":  "nothing",
	}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("search empty failed: %v", err)
	}
	if !strings.Contains(result, "没有") {
		t.Errorf("empty search should indicate no results: %s", result)
	}
}

func TestRecallToolSearchNoQuery(t *testing.T) {
	tmpDir := t.TempDir()
	store := memory.NewStore(memory.StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})
	tool := NewRecallTool(store)

	args := map[string]any{
		"action": "search",
	}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("search without query should return error")
	}
}

func TestRecallToolInvalidAction(t *testing.T) {
	tmpDir := t.TempDir()
	store := memory.NewStore(memory.StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})
	tool := NewRecallTool(store)

	args := map[string]any{
		"action": "invalid",
	}
	_, err := tool.Execute(context.Background(), args)
	if err == nil {
		t.Error("invalid action should return error")
	}
}

func TestRecallToolListEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	store := memory.NewStore(memory.StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})
	tool := NewRecallTool(store)

	args := map[string]any{"action": "list"}
	result, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("list empty failed: %v", err)
	}
	if !strings.Contains(result, "没有") {
		t.Errorf("empty list should indicate no results: %s", result)
	}
}
