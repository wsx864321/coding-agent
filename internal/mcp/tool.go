package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// ToolNamePrefix 是 MCP 工具名称的前缀，格式为 mcp__<server>__<tool>
const ToolNamePrefix = "mcp__"

// BuildToolName 构建 MCP 工具的完整名称：mcp__<server>__<tool>
func BuildToolName(serverName, toolName string) string {
	return ToolNamePrefix + serverName + "__" + toolName
}

// ParseToolName 从完整工具名中解析出 server 名称和原始工具名
func ParseToolName(fullName string) (serverName, toolName string, ok bool) {
	if !strings.HasPrefix(fullName, ToolNamePrefix) {
		return "", "", false
	}
	rest := fullName[len(ToolNamePrefix):]
	idx := strings.Index(rest, "__")
	if idx < 0 {
		return "", "", false
	}
	return rest[:idx], rest[idx+2:], true
}

// Tool 是满足 tools.Tool 接口的 MCP 工具包装器
type Tool struct {
	ServerName string
	Info       ToolInfo
	client     Client // 关联的 MCP 客户端
	mu         sync.Mutex
}

// NewTool 创建一个 MCP 工具包装器
func NewTool(serverName string, info ToolInfo, client Client) *Tool {
	return &Tool{
		ServerName: serverName,
		Info:       info,
		client:     client,
	}
}

// FullName 返回工具的完整名称（mcp__<server>__<tool>）
func (t *Tool) FullName() string {
	return BuildToolName(t.ServerName, t.Info.Name)
}

// Name 返回工具名称
func (t *Tool) Name() string {
	return t.FullName()
}

// Description 返回工具描述
func (t *Tool) Description() string {
	return fmt.Sprintf("[MCP:%s] %s", t.ServerName, t.Info.Description)
}

// Schema 返回工具的参数 JSON Schema
func (t *Tool) Schema() json.RawMessage {
	return t.Info.InputSchema
}

// Execute 通过 MCP client 调用远程工具
func (t *Tool) Execute(ctx context.Context, args map[string]any) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.client == nil {
		return "", fmt.Errorf("MCP client for server %q is not connected", t.ServerName)
	}

	result, err := t.client.CallTool(ctx, t.Info.Name, args)
	if err != nil {
		return "", fmt.Errorf("MCP tool %q call failed: %w", t.Info.Name, err)
	}

	return result, nil
}

// ReadOnly 返回是否为只读工具；默认 false（保守策略）
func (t *Tool) ReadOnly() bool {
	// MCP spec 暂无 ReadOnly annotation，保守返回 false
	return false
}

// SetClient 更新关联的 MCP 客户端（用于重连场景）
func (t *Tool) SetClient(client Client) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.client = client
}
