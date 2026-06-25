package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------- JSON-RPC framing ----------

// jsonRPCMessage 是 LSP 使用的 JSON-RPC 2.0 消息
type jsonRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *jsonRPCError) Error() string {
	return fmt.Sprintf("LSP error %d: %s", e.Code, e.Message)
}

// Client 是 LSP stdio 客户端
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	mu       sync.Mutex
	nextID   int64
	pending  map[int64]chan *jsonRPCMessage
	diagMu   sync.RWMutex
	diags    map[string][]Diagnostic // uri → diagnostics
	rootURI  string
	rootPath string

	readerDone chan struct{}
}

// NewClient 创建并启动一个 LSP 客户端
func NewClient(command string, args []string, rootPath string) (*Client, error) {
	c := &Client{
		pending:    make(map[int64]chan *jsonRPCMessage),
		diags:      make(map[string][]Diagnostic),
		rootPath:   rootPath,
		rootURI:    pathToURI(rootPath),
		readerDone: make(chan struct{}),
	}

	c.cmd = exec.Command(command, args...)
	c.cmd.Dir = rootPath

	var err error
	c.stdin, err = c.cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	c.stdout, err = c.cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := c.cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", command, err)
	}

	// 启动 reader goroutine
	go c.readLoop()

	// initialize
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.initialize(ctx); err != nil {
		c.Close()
		return nil, fmt.Errorf("initialize LSP server %s: %w", command, err)
	}

	return c, nil
}

// initialize 完成 LSP 握手
func (c *Client) initialize(ctx context.Context) error {
	initParams := map[string]any{
		"processId": nil,
		"rootUri":   c.rootURI,
		"rootPath":  c.rootPath,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"hover":          map[string]any{"contentFormat": []string{"markdown", "plaintext"}},
				"definition":     map[string]any{"linkSupport": false},
				"references":     map[string]any{},
				"documentSymbol": map[string]any{},
			},
			"workspace": map[string]any{
				"symbol": map[string]any{},
			},
		},
		"initializationOptions": nil,
	}

	resp, err := c.call(ctx, "initialize", initParams)
	if err != nil {
		return err
	}

	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return fmt.Errorf("parse initialize result: %w", err)
	}

	// 发送 initialized 通知
	notif := jsonRPCMessage{
		JSONRPC: "2.0",
		Method:  "initialized",
		Params:  json.RawMessage("{}"),
	}
	return c.write(notif)
}

// call 发送请求并等待响应
func (c *Client) call(ctx context.Context, method string, params any) (*jsonRPCMessage, error) {
	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}

	c.mu.Lock()
	c.nextID++
	id := c.nextID
	ch := make(chan *jsonRPCMessage, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	msg := jsonRPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  paramsBytes,
	}
	if err := c.write(msg); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("timeout: %w", ctx.Err())
	case resp := <-ch:
		return resp, nil
	}
}

// notify 发送通知（不等待响应）
func (c *Client) notify(method string, params any) error {
	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return err
	}
	msg := jsonRPCMessage{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsBytes,
	}
	return c.write(msg)
}

// write 写入一条 JSON-RPC 消息（带 Content-Length 头）
func (c *Client) write(msg jsonRPCMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := c.stdin.Write([]byte(header)); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("write body: %w", err)
	}
	return nil
}

// readLoop 持续从 stdout 读取 LSP 消息
func (c *Client) readLoop() {
	defer close(c.readerDone)
	reader := bufio.NewReader(c.stdout)

	for {
		// 读 Content-Length 头
		header, err := reader.ReadString('\n')
		if err != nil {
			return // 连接关闭
		}
		header = strings.TrimSpace(header)
		if !strings.HasPrefix(header, "Content-Length: ") {
			continue
		}

		length, err := strconv.Atoi(strings.TrimPrefix(header, "Content-Length: "))
		if err != nil {
			continue
		}

		// 跳过 \r\n
		reader.ReadString('\n')

		// 读 body
		body := make([]byte, length)
		if _, err := io.ReadFull(reader, body); err != nil {
			return
		}

		var msg jsonRPCMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			continue
		}

		// 通知消息 (no id)
		if msg.ID == 0 && msg.Method != "" {
			c.handleNotification(msg)
			continue
		}

		// 响应消息
		if msg.ID > 0 {
			c.mu.Lock()
			ch, ok := c.pending[msg.ID]
			if ok {
				delete(c.pending, msg.ID)
			}
			c.mu.Unlock()
			if ok {
				ch <- &msg
			}
		}
	}
}

