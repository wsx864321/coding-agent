package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Empty(t *testing.T) {
	dir := t.TempDir()
	configs := Load(LoadOptions{ProjectRoot: dir})
	if len(configs) != 0 {
		t.Fatalf("expected 0 configs, got %d", len(configs))
	}
}

func TestLoad_ProjectOnly(t *testing.T) {
	dir := t.TempDir()
	mcpDir := filepath.Join(dir, ".coding-agent")
	if err := os.MkdirAll(mcpDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mcpJSON := `{
  "servers": [
    {
      "name": "my-server",
      "command": "my-tool",
      "args": ["--flag"],
      "tier": "eager"
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(mcpDir, "mcp.json"), []byte(mcpJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	configs := Load(LoadOptions{ProjectRoot: dir, HomeDir: dir})
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}

	c := configs[0]
	if c.Name != "my-server" {
		t.Errorf("name: got %q, want %q", c.Name, "my-server")
	}
	if c.Command != "my-tool" {
		t.Errorf("command: got %q, want %q", c.Command, "my-tool")
	}
	if len(c.Args) != 1 || c.Args[0] != "--flag" {
		t.Errorf("args: got %v, want [--flag]", c.Args)
	}
	if c.Tier != TierEager {
		t.Errorf("tier: got %q, want %q", c.Tier, TierEager)
	}
	if !c.IsStdio() {
		t.Errorf("transport: got %q, want %q", c.Transport, TransportStdio)
	}
	if c.Scope != ScopeProject {
		t.Errorf("scope: got %q, want %q", c.Scope, ScopeProject)
	}
}

func TestLoad_DedupProjectOverridesGlobal(t *testing.T) {
	dir := t.TempDir()
	projDir := filepath.Join(dir, ".coding-agent")
	globalDir := filepath.Join(dir, "home", ".coding-agent")

	for _, d := range []string{projDir, globalDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// 全局配置
	globalJSON := `{
  "servers": [
    {"name": "shared", "command": "global-cmd"},
    {"name": "global-only", "command": "global-only-cmd"}
  ]
}`
	if err := os.WriteFile(filepath.Join(globalDir, "mcp.json"), []byte(globalJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	// 项目配置（覆盖 shared）
	projJSON := `{
  "servers": [
    {"name": "shared", "command": "proj-cmd"}
  ]
}`
	if err := os.WriteFile(filepath.Join(projDir, "mcp.json"), []byte(projJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	configs := Load(LoadOptions{ProjectRoot: dir, HomeDir: filepath.Join(dir, "home")})
	if len(configs) != 2 {
		t.Fatalf("expected 2 configs, got %d (%v)", len(configs), names(configs))
	}

	byName := make(map[string]ServerConfig)
	for _, c := range configs {
		byName[c.Name] = c
	}

	shared, ok := byName["shared"]
	if !ok {
		t.Fatal("shared server not found")
	}
	if shared.Command != "proj-cmd" {
		t.Errorf("shared should be overridden by project: got %q", shared.Command)
	}

	if _, ok := byName["global-only"]; !ok {
		t.Error("global-only server should be preserved")
	}
}

func TestLoad_HTTPTransport(t *testing.T) {
	dir := t.TempDir()
	mcpDir := filepath.Join(dir, ".coding-agent")
	if err := os.MkdirAll(mcpDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mcpJSON := `{
  "servers": [
    {
      "name": "http-server",
      "url": "http://localhost:8080/mcp",
      "headers": {"Authorization": "Bearer ${TOKEN}"}
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(mcpDir, "mcp.json"), []byte(mcpJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	configs := Load(LoadOptions{ProjectRoot: dir, HomeDir: dir})
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}

	c := configs[0]
	if !c.IsHTTP() {
		t.Errorf("transport: got %q, want %q", c.Transport, TransportHTTP)
	}
	if c.URL != "http://localhost:8080/mcp" {
		t.Errorf("url: got %q", c.URL)
	}
	if c.Headers["Authorization"] != "Bearer ${TOKEN}" {
		t.Errorf("headers: got %v", c.Headers)
	}
}

func TestLoad_InvalidConfig(t *testing.T) {
	dir := t.TempDir()
	mcpDir := filepath.Join(dir, ".coding-agent")
	if err := os.MkdirAll(mcpDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 缺少 name
	badJSON := `{"servers": [{"command": "cmd"}]}`
	if err := os.WriteFile(filepath.Join(mcpDir, "mcp.json"), []byte(badJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	configs := Load(LoadOptions{ProjectRoot: dir, HomeDir: dir})
	if len(configs) != 0 {
		t.Fatalf("expected 0 configs (no name), got %d", len(configs))
	}
}

func TestLoad_DefaultTier(t *testing.T) {
	dir := t.TempDir()
	mcpDir := filepath.Join(dir, ".coding-agent")
	if err := os.MkdirAll(mcpDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mcpJSON := `{
  "servers": [
    {"name": "default-tier", "command": "cmd"}
  ]
}`
	if err := os.WriteFile(filepath.Join(mcpDir, "mcp.json"), []byte(mcpJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	configs := Load(LoadOptions{ProjectRoot: dir, HomeDir: dir})
	if len(configs) != 1 {
		t.Fatalf("expected 1 config, got %d", len(configs))
	}
	if configs[0].Tier != TierBackground {
		t.Errorf("default tier should be background, got %q", configs[0].Tier)
	}
}

func names(configs []ServerConfig) []string {
	n := make([]string, len(configs))
	for i, c := range configs {
		n[i] = c.Name
	}
	return n
}
