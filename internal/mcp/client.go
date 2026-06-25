package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/wsx864321/coding-agent/internal/jsonrpc"
)

// MCP 协议版本
const mcpVersion = "2024-11-05"

// initParams 是 initialize 请求的参数
type initParams struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    capabilities `json:"capabilities"`
	ClientInfo      clientInfo   `json:"clientInfo"`
}

type capabilities struct {
	Tools *struct{} `json:"tools,omitempty"`
}

type clientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ToolInfo 是 tools/list 响应中单个工具的描述
type ToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// listToolsResult 是 tools/list 的响应
type listToolsResult struct {
	Tools []ToolInfo `json:"tools"`
}

// callToolParams 是 tools/call 的请求参数
type callToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// callToolResult 是 tools/call 的响应
type callToolResult struct {
	Content []contentItem `json:"content"`
	IsError bool          `json:"isError"`
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ---------- Client interface ----------

// Client 是 MCP server 的 JSON-RPC 客户端
type Client interface {
	Connect(ctx context.Context) error
	ListTools(ctx context.Context) ([]ToolInfo, error)
	CallTool(ctx context.Context, name string, args map[string]any) (string, error)
	Close() error
}

// ---------- StdioClient ----------

// StdioClient 通过子进程 stdin/stdout 与 MCP server 通信
type StdioClient struct {
	cfg ServerConfig
	*jsonrpc.BaseClient
}

// NewStdioClient 创建一个 stdio 传输的 MCP 客户端
func NewStdioClient(cfg ServerConfig) *StdioClient {
	return &StdioClient{cfg: cfg}
}

func (c *StdioClient) Connect(ctx context.Context) error {
	var env []string
	for k, v := range c.cfg.Env {
		env = append(env, k+"="+v)
	}

	client, err := jsonrpc.NewBaseClient(jsonrpc.BaseClientOptions{
		Command:   c.cfg.Command,
		Args:      c.cfg.Args,
		Dir:       "",   // MCP 不指定 Dir
		Env:       env,
		Transport: jsonrpc.LineTransport{},
	})
	if err != nil {
		return fmt.Errorf("start mcp server %q: %w", c.cfg.Command, err)
	}
	c.BaseClient = client

	// MCP 握手：initialize
	if err := c.initialize(ctx); err != nil {
		c.Close()
		return fmt.Errorf("initialize mcp server %q: %w", c.cfg.Name, err)
	}

	return nil
}

func (c *StdioClient) initialize(ctx context.Context) error {
	params := initParams{
		ProtocolVersion: mcpVersion,
		Capabilities: capabilities{
			Tools: &struct{}{},
		},
		ClientInfo: clientInfo{
			Name:    "coding-agent",
			Version: "1.0.0",
		},
	}

	resp, err := c.Call(ctx, "initialize", params)
	if err != nil {
		return err
	}

	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return fmt.Errorf("parse initialize result: %w", err)
	}

	return c.Notify("notifications/initialized", nil)
}

func (c *StdioClient) ListTools(ctx context.Context) ([]ToolInfo, error) {
	resp, err := c.Call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	var result listToolsResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse tools/list result: %w", err)
	}
	return result.Tools, nil
}

func (c *StdioClient) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	resp, err := c.Call(ctx, "tools/call", callToolParams{Name: name, Arguments: args})
	if err != nil {
		return "", err
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("parse tools/call result: %w", err)
	}

	if result.IsError {
		if len(result.Content) > 0 {
			return "", fmt.Errorf("tool error: %s", result.Content[0].Text)
		}
		return "", fmt.Errorf("tool returned isError=true but no content")
	}

	var texts []string
	for _, item := range result.Content {
		if item.Type == "text" {
			texts = append(texts, item.Text)
		}
	}
	return strings.Join(texts, "\n"), nil
}

// ---------- HTTPClient ----------

// HTTPClient 通过 HTTP POST 与 MCP server 通信
type HTTPClient struct {
	cfg    ServerConfig
	client *http.Client
	mu     sync.Mutex
	nextID int64
}

// NewHTTPClient 创建一个 HTTP 传输的 MCP 客户端
func NewHTTPClient(cfg ServerConfig) *HTTPClient {
	return &HTTPClient{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *HTTPClient) Connect(ctx context.Context) error {
	return c.initialize(ctx)
}

func (c *HTTPClient) initialize(ctx context.Context) error {
	params := initParams{
		ProtocolVersion: mcpVersion,
		Capabilities: capabilities{
			Tools: &struct{}{},
		},
		ClientInfo: clientInfo{
			Name:    "coding-agent",
			Version: "1.0.0",
		},
	}

	resp, err := c.call(ctx, "initialize", params)
	if err != nil {
		return err
	}

	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return fmt.Errorf("parse initialize result: %w", err)
	}

	if _, err := c.post(ctx, jsonrpc.Message{
		JSONRPC: jsonrpc.Version,
		Method:  "notifications/initialized",
	}); err != nil {
		return fmt.Errorf("send initialized notification: %w", err)
	}

	return nil
}

func (c *HTTPClient) ListTools(ctx context.Context) ([]ToolInfo, error) {
	resp, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	var result listToolsResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse tools/list result: %w", err)
	}
	return result.Tools, nil
}

func (c *HTTPClient) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	resp, err := c.call(ctx, "tools/call", callToolParams{Name: name, Arguments: args})
	if err != nil {
		return "", err
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("parse tools/call result: %w", err)
	}

	if result.IsError {
		if len(result.Content) > 0 {
			return "", fmt.Errorf("tool error: %s", result.Content[0].Text)
		}
		return "", fmt.Errorf("tool returned isError=true but no content")
	}

	var texts []string
	for _, item := range result.Content {
		if item.Type == "text" {
			texts = append(texts, item.Text)
		}
	}
	return strings.Join(texts, "\n"), nil
}

func (c *HTTPClient) call(ctx context.Context, method string, params any) (*jsonrpc.Message, error) {
	msg, err := jsonrpc.NewRequest(c.nextID+1, method, params)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.nextID++
	msg.ID = c.nextID
	c.mu.Unlock()

	resp, err := c.post(ctx, *msg)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	return resp, nil
}

func (c *HTTPClient) post(ctx context.Context, msg jsonrpc.Message) (*jsonrpc.Message, error) {
	body, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	for k, v := range c.cfg.Headers {
		httpReq.Header.Set(k, osExpand(v))
	}

	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("post: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d: %s", httpResp.StatusCode, string(respBody))
	}

	var rpcResp jsonrpc.Message
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &rpcResp, nil
}

func (c *HTTPClient) Close() error {
	c.client.CloseIdleConnections()
	return nil
}

// ---------- helpers ----------

func osExpand(s string) string {
	return os.Expand(s, func(key string) string {
		return os.Getenv(key)
	})
}

// Ensure exec is used.
var _ = exec.Command