// handleNotification 处理 server→client 通知
func (c *Client) handleNotification(msg jsonRPCMessage) {
	switch msg.Method {
	case "textDocument/publishDiagnostics":
		var params PublishDiagnosticsParams
		if err := json.Unmarshal(msg.Params, &params); err == nil {
			c.diagMu.Lock()
			c.diags[params.URI] = params.Diagnostics
			c.diagMu.Unlock()
		}
	case "$/logTrace", "window/logMessage":
		// 忽略
	}
}

// ---------- 公开 API ----------

// Definition 获取符号定义位置
func (c *Client) Definition(ctx context.Context, file string, line, character int) ([]Location, error) {
	uri := pathToURI(file)
	c.ensureOpen(ctx, uri)

	params := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: character},
	}

	resp, err := c.call(ctx, "textDocument/definition", params)
	if err != nil {
		return nil, err
	}

	// 可能返回单个 Location、[]Location 或 null
	var locs []Location
	if resp.Result == nil || string(resp.Result) == "null" {
		return nil, nil
	}
	// 尝试数组
	if err := json.Unmarshal(resp.Result, &locs); err != nil {
		// 尝试单个
		var loc Location
		if err := json.Unmarshal(resp.Result, &loc); err != nil {
			return nil, fmt.Errorf("parse definition result: %w, raw: %s", err, string(resp.Result))
		}
		locs = []Location{loc}
	}
	return locs, nil
}

// References 获取符号的所有引用
func (c *Client) References(ctx context.Context, file string, line, character int) ([]Location, error) {
	uri := pathToURI(file)
	c.ensureOpen(ctx, uri)

	params := ReferenceParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Position:     Position{Line: line, Character: character},
		},
		Context: ReferenceContext{IncludeDeclaration: true},
	}

	resp, err := c.call(ctx, "textDocument/references", params)
	if err != nil {
		return nil, err
	}

	var locs []Location
	if resp.Result == nil || string(resp.Result) == "null" {
		return nil, nil
	}
	if err := json.Unmarshal(resp.Result, &locs); err != nil {
		return nil, fmt.Errorf("parse references result: %w", err)
	}
	return locs, nil
}

// Hover 获取符号的类型和文档
func (c *Client) Hover(ctx context.Context, file string, line, character int) (*Hover, error) {
	uri := pathToURI(file)
	c.ensureOpen(ctx, uri)

	params := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: character},
	}

	resp, err := c.call(ctx, "textDocument/hover", params)
	if err != nil {
		return nil, err
	}

	if resp.Result == nil || string(resp.Result) == "null" {
		return nil, nil
	}

	var h Hover
	if err := json.Unmarshal(resp.Result, &h); err != nil {
		return nil, fmt.Errorf("parse hover result: %w", err)
	}
	return &h, nil
}

// DocumentSymbols 获取文件中的所有符号（outline）
func (c *Client) DocumentSymbols(ctx context.Context, file string) ([]DocumentSymbol, error) {
	uri := pathToURI(file)
	c.ensureOpen(ctx, uri)

	params := map[string]any{
		"textDocument": map[string]string{"uri": uri},
	}

	resp, err := c.call(ctx, "textDocument/documentSymbol", params)
	if err != nil {
		return nil, err
	}

	if resp.Result == nil || string(resp.Result) == "null" {
		return nil, nil
	}

	// 可能返回 DocumentSymbol[] 或 SymbolInformation[]
	var docSyms []DocumentSymbol
	if err := json.Unmarshal(resp.Result, &docSyms); err != nil {
		// 尝试 SymbolInformation[]（扁平格式）
		var symInfos []SymbolInformation
		if err := json.Unmarshal(resp.Result, &symInfos); err != nil {
			return nil, fmt.Errorf("parse documentSymbol result: %w, raw: %s", err, string(resp.Result))
		}
		// 转换为简单 DocumentSymbol
		for _, si := range symInfos {
			docSyms = append(docSyms, DocumentSymbol{
				Name:           si.Name,
				Kind:           si.Kind,
				Range:          si.Location.Range,
				SelectionRange: si.Location.Range,
			})
		}
	}
	return docSyms, nil
}

