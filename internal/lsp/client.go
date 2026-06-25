package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wsx864321/coding-agent/internal/jsonrpc"
)

// Client 是 LSP stdio 客户端，内嵌 jsonrpc.BaseClient 处理传输层
type Client struct {
	*jsonrpc.BaseClient

	diagMu sync.RWMutex
	diags  map[string][]Diagnostic
	rootURI  string
	rootPath string
}

// NewClient 创建并启动一个 LSP 客户端
func NewClient(command string, args []string, rootPath string) (*Client, error) {
	c := &Client{
		diags:    make(map[string][]Diagnostic),
		rootPath: rootPath,
		rootURI:  pathToURI(rootPath),
	}

	base, err := jsonrpc.NewBaseClient(jsonrpc.BaseClientOptions{
		Command:   command,
		Args:      args,
		Dir:       rootPath,
		Transport: jsonrpc.ContentLengthTransport{},
	})
	if err != nil {
		return nil, fmt.Errorf("start %s: %w", command, err)
	}
	c.BaseClient = base

	// 设置通知处理器（捕获 publishDiagnostics）
	c.OnNotification = c.handleNotification

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

	resp, err := c.Call(ctx, "initialize", initParams)
	if err != nil {
		return err
	}

	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return fmt.Errorf("parse initialize result: %w", err)
	}

	return c.Notify("initialized", struct{}{})
}

// handleNotification 处理 server→client 通知
func (c *Client) handleNotification(msg *jsonrpc.Message) {
	switch msg.Method {
	case "textDocument/publishDiagnostics":
		var params PublishDiagnosticsParams
		if err := json.Unmarshal(msg.Params, &params); err == nil {
			c.diagMu.Lock()
			c.diags[params.URI] = params.Diagnostics
			c.diagMu.Unlock()
		}
	}
}

// ---------- 公开 API ----------

// Definition 获取符号定义位置
func (c *Client) Definition(ctx context.Context, file string, line, character int) ([]Location, error) {
	uri := pathToURI(file)
	c.ensureOpen(uri)

	params := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: character},
	}

	resp, err := c.Call(ctx, "textDocument/definition", params)
	if err != nil {
		return nil, err
	}
	return parseLocations(resp), nil
}

// References 获取符号的所有引用
func (c *Client) References(ctx context.Context, file string, line, character int) ([]Location, error) {
	uri := pathToURI(file)
	c.ensureOpen(uri)

	params := ReferenceParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Position:     Position{Line: line, Character: character},
		},
		Context: ReferenceContext{IncludeDeclaration: true},
	}

	resp, err := c.Call(ctx, "textDocument/references", params)
	if err != nil {
		return nil, err
	}
	return parseLocations(resp), nil
}

// Hover 获取符号的类型和文档
func (c *Client) Hover(ctx context.Context, file string, line, character int) (*Hover, error) {
	uri := pathToURI(file)
	c.ensureOpen(uri)

	params := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: line, Character: character},
	}

	resp, err := c.Call(ctx, "textDocument/hover", params)
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
	c.ensureOpen(uri)

	params := map[string]any{
		"textDocument": map[string]string{"uri": uri},
	}

	resp, err := c.Call(ctx, "textDocument/documentSymbol", params)
	if err != nil {
		return nil, err
	}

	if resp.Result == nil || string(resp.Result) == "null" {
		return nil, nil
	}

	var docSyms []DocumentSymbol
	if err := json.Unmarshal(resp.Result, &docSyms); err != nil {
		var symInfos []SymbolInformation
		if err := json.Unmarshal(resp.Result, &symInfos); err != nil {
			return nil, fmt.Errorf("parse documentSymbol: %w", err)
		}
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

	resp, err := c.Call(ctx, "workspace/symbol", params)
	if err != nil {
		return nil, err
	}

	if resp.Result == nil || string(resp.Result) == "null" {
		return nil, nil
	}

	var syms []SymbolInformation
	if err := json.Unmarshal(resp.Result, &syms); err != nil {
		return nil, fmt.Errorf("parse workspace/symbol: %w", err)
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
	c.ensureOpen(uri)
	time.Sleep(500 * time.Millisecond)
	return c.GetDiagnostics(uri), nil
}

// Close 关闭 LSP 连接（发送 shutdown + exit）
func (c *Client) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c.Call(ctx, "shutdown", nil)
	c.Notify("exit", nil)
	return c.BaseClient.Close()
}

// ensureOpen 确保文件已被服务器跟踪
func (c *Client) ensureOpen(uri string) {
	c.Notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": detectLanguage(uri),
			"version":    1,
			"text":       "",
		},
	})
}

// ---------- helpers ----------

func parseLocations(resp *jsonrpc.Message) []Location {
	if resp.Result == nil || string(resp.Result) == "null" {
		return nil
	}
	var locs []Location
	if err := json.Unmarshal(resp.Result, &locs); err != nil {
		var loc Location
		if err := json.Unmarshal(resp.Result, &loc); err != nil {
			return nil
		}
		locs = []Location{loc}
	}
	return locs
}

func pathToURI(path string) string {
	abs, _ := filepath.Abs(path)
	abs = filepath.ToSlash(abs)
	if !strings.HasPrefix(abs, "/") {
		abs = "/" + abs
	}
	return "file://" + abs
}

// URIToPath 将 URI 转换回文件路径
func URIToPath(uri string) string {
	path := strings.TrimPrefix(uri, "file://")
	if len(path) > 2 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path)
}

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
