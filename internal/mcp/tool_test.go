package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildToolName(t *testing.T) {
	name := BuildToolName("grafana", "query_prometheus")
	if name != "mcp__grafana__query_prometheus" {
		t.Errorf("got %q, want mcp__grafana__query_prometheus", name)
	}
}

func TestParseToolName_Valid(t *testing.T) {
	server, tool, ok := ParseToolName("mcp__grafana__query_prometheus")
	if !ok {
		t.Fatal("expected ok")
	}
	if server != "grafana" {
		t.Errorf("server: got %q, want grafana", server)
	}
	if tool != "query_prometheus" {
		t.Errorf("tool: got %q, want query_prometheus", tool)
	}
}

func TestParseToolName_Invalid(t *testing.T) {
	cases := []string{
		"bash",
		"mcp__",
		"mcp__grafana",
		"mcp__grafana_",
		"",
	}
	for _, name := range cases {
		_, _, ok := ParseToolName(name)
		if ok {
			t.Errorf("expected false for %q", name)
		}
	}
}

func TestParseToolName_Roundtrip(t *testing.T) {
	serverName := "my-server"
	toolName := "my-tool"
	full := BuildToolName(serverName, toolName)
	s, tn, ok := ParseToolName(full)
	if !ok {
		t.Fatal("parse failed")
	}
	if s != serverName || tn != toolName {
		t.Errorf("roundtrip: (%q, %q) vs (%q, %q)", serverName, toolName, s, tn)
	}
}

func TestTool_Name(t *testing.T) {
	info := ToolInfo{
		Name:        "echo",
		Description: "Echo back input",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}
	mcpTool := NewTool("test-server", info, nil)
	if mcpTool.Name() != "mcp__test-server__echo" {
		t.Errorf("Name: got %q", mcpTool.Name())
	}
}

func TestTool_Description(t *testing.T) {
	info := ToolInfo{
		Name:        "echo",
		Description: "Echo back input",
		InputSchema: json.RawMessage(`{}`),
	}
	mcpTool := NewTool("test", info, nil)
	desc := mcpTool.Description()
	if !strings.Contains(desc, "[MCP:test]") {
		t.Errorf("Description should contain [MCP:test]: %q", desc)
	}
	if !strings.Contains(desc, "Echo back input") {
		t.Errorf("Description should contain original: %q", desc)
	}
}

func TestTool_Schema(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}}}`)
	info := ToolInfo{
		Name:        "test",
		Description: "desc",
		InputSchema: schema,
	}
	mcpTool := NewTool("s", info, nil)
	if string(mcpTool.Schema()) != string(schema) {
		t.Error("schema mismatch")
	}
}

func TestTool_ReadOnly(t *testing.T) {
	mcpTool := NewTool("s", ToolInfo{}, nil)
	if mcpTool.ReadOnly() {
		t.Error("MCP tools should default to ReadOnly=false")
	}
}

// mockClient 实现了 Client 接口用于单元测试
type mockClient struct {
	tools     []ToolInfo
	callFunc  func(ctx context.Context, name string, args map[string]any) (string, error)
	connected bool
}

func (m *mockClient) Connect(ctx context.Context) error {
	m.connected = true
	return nil
}

func (m *mockClient) ListTools(ctx context.Context) ([]ToolInfo, error) {
	return m.tools, nil
}

func (m *mockClient) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	if m.callFunc != nil {
		return m.callFunc(ctx, name, args)
	}
	return "ok", nil
}

func (m *mockClient) Close() error {
	m.connected = false
	return nil
}

func TestTool_Execute(t *testing.T) {
	client := &mockClient{
		callFunc: func(ctx context.Context, name string, args map[string]any) (string, error) {
			return "hello from mock", nil
		},
	}

	info := ToolInfo{
		Name:        "mock-tool",
		Description: "mock",
		InputSchema: json.RawMessage(`{}`),
	}
	mcpTool := NewTool("mock-server", info, client)

	ctx := context.Background()
	result, err := mcpTool.Execute(ctx, map[string]any{"key": "value"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello from mock" {
		t.Errorf("result: got %q", result)
	}
}

func TestTool_Execute_NoClient(t *testing.T) {
	info := ToolInfo{
		Name:        "orphan",
		Description: "no client",
		InputSchema: json.RawMessage(`{}`),
	}
	mcpTool := NewTool("orphan", info, nil)

	ctx := context.Background()
	_, err := mcpTool.Execute(ctx, map[string]any{})
	if err == nil {
		t.Error("expected error for nil client")
	}
}

func TestTool_SetClient(t *testing.T) {
	info := ToolInfo{Name: "t"}
	tool := NewTool("s", info, nil)

	// 无 client 时应报错
	_, err := tool.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error without client")
	}

	// 设置 client 后应正常工作
	client := &mockClient{}
	tool.SetClient(client)

	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error after SetClient: %v", err)
	}
	if result != "ok" {
		t.Errorf("result: got %q", result)
	}
}