// WorkspaceSymbols 跨项目搜索符号
func (c *Client) WorkspaceSymbols(ctx context.Context, query string) ([]SymbolInformation, error) {
	params := map[string]any{"query": query}

	resp, err := c.call(ctx, "workspace/symbol", params)
	if err != nil {
		return nil, err
	}

	if resp.Result == nil || string(resp.Result) == "null" {
		return nil, nil
	}

	var syms []SymbolInformation
	if err := json.Unmarshal(resp.Result, &syms); err != nil {
		return nil, fmt.Errorf("parse workspace/symbol result: %w", err)
	}
	return syms, nil
}

// GetDiagnostics 返回已缓存的诊断信息
func (c *Client) GetDiagnostics(uri string) []Diagnostic {
	c.diagMu.RLock()
	defer c.diagMu.RUnlock()
	return c.diags[uri]
}

// ForceDiagnostics 发送 textDocument/didOpen 以触发服务器重新诊断
func (c *Client) ForceDiagnostics(ctx context.Context, file string) ([]Diagnostic, error) {
	uri := pathToURI(file)
	c.ensureOpen(ctx, uri)

	// 等待一小段时间让服务器有时间推送诊断
	time.Sleep(500 * time.Millisecond)

	return c.GetDiagnostics(uri), nil
}

// Close 关闭 LSP 连接
func (c *Client) Close() error {
	// 发送 shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c.call(ctx, "shutdown", nil)

	// 发送 exit 通知
	c.notify("exit", nil)

	// 等待 reader 退出
	select {
	case <-c.readerDone:
	case <-time.After(2 * time.Second):
	}

	if c.stdin != nil {
		c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		c.cmd.Process.Kill()
	}
	return nil
}

// ensureOpen 确保文件已被服务器跟踪
func (c *Client) ensureOpen(ctx context.Context, uri string) {
	params := map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": detectLanguage(uri),
			"version":    1,
			"text":       "",
		},
	}
	c.notify("textDocument/didOpen", params)
}

// ---------- helpers ----------

// pathToURI 将文件路径转换为 URI
func pathToURI(path string) string {
	abs, _ := filepath.Abs(path)
	// Windows: C:\foo\bar.go → file:///C:/foo/bar.go
	// Unix:    /foo/bar.go    → file:///foo/bar.go
	abs = filepath.ToSlash(abs)
	if !strings.HasPrefix(abs, "/") {
		abs = "/" + abs
	}
	return "file://" + abs
}

// URIToPath 将 URI 转换回文件路径
func URIToPath(uri string) string {
	path := strings.TrimPrefix(uri, "file://")
	// Windows: /C:/foo/bar.go → C:/foo/bar.go
	if len(path) > 2 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path)
}

// detectLanguage 从文件扩展名推测语言 ID
func detectLanguage(uri string) string {
	ext := strings.ToLower(filepath.Ext(uri))
	switch ext {
	case ".go":
		return "go"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "typescriptreact"
	case ".js":
		return "javascript"
	case ".jsx":
		return "javascriptreact"
	case ".py":
		return "python"
	case ".rs":
		return "rust"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".cxx", ".hpp":
		return "cpp"
	case ".java":
		return "java"
	case ".cs":
		return "csharp"
	case ".rb":
		return "ruby"
	case ".swift":
		return "swift"
	case ".kt":
		return "kotlin"
	case ".scala":
		return "scala"
	case ".lua":
		return "lua"
	case ".zig":
		return "zig"
	case ".proto":
		return "proto"
	default:
		return "plaintext"
	}
}
