package skill

import (
	"context"
	"strings"
	"testing"
)

func TestRunSkillTool_Inline(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(StoreOptions{Workdir: tmpDir, HomeDir: t.TempDir()})

	// builtin skill-creator 是 inline 的
	tool := NewRunSkillTool(store, nil)

	result, err := tool.Execute(context.Background(), map[string]any{
		"name": "skill-creator",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Skill: skill-creator") {
		t.Errorf("expected skill content, got %q", result[:min(len(result), 100)])
	}
}

func TestRunSkillTool_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(StoreOptions{Workdir: tmpDir, HomeDir: t.TempDir()})
	tool := NewRunSkillTool(store, nil)

	_, err := tool.Execute(context.Background(), map[string]any{
		"name": "nonexistent",
	})
	if err == nil {
		t.Error("expected error for missing skill")
	}
}

func TestRunSkillTool_EmptyName(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(StoreOptions{Workdir: tmpDir, HomeDir: t.TempDir()})
	tool := NewRunSkillTool(store, nil)

	_, err := tool.Execute(context.Background(), map[string]any{
		"name": "",
	})
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestRunSkillTool_SubagentNoRunner(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(StoreOptions{Workdir: tmpDir, HomeDir: t.TempDir()})

	// 手动注册一个 subagent skill
	store.mu.Lock()
	store.skills["sub-test"] = Skill{
		Name: "sub-test", Description: "test", Body: "body",
		RunAs: RunSubagent, Scope: ScopeProject,
	}
	store.mu.Unlock()

	tool := NewRunSkillTool(store, nil)
	_, err := tool.Execute(context.Background(), map[string]any{
		"name": "sub-test",
	})
	if err == nil || !strings.Contains(err.Error(), "runner 未配置") {
		t.Errorf("expected runner not configured error, got %v", err)
	}
}

func TestRunSkillTool_SubagentWithRunner(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(StoreOptions{Workdir: tmpDir, HomeDir: t.TempDir()})

	store.mu.Lock()
	store.skills["sub-test"] = Skill{
		Name: "sub-test", Description: "test", Body: "do something",
		RunAs: RunSubagent, Scope: ScopeProject,
	}
	store.mu.Unlock()

	runner := func(ctx context.Context, sk Skill, task string) (string, error) {
		return "subagent result for " + sk.Name, nil
	}
	tool := NewRunSkillTool(store, runner)

	result, err := tool.Execute(context.Background(), map[string]any{
		"name": "sub-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result != "subagent result for sub-test" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestInstallSkillTool_Success(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(StoreOptions{Workdir: tmpDir, HomeDir: t.TempDir()})
	tool := NewInstallSkillTool(store)

	content := "---\nname: new-skill\ndescription: Fresh skill\n---\n\nBody."
	result, err := tool.Execute(context.Background(), map[string]any{
		"name":    "new-skill",
		"content": content,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "已保存") {
		t.Errorf("expected success message, got %q", result)
	}

	// 验证可以查到
	sk := store.Get("new-skill")
	if sk == nil {
		t.Fatal("installed skill not found")
	}
}

func TestInstallSkillTool_EmptyName(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(StoreOptions{Workdir: tmpDir, HomeDir: t.TempDir()})
	tool := NewInstallSkillTool(store)

	_, err := tool.Execute(context.Background(), map[string]any{
		"name":    "",
		"content": "something",
	})
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
