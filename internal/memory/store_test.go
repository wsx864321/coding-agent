package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewStore(t *testing.T) {
	tmpDir := t.TempDir()
	userDir := filepath.Join(tmpDir, ".coding-agent")
	workDir := filepath.Join(tmpDir, "project")

	s := NewStore(StoreOptions{
		CWD:     workDir,
		Workdir: workDir,
		UserDir: userDir,
	})

	if s.Dir == "" {
		t.Error("Dir should not be empty")
	}
	if s.GlobalDir == "" {
		t.Error("GlobalDir should not be empty")
	}
	if !strings.Contains(s.Dir, "projects") || !strings.Contains(s.Dir, "memory") {
		t.Errorf("Dir should contain projects/memory: %q", s.Dir)
	}
	if !strings.Contains(s.GlobalDir, "memory") || !strings.Contains(s.GlobalDir, "global") {
		t.Errorf("GlobalDir should contain memory/global: %q", s.GlobalDir)
	}
}

func TestDirFor(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})

	tests := []struct {
		typ   Type
		isDir func(string) bool
	}{
		{TypeUser, func(d string) bool { return d == s.GlobalDir }},
		{TypeFeedback, func(d string) bool { return d == s.GlobalDir }},
		{TypeProject, func(d string) bool { return d == s.Dir }},
		{TypeReference, func(d string) bool { return d == s.Dir }},
	}

	for _, tt := range tests {
		if got := s.DirFor(tt.typ); !tt.isDir(got) {
			t.Errorf("DirFor(%s) = %q", tt.typ, got)
		}
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})

	m := Memory{
		Name:        "prefers-tabs",
		Title:       "Prefers tabs",
		Description: "User prefers tabs for indentation",
		Type:        TypeUser,
		Body:        "Always use tabs.\n**Why:** Consistency.",
	}

	if err := s.Save(m); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// 检查文件是否存在
	path := filepath.Join(s.GlobalDir, "prefers-tabs.md")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("memory file not found at %s", path)
	}

	// Load
	loaded, err := s.Load("prefers-tabs")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Name != m.Name {
		t.Errorf("Name = %q, want %q", loaded.Name, m.Name)
	}
	if loaded.Title != m.Title {
		t.Errorf("Title = %q, want %q", loaded.Title, m.Title)
	}
	if loaded.Type != m.Type {
		t.Errorf("Type = %q, want %q", loaded.Type, m.Type)
	}
	if loaded.Body != m.Body {
		t.Errorf("Body = %q, want %q", loaded.Body, m.Body)
	}
}

func TestSaveOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})

	m1 := Memory{
		Name:        "test",
		Title:       "V1",
		Description: "first",
		Type:        TypeUser,
		Body:        "body1",
	}
	m2 := Memory{
		Name:        "test",
		Title:       "V2",
		Description: "second",
		Type:        TypeUser,
		Body:        "body2",
	}

	s.Save(m1)
	s.Save(m2)

	loaded, _ := s.Load("test")
	if loaded.Title != "V2" {
		t.Errorf("after overwrite Title = %q, want V2", loaded.Title)
	}
}

func TestSaveTypeMigration(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})

	// 先存为 user（global）
	m := Memory{
		Name:        "migrate-test",
		Title:       "Test",
		Description: "desc",
		Type:        TypeUser,
		Body:        "body",
	}
	s.Save(m)

	// 确认在 global 目录
	if _, err := os.Stat(filepath.Join(s.GlobalDir, "migrate-test.md")); err != nil {
		t.Errorf("initial save not in global dir: %v", err)
	}

	// 改为 project 类型
	m.Type = TypeProject
	s.Save(m)

	// 确认在 project 目录
	if _, err := os.Stat(filepath.Join(s.Dir, "migrate-test.md")); err != nil {
		t.Errorf("migrated file not in project dir: %v", err)
	}
	// 确认 global 的旧文件已归档
	archiveDir := filepath.Join(s.GlobalDir, ".archive")
	entries, _ := os.ReadDir(archiveDir)
	found := false
	for _, e := range entries {
		if strings.Contains(e.Name(), "migrate-test.md") {
			found = true
		}
	}
	if !found {
		t.Error("old file not archived after type migration")
	}
}

func TestDelete(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})

	m := Memory{
		Name:        "to-delete",
		Title:       "Delete Me",
		Description: "will be deleted",
		Type:        TypeUser,
		Body:        "body",
	}
	s.Save(m)

	if err := s.Delete("to-delete"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// 原文件应不存在
	if _, err := os.Stat(filepath.Join(s.GlobalDir, "to-delete.md")); !os.IsNotExist(err) {
		t.Error("file should be deleted from active dir")
	}

	// 归档目录应有该文件
	archiveDir := filepath.Join(s.GlobalDir, ".archive")
	entries, _ := os.ReadDir(archiveDir)
	found := false
	for _, e := range entries {
		if strings.Contains(e.Name(), "to-delete.md") {
			found = true
		}
	}
	if !found {
		t.Error("deleted file not found in archive")
	}

	// Load 应失败
	if _, err := s.Load("to-delete"); err == nil {
		t.Error("Load should fail after Delete")
	}
}

func TestDeleteNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})

	err := s.Delete("nonexistent")
	if err == nil {
		t.Error("Delete nonexistent should return error")
	}
}

