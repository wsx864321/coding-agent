package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsValidSkillName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"code-review", true},
		{"my_skill", true},
		{"explore", true},
		{"Skill1", true},
		{"", false},
		{"123", false},         // 不以字母开头
		{"-bad", false},        // 不以字母开头
		{"help", false},        // 保留名
		{"reset", false},       // 保留名
		{"skills", false},      // 保留名
		{"a b c", false},       // 含空格
		{"foo/bar", false},     // 含路径分隔符
	}
	for _, tt := range tests {
		if got := IsValidSkillName(tt.name); got != tt.want {
			t.Errorf("IsValidSkillName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestStore_ScanAndList(t *testing.T) {
	// 创建临时目录结构
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".coding-agent", "skills", "test-skill")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := `---
name: test-skill
description: A test skill
runAs: inline
---

# Test Skill

Do something.`
	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewStore(StoreOptions{Workdir: tmpDir, HomeDir: t.TempDir()})
	skills := store.List()

	// 应该有 test-skill + builtin skill-creator
	found := false
	for _, sk := range skills {
		if sk.Name == "test-skill" {
			found = true
			if sk.Description != "A test skill" {
				t.Errorf("description = %q", sk.Description)
			}
			if sk.RunAs != RunInline {
				t.Errorf("runAs = %q, want inline", sk.RunAs)
			}
			if sk.Scope != ScopeProject {
				t.Errorf("scope = %q, want project", sk.Scope)
			}
		}
	}
	if !found {
		t.Errorf("test-skill not found in store; skills: %v", skills)
	}
}

func TestStore_Get(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(StoreOptions{Workdir: tmpDir, HomeDir: t.TempDir()})

	// builtin skill-creator 应该始终存在
	sk := store.Get("skill-creator")
	if sk == nil {
		t.Fatal("skill-creator not found")
	}
	if sk.Scope != ScopeBuiltin {
		t.Errorf("scope = %q, want builtin", sk.Scope)
	}

	// 不存在的 skill 返回 nil
	if store.Get("nonexistent") != nil {
		t.Error("expected nil for nonexistent skill")
	}
}

func TestStore_Install(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(StoreOptions{Workdir: tmpDir, HomeDir: t.TempDir()})

	content := `---
name: my-new-skill
description: A newly installed skill
---

# My New Skill

Instructions here.`

	path, err := store.Install("my-new-skill", content)
	if err != nil {
		t.Fatal(err)
	}

	// 验证文件已创建
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created: %v", err)
	}

	// 验证 store 已更新
	sk := store.Get("my-new-skill")
	if sk == nil {
		t.Fatal("installed skill not found in store")
	}
	if sk.Description != "A newly installed skill" {
		t.Errorf("description = %q", sk.Description)
	}
}

func TestStore_InstallInvalidName(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(StoreOptions{Workdir: tmpDir, HomeDir: t.TempDir()})

	_, err := store.Install("help", "content")
	if err == nil {
		t.Error("expected error for reserved name")
	}

	_, err = store.Install("../escape", "content")
	if err == nil {
		t.Error("expected error for invalid name")
	}
}

func TestStore_PriorityOverride(t *testing.T) {
	// project 级 skill 应覆盖 builtin
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".coding-agent", "skills", "skill-creator")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := `---
name: skill-creator
description: Custom override
---

# Custom skill-creator`
	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewStore(StoreOptions{Workdir: tmpDir, HomeDir: t.TempDir()})
	sk := store.Get("skill-creator")
	if sk == nil {
		t.Fatal("skill-creator not found")
	}
	if sk.Scope != ScopeProject {
		t.Errorf("expected project scope override, got %q", sk.Scope)
	}
	if sk.Description != "Custom override" {
		t.Errorf("description = %q, expected custom override", sk.Description)
	}
}

func TestStore_CompatDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// 在 .agents/skills/ 下创建 skill
	agentsDir := filepath.Join(tmpDir, ".agents", "skills", "compat-skill")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `---
name: compat-skill
description: From .agents dir
---

Body.`
	if err := os.WriteFile(filepath.Join(agentsDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewStore(StoreOptions{Workdir: tmpDir, HomeDir: t.TempDir()})
	sk := store.Get("compat-skill")
	if sk == nil {
		t.Fatal("compat-skill not found from .agents/skills/")
	}
	if sk.Description != "From .agents dir" {
		t.Errorf("description = %q", sk.Description)
	}
}

func TestStore_FlatLayout(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".coding-agent", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	content := `---
name: flat-skill
description: A flat layout skill
---

Flat body.`
	if err := os.WriteFile(filepath.Join(skillsDir, "flat-skill.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewStore(StoreOptions{Workdir: tmpDir, HomeDir: t.TempDir()})
	sk := store.Get("flat-skill")
	if sk == nil {
		t.Fatal("flat-skill not found")
	}
}
