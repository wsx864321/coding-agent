package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wsx864321/coding-agent/internal/tools"
)

func TestInstallSourceTool_Name(t *testing.T) {
	tool := NewInstallSourceTool(nil, "")
	if tool.Name() != "install_source" {
		t.Errorf("name: got %q", tool.Name())
	}
}

func TestInstallSourceTool_Schema(t *testing.T) {
	tool := NewInstallSourceTool(nil, "")
	schema := tool.Schema()
	if len(schema) == 0 {
		t.Error("schema should not be empty")
	}
	var parsed map[string]any
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatal(err)
	}
}

func TestInstallSourceTool_ReadOnly(t *testing.T) {
	tool := NewInstallSourceTool(nil, "")
	if tool.ReadOnly() {
		t.Error("install_source should not be read-only")
	}
}

func TestInstallSourceTool_InvalidOp(t *testing.T) {
	manager := &Manager{} // nil registry is fine for this test
	tool := NewInstallSourceTool(manager, "")

	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{
		"op": "delete",
	})
	if err == nil {
		t.Error("expected error for invalid op")
	}
	if !strings.Contains(err.Error(), "不支持的操作") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInstallSourceTool_UninstallMissingName(t *testing.T) {
	manager := &Manager{}
	tool := NewInstallSourceTool(manager, "")

	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{
		"op": "uninstall",
	})
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestInstallSourceTool_InstallMissingTransport(t *testing.T) {
	manager := NewManager(nil, tools.NewRegistry())
	tool := NewInstallSourceTool(manager, "")

	ctx := context.Background()
	_, err := tool.Execute(ctx, map[string]any{
		"op":   "install",
		"name": "test-server",
	})
	if err == nil {
		t.Error("expected error for missing command/url")
	}
}

func TestSaveAndLoadManifest(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager(nil, tools.NewRegistry())
	tool := NewInstallSourceTool(manager, dir)

	// 保存配置
	cfg := ServerConfig{
		Name:      "my-server",
		Command:   "my-cmd",
		Args:      []string{"--flag"},
		Transport: TransportStdio,
		Tier:      TierEager,
	}
	if err := tool.saveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	// 验证文件内容
	path := tool.configPath()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var manifest ManifestFile
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(manifest.Servers))
	}
	s := manifest.Servers[0]
	if s.Name != "my-server" {
		t.Errorf("name: got %q", s.Name)
	}
	if s.Command != "my-cmd" {
		t.Errorf("command: got %q", s.Command)
	}
	if s.Tier != string(TierEager) {
		t.Errorf("tier: got %q", s.Tier)
	}
}

func TestSaveConfig_Deduplicate(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager(nil, tools.NewRegistry())
	tool := NewInstallSourceTool(manager, dir)

	// 保存第一个 server
	if err := tool.saveConfig(ServerConfig{
		Name:      "server-a",
		Command:   "cmd-a",
		Transport: TransportStdio,
	}); err != nil {
		t.Fatal(err)
	}

	// 保存第二个 server
	if err := tool.saveConfig(ServerConfig{
		Name:      "server-b",
		Command:   "cmd-b",
		Transport: TransportStdio,
	}); err != nil {
		t.Fatal(err)
	}

	// 覆盖 server-a
	if err := tool.saveConfig(ServerConfig{
		Name:      "server-a",
		Command:   "cmd-a-v2",
		Transport: TransportStdio,
	}); err != nil {
		t.Fatal(err)
	}

	manifest, err := tool.loadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(manifest.Servers))
	}

	// 找到 server-a 确认已更新
	for _, s := range manifest.Servers {
		if s.Name == "server-a" && s.Command != "cmd-a-v2" {
			t.Errorf("server-a not updated: command=%q", s.Command)
		}
	}
}

func TestRemoveConfig(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager(nil, tools.NewRegistry())
	tool := NewInstallSourceTool(manager, dir)

	// 保存两个 server
	for _, name := range []string{"keep", "remove-me"} {
		if err := tool.saveConfig(ServerConfig{
			Name:      name,
			Command:   "cmd",
			Transport: TransportStdio,
		}); err != nil {
			t.Fatal(err)
		}
	}

	// 移除 "remove-me"
	if err := tool.removeConfig("remove-me"); err != nil {
		t.Fatal(err)
	}

	manifest, err := tool.loadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(manifest.Servers))
	}
	if manifest.Servers[0].Name != "keep" {
		t.Errorf("expected 'keep', got %q", manifest.Servers[0].Name)
	}
}

func TestRemoveConfig_NonExistent(t *testing.T) {
	dir := t.TempDir()
	manager := NewManager(nil, tools.NewRegistry())
	tool := NewInstallSourceTool(manager, dir)

	// 移除不存在的 server 不应报错
	if err := tool.removeConfig("nonexistent"); err != nil {
		t.Fatal(err)
	}
}

func TestServerConfigToFile(t *testing.T) {
	cfg := ServerConfig{
		Name:      "test",
		Command:   "cmd",
		Args:      []string{"--flag"},
		Transport: TransportStdio,
		Tier:      TierEager,
	}
	f := serverConfigToFile(cfg)
	if f.Name != "test" {
		t.Errorf("name: got %q", f.Name)
	}
	if f.Tier != string(TierEager) {
		t.Errorf("tier: got %q", f.Tier)
	}
}

func TestInstallSource_Integration(t *testing.T) {
	dir := t.TempDir()
	registry := tools.NewRegistry()
	manager := NewManager(nil, registry)
	manager.ctx, manager.cancel = context.WithCancel(context.Background())
	defer manager.Stop()

	tool := NewInstallSourceTool(manager, dir)

	// 安装一个 server（仅保存配置，不实际连接因为 cmd 不存在）
	ctx := context.Background()
	result, err := tool.Execute(ctx, map[string]any{
		"op":      "install",
		"name":    "echo-server",
		"command": "echo",
		"args":    []any{"hello"},
		"tier":    "eager",
	})
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	t.Logf("install result: %s", result)

	// 验证文件已创建
	configPath := filepath.Join(dir, ".coding-agent", "mcp.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("mcp.json was not created")
	}

	// 卸载
	result, err = tool.Execute(ctx, map[string]any{
		"op":   "uninstall",
		"name": "echo-server",
	})
	if err != nil {
		t.Fatalf("uninstall failed: %v", err)
	}
	t.Logf("uninstall result: %s", result)

	// 验证文件已清空 servers
	manifest, err := tool.loadManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Servers) != 0 {
		t.Errorf("expected 0 servers after uninstall, got %d", len(manifest.Servers))
	}
}
