package mcp

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRPCRequest_Marshal(t *testing.T) {
	req := rpcRequest{
		JSONRPC: jsonrpcVersion,
		ID:      1,
		Method:  "tools/list",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed["jsonrpc"] != "2.0" {
		t.Error("jsonrpc should be 2.0")
	}
	if parsed["method"] != "tools/list" {
		t.Error("method mismatch")
	}
}

func TestRPCResponse_Unmarshal(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"test","description":"a test tool","inputSchema":{"type":"object"}}]}}`

	var resp rpcResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}

	if resp.ID != 1 {
		t.Errorf("id: got %d, want 1", resp.ID)
	}
	if resp.Error != nil {
		t.Error("unexpected error")
	}

	var result listToolsResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result.Tools))
	}
	if result.Tools[0].Name != "test" {
		t.Errorf("tool name: got %q, want %q", result.Tools[0].Name, "test")
	}
}

func TestRPCResponse_Error(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"Method not found"}}`

	var resp rpcResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}

	if resp.Error == nil {
		t.Fatal("expected error")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("error code: got %d, want -32601", resp.Error.Code)
	}
}

func TestListToolsResult_Unmarshal(t *testing.T) {
	raw := `{"tools":[
		{"name":"echo","description":"Echo back input","inputSchema":{"type":"object","properties":{"message":{"type":"string"}}}}
	]}`

	var result listToolsResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}

	if len(result.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result.Tools))
	}
	tool := result.Tools[0]
	if tool.Name != "echo" {
		t.Errorf("name: got %q, want %q", tool.Name, "echo")
	}
	if tool.Description != "Echo back input" {
		t.Errorf("description mismatch")
	}
}

func TestCallToolResult_Text(t *testing.T) {
	raw := `{"content":[{"type":"text","text":"Hello, world!"}],"isError":false}`

	var result callToolResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Error("unexpected isError=true")
	}
	if len(result.Content) != 1 || result.Content[0].Text != "Hello, world!" {
		t.Errorf("content mismatch: %v", result.Content)
	}
}

func TestCallToolResult_Error(t *testing.T) {
	raw := `{"content":[{"type":"text","text":"Something went wrong"}],"isError":true}`

	var result callToolResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}

	if !result.IsError {
		t.Error("expected isError=true")
	}
}

func TestOSExpand(t *testing.T) {
	t.Setenv("TEST_TOKEN", "abc123")
	t.Setenv("TEST_HOST", "localhost")

	tests := []struct {
		input    string
		expected string
	}{
		{"Bearer ${TEST_TOKEN}", "Bearer abc123"},
		{"http://${TEST_HOST}:8080", "http://localhost:8080"},
		{"no variable here", "no variable here"},
		{"", ""},
	}

	for _, tt := range tests {
		got := osExpand(tt.input)
		if got != tt.expected {
			t.Errorf("osExpand(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNewStdioClient(t *testing.T) {
	cfg := ServerConfig{
		Name:    "test",
		Command: "echo",
		Args:    []string{"hello"},
	}
	client := NewStdioClient(cfg)
	if client == nil {
		t.Fatal("NewStdioClient returned nil")
	}
	if client.cfg.Name != "test" {
		t.Errorf("name mismatch")
	}
}

func TestNewHTTPClient(t *testing.T) {
	cfg := ServerConfig{
		Name: "test",
		URL:  "http://localhost:8080",
	}
	client := NewHTTPClient(cfg)
	if client == nil {
		t.Fatal("NewHTTPClient returned nil")
	}
	if client.cfg.URL != "http://localhost:8080" {
		t.Errorf("url mismatch")
	}
}

func TestStdioClient_Connect_BadCommand(t *testing.T) {
	cfg := ServerConfig{
		Name:    "bad",
		Command: "this-command-does-not-exist-xyz",
	}
	client := NewStdioClient(cfg)
	ctx := context.Background()
	err := client.Connect(ctx)
	if err == nil {
		t.Error("expected error for bad command")
		client.Close()
	}
}

func TestHTTPClient_Connect_BadURL(t *testing.T) {
	cfg := ServerConfig{
		Name: "bad",
		URL:  "http://127.0.0.1:1/mcp",
	}
	client := NewHTTPClient(cfg)
	ctx := context.Background()
	err := client.Connect(ctx)
	if err == nil {
		t.Error("expected error for unreachable URL")
	}
}
