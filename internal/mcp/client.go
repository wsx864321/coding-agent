package mcp

import (
	"bufio"
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
)

// JSON-RPC 2.0 常量
const (
	jsonrpcVersion = "2.0"
	mcpVersion     = "2024-11-05"
)

// rpcRequest 是 JSON-RPC 2.0 请求
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// rpcResponse 是 JSON-RPC 2.0 响应
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
	Method  string          `json:"method,omitempty"` // 用于识别 server→client 通知
}

// rpcError 是 JSON-RPC 2.0 错误
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *rpcError) Error() string {
	return fmt.Sprintf("MCP error %d: %s", e.Code, e.Message)
}

// initParams 是 initialize 请求的参数
type initParams struct {
	ProtocolVersion string      `json:"protocolVersion"`
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
	// Connect 建立连接并完成 MCP 握手（initialize → initialized）
	Connect(ctx context.Context) error
	// ListTools 获取 server 提供的工具列表
	ListTools(ctx context.Context) ([]ToolInfo, error)
	// CallTool 调用指定工具并返回结果文本
	CallTool(ctx context.Context, name string, args map[string]any) (string, error)
	// Close 关闭连接
	Close() error
}

// ---------- StdioClient ----------

// StdioClient 通过子进程 stdin/stdout 与 MCP server 通信
type StdioClient struct {
	cfg   ServerConfig
	cmd   *exec.Cmd
	stdin io.WriteCloser
	stdout io.ReadCloser

	mu     sync.Mutex
	nextID int64
}

// NewStdioClient 创建一个 stdio 传输的 MCP 客户端
func NewStdioClient(cfg ServerConfig) *StdioClient {
	return &StdioClient{cfg: cfg}
}

func (c *StdioClient) Connect(ctx context.Context) error {
	c.cmd = exec.CommandContext(ctx, c.cfg.Command, c.cfg.Args...)
	for k, v := range c.cfg.Env {
		c.cmd.Env = append(c.cmd.Env, k+"="+v)
	}

	var err error
	c.stdin, err = c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	c.stdout, err = c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	// 将 stderr 重定向到父进程 stderr 以便调试
	c.cmd.Stderr = nil

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("start mcp server %q: %w", c.cfg.Command, err)
	}

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

	resp, err := c.call(ctx, "initialize", params)
	if err != nil {
		return err
	}

	// 检查服务器支持的协议版本
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return fmt.Errorf("parse initialize result: %w", err)
	}

	// 发送 initialized 通知（无 id，不是请求）
	notif := rpcRequest{
		JSONRPC: jsonrpcVersion,
		Method:  "notifications/initialized",
	}
	if err := c.write(notif); err != nil {
		return fmt.Errorf("send initialized notification: %w", err)
	}

	return nil
}

func (c *StdioClient) ListTools(ctx context.Context) ([]ToolInfo, error) {
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

func (c *StdioClient) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	resp, err := c.call(ctx, "tools/call", callToolParams{Name: name, Arguments: args})
	if err != nil {
		return "", err
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("parse tools/call result: %w", err)
	}

	if result.IsError {
		// 将第一个 content 的 text 作为错误信息
		if len(result.Content) > 0 {
			return "", fmt.Errorf("tool error: %s", result.Content[0].Text)
		}
		return "", fmt.Errorf("tool returned isError=true but no content")
	}

	// 拼接所有 text content
	var texts []string
	for _, item := range result.Content {
		if item.Type == "text" {
			texts = append(texts, item.Text)
		}
	}
	return strings.Join(texts, "\n"), nil
}

func (c *StdioClient) call(ctx context.Context, method string, params any) (*rpcResponse, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	req := rpcRequest{
		JSONRPC: jsonrpcVersion,
		ID:      id,
		Method:  method,
		Params:  params,
	}
	c.mu.Unlock()

	if err := c.write(req); err != nil {
		return nil, fmt.Errorf("write request: %w", err)
	}

	resp, err := c.readResponse(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	return resp, nil
}

func (c *StdioClient) write(req rpcRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("write to stdin: %w", err)
	}
	if _, err := c.stdin.Write([]byte("\n")); err != nil {
		return fmt.Errorf("write newline: %w", err)
	}

	return nil
}

func (c *StdioClient) readResponse(ctx context.Context, expectedID int64) (*rpcResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	type result struct {
		resp *rpcResponse
		err  error
	}
	ch := make(chan result, 1)

	go func() {
		reader := bufio.NewReader(c.stdout)
		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				ch <- result{err: fmt.Errorf("read line: %w", err)}
				return
			}
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}

			var resp rpcResponse
			if err := json.Unmarshal(line, &resp); err != nil {
				ch <- result{err: fmt.Errorf("unmarshal response: %w", err)}
				return
			}

			// 跳过通知（无 id）
			if resp.ID == 0 && resp.Method != "" {
				continue
			}

			if resp.ID == expectedID {
				ch <- result{resp: &resp}
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("timeout waiting for response (id=%d)", expectedID)
	case r := <-ch:
		return r.resp, r.err
	}
}

func (c *StdioClient) Close() error {
	if c.stdin != nil {
		c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		// 使用独立的 context 来杀死进程（主 context 可能已取消）
		c.cmd.Process.Kill()
	}
	return nil
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

	// 发送 initialized 通知
	notif := rpcRequest{
		JSONRPC: jsonrpcVersion,
		Method:  "notifications/initialized",
	}
	if _, err := c.post(ctx, notif); err != nil {
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

func (c *HTTPClient) call(ctx context.Context, method string, params any) (*rpcResponse, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	req := rpcRequest{
		JSONRPC: jsonrpcVersion,
		ID:      id,
		Method:  method,
		Params:  params,
	}
	c.mu.Unlock()

	resp, err := c.post(ctx, req)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, resp.Error
	}

	return resp, nil
}

func (c *HTTPClient) post(ctx context.Context, req rpcRequest) (*rpcResponse, error) {
	body, err := json.Marshal(req)
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

	var rpcResp rpcResponse
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

// osExpand 展开字符串中的环境变量（${VAR} 或 $VAR）
func osExpand(s string) string {
	return os.Expand(s, func(key string) string {
		return os.Getenv(key)
	})
}