func TestListActive(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})

	s.Save(Memory{Name: "u1", Title: "U1", Description: "u", Type: TypeUser, Body: "u"})
	s.Save(Memory{Name: "p1", Title: "P1", Description: "p", Type: TypeProject, Body: "p"})
	s.Save(Memory{Name: "u2", Title: "U2", Description: "u2", Type: TypeUser, Body: "u2"})

	// 全部列出
	all := s.ListActive("")
	if len(all) != 3 {
		t.Errorf("ListActive all = %d, want 3", len(all))
	}

	// 按类型过滤
	users := s.ListActive(TypeUser)
	if len(users) != 2 {
		t.Errorf("ListActive user = %d, want 2", len(users))
	}

	projects := s.ListActive(TypeProject)
	if len(projects) != 1 {
		t.Errorf("ListActive project = %d, want 1", len(projects))
	}

	// 排序
	if len(all) >= 2 && all[0].Name > all[1].Name {
		t.Error("ListActive should be sorted by name")
	}
}

func TestIndex(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(StoreOptions{CWD: tmpDir, Workdir: tmpDir, UserDir: tmpDir})

	// 空索引
	if s.Index() != "" {
		t.Errorf("empty Index = %q, want empty", s.Index())
	}

	s.Save(Memory{Name: "test", Title: "Test", Description: "a test", Type: TypeUser, Body: "b"})

	idx := s.Index()
	if !strings.Contains(idx, "Test") {
		t.Error("Index should contain title")
	}
	if !strings.Contains(idx, "a test") {
		t.Error("Index should contain description")
	}
	if !strings.Contains(idx, "test.md") {
		t.Error("Index should contain filename")
	}
}

func TestParseMemoryFile(t *testing.T) {
	content := `---
name: test
title: Test Title
description: Test desc
type: user
---

Body content here.`

	m := parseMemoryFile("test", content)
	if m == nil {
		t.Fatal("parseMemoryFile returned nil")
	}
	if m.Name != "test" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.Title != "Test Title" {
		t.Errorf("Title = %q", m.Title)
	}
	if m.Type != TypeUser {
		t.Errorf("Type = %q, want user", m.Type)
	}
	if m.Body != "Body content here." {
		t.Errorf("Body = %q", m.Body)
	}
}

func TestParseMemoryFileNoFrontmatter(t *testing.T) {
	m := parseMemoryFile("test", "Just body, no frontmatter")
	if m != nil {
		t.Error("parseMemoryFile with no frontmatter should return nil")
	}
}

func TestParseMemoryFileTitleFallback(t *testing.T) {
	content := `---
name: my-memory
type: project
---

Body`

	m := parseMemoryFile("my-memory", content)
	if m == nil {
		t.Fatal("parseMemoryFile returned nil")
	}
	if m.Title != "my-memory" {
		t.Errorf("Title fallback = %q, want my-memory", m.Title)
	}
}

func TestRenderMemoryFile(t *testing.T) {
	m := Memory{
		Name:        "test",
		Title:       "Test",
		Description: "desc",
		Type:        TypeUser,
		Body:        "body",
	}
	rendered := renderMemoryFile(m)
	if !strings.Contains(rendered, "---") {
		t.Error("rendered should have frontmatter")
	}
	if !strings.Contains(rendered, "name: test") {
		t.Error("rendered should have name")
	}
	if !strings.Contains(rendered, "type: user") {
		t.Error("rendered should have type")
	}
	if !strings.HasSuffix(rendered, "body") {
		t.Errorf("rendered should end with body, got %q", rendered)
	}
}

func TestSearchText(t *testing.T) {
	m := Memory{
		Name:        "test",
		Title:       "Test",
		Description: "desc",
		Type:        TypeUser,
		Body:        "body",
	}
	text := SearchText(m)
	if !strings.Contains(text, "test") {
		t.Error("SearchText missing name")
	}
	if !strings.Contains(text, "user") {
		t.Error("SearchText missing type")
	}
	if !strings.Contains(text, "body") {
		t.Error("SearchText missing body")
	}
}

func TestParseSimpleFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantMeta map[string]string
		wantBody string
	}{
		{
			name:  "valid",
			input: "---\nkey: value\n---\nbody",
			wantMeta: map[string]string{"key": "value"},
			wantBody: "body",
		},
		{
			name:     "no frontmatter",
			input:    "just body",
			wantMeta: nil,
			wantBody: "just body",
		},
		{
			name:  "quoted value",
			input: "---\nname: \"hello\"\n---\nbody",
			wantMeta: map[string]string{"name": "hello"},
			wantBody: "body",
		},
		{
			name:  "multiple keys",
			input: "---\nkey1: v1\nkey2: v2\n---\nbody",
			wantMeta: map[string]string{"key1": "v1", "key2": "v2"},
			wantBody: "body",
		},
		{
			name:     "only opening ---",
			input:    "---\nkey: value\nbody",
			wantMeta: nil,
			wantBody: "---\nkey: value\nbody",
		},
		{
			name:     "empty",
			input:    "",
			wantMeta: nil,
			wantBody: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, body := parseSimpleFrontmatter(tt.input)
			if tt.wantMeta == nil && meta != nil {
				t.Fatalf("expected nil meta, got %v", meta)
			}
			if tt.wantMeta != nil {
				for k, v := range tt.wantMeta {
					if got := meta[k]; got != v {
						t.Errorf("meta[%s] = %q, want %q", k, got, v)
					}
				}
			}
			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

func TestProjectBucket(t *testing.T) {
	b1 := projectBucket("/home/user/project-a")
	b2 := projectBucket("/home/user/project-b")
	b3 := projectBucket("/home/user/project-a") // 同项目

	if b1 == b2 {
		t.Error("different projects should have different buckets")
	}
	if b1 != b3 {
		t.Error("same project should have same bucket")
	}
	if !strings.Contains(b1, "project-a") {
		t.Errorf("bucket should contain project name: %s", b1)
	}
}
