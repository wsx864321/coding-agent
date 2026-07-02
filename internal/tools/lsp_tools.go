package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wsx864321/coding-agent/internal/lsp"
)

// ---------- lsp_definition ----------

// LSPDefinitionTool 跳转到符号定义
type LSPDefinitionTool struct {
	manager     *lsp.Manager
	allowedDirs []string
}

func NewLSPDefinitionTool(manager *lsp.Manager, allowedDirs []string) *LSPDefinitionTool {
	return &LSPDefinitionTool{manager: manager, allowedDirs: allowedDirs}
}

func (t *LSPDefinitionTool) ReadOnly() bool { return true }
func (t *LSPDefinitionTool) Name() string   { return "lsp_definition" }
func (t *LSPDefinitionTool) Description() string {
	return "跳转到符号的定义位置。传入源文件路径、行号和符号名，返回定义所在的文件和位置。"
}

func (t *LSPDefinitionTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file": map[string]any{
				"type":        "string",
				"description": "源文件路径（相对于 workspace 根目录或绝对路径）",
			},
			"line": map[string]any{
				"type":        "integer",
				"description": "符号所在的 1-based 行号",
			},
			"symbol": map[string]any{
				"type":        "string",
				"description": "该行上的符号文本，用于定位列号",
			},
		},
		"required": []string{"file", "line", "symbol"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

type lspPosArgs struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Symbol string `json:"symbol"`
}

func (t *LSPDefinitionTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	var p lspPosArgs
	if err := decode(args, &p); err != nil {
		return "", err
	}
	if err := checkLSPPath(t.allowedDirs, p.File); err != nil {
		return "", err
	}
	if t.manager == nil {
		return "", fmt.Errorf("LSP 管理器未初始化")
	}
	if !t.manager.IsAvailable() {
		return "LSP server 未启动。请确认已安装对应语言的语言服务器（如 gopls、typescript-language-server、pyright 等）。", nil
	}

	loc, err := t.manager.Definition(p.File, p.Line-1, colInLine(p.File, p.Symbol, p.Line))
	if err != nil {
		return formatLSPError(err), nil
	}
	return formatLocations(loc), nil
}

// ---------- lsp_references ----------

// LSPReferencesTool 查找所有引用
type LSPReferencesTool struct {
	manager     *lsp.Manager
	allowedDirs []string
}

func NewLSPReferencesTool(manager *lsp.Manager, allowedDirs []string) *LSPReferencesTool {
	return &LSPReferencesTool{manager: manager, allowedDirs: allowedDirs}
}

func (t *LSPReferencesTool) ReadOnly() bool { return true }
func (t *LSPReferencesTool) Name() string   { return "lsp_references" }
func (t *LSPReferencesTool) Description() string {
	return "列出符号在项目中的所有引用位置。"
}

func (t *LSPReferencesTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file": map[string]any{
				"type":        "string",
				"description": "源文件路径",
			},
			"line": map[string]any{
				"type":        "integer",
				"description": "符号所在的 1-based 行号",
			},
			"symbol": map[string]any{
				"type":        "string",
				"description": "该行上的符号文本，用于定位列号",
			},
		},
		"required": []string{"file", "line", "symbol"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

func (t *LSPReferencesTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	var p lspPosArgs
	if err := decode(args, &p); err != nil {
		return "", err
	}
	if err := checkLSPPath(t.allowedDirs, p.File); err != nil {
		return "", err
	}
	if t.manager == nil {
		return "", fmt.Errorf("LSP 管理器未初始化")
	}
	if !t.manager.IsAvailable() {
		return "LSP server 未启动。", nil
	}

	locs, err := t.manager.References(p.File, p.Line-1, colInLine(p.File, p.Symbol, p.Line))
	if err != nil {
		return formatLSPError(err), nil
	}
	if len(locs) == 0 {
		return "未找到引用。", nil
	}
	return fmt.Sprintf("找到 %d 个引用：\n%s", len(locs), formatLocations(locs)), nil
}

// ---------- lsp_hover ----------

// LSPHoverTool 获取符号类型和文档
type LSPHoverTool struct {
	manager     *lsp.Manager
	allowedDirs []string
}

func NewLSPHoverTool(manager *lsp.Manager, allowedDirs []string) *LSPHoverTool {
	return &LSPHoverTool{manager: manager, allowedDirs: allowedDirs}
}

func (t *LSPHoverTool) ReadOnly() bool { return true }
func (t *LSPHoverTool) Name() string   { return "lsp_hover" }
func (t *LSPHoverTool) Description() string {
	return "显示符号的类型签名和文档注释。传入文件、行号、符号名即可获取类型信息。"
}

func (t *LSPHoverTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file": map[string]any{
				"type":        "string",
				"description": "源文件路径",
			},
			"line": map[string]any{
				"type":        "integer",
				"description": "符号所在的 1-based 行号",
			},
			"symbol": map[string]any{
				"type":        "string",
				"description": "该行上的符号文本，用于定位列号",
			},
		},
		"required": []string{"file", "line", "symbol"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

func (t *LSPHoverTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	var p lspPosArgs
	if err := decode(args, &p); err != nil {
		return "", err
	}
	if err := checkLSPPath(t.allowedDirs, p.File); err != nil {
		return "", err
	}
	if t.manager == nil {
		return "", fmt.Errorf("LSP 管理器未初始化")
	}
	if !t.manager.IsAvailable() {
		return "LSP server 未启动。", nil
	}

	h, err := t.manager.Hover(p.File, p.Line-1, colInLine(p.File, p.Symbol, p.Line))
	if err != nil {
		return formatLSPError(err), nil
	}
	if h == nil {
		return "无可用信息。", nil
	}

	// 返回 markdown 格式的 hover 内容
	if h.Contents.Value == "" {
		return "无可用信息。", nil
	}
	return h.Contents.Value, nil
}

// ---------- lsp_diagnostics ----------

// LSPDiagnosticsTool 获取文件诊断信息
type LSPDiagnosticsTool struct {
	manager     *lsp.Manager
	allowedDirs []string
}

func NewLSPDiagnosticsTool(manager *lsp.Manager, allowedDirs []string) *LSPDiagnosticsTool {
	return &LSPDiagnosticsTool{manager: manager, allowedDirs: allowedDirs}
}

func (t *LSPDiagnosticsTool) ReadOnly() bool { return true }
func (t *LSPDiagnosticsTool) Name() string   { return "lsp_diagnostics" }
func (t *LSPDiagnosticsTool) Description() string {
	return "报告文件中的编译/静态分析诊断信息（错误、警告等）。传入文件路径即可获取所有诊断。"
}

func (t *LSPDiagnosticsTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file": map[string]any{
				"type":        "string",
				"description": "要检查的文件路径",
			},
		},
		"required": []string{"file"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

type lspFileArg struct {
	File string `json:"file"`
}

func (t *LSPDiagnosticsTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	var p lspFileArg
	if err := decode(args, &p); err != nil {
		return "", err
	}
	if err := checkLSPPath(t.allowedDirs, p.File); err != nil {
		return "", err
	}
	if t.manager == nil {
		return "", fmt.Errorf("LSP 管理器未初始化")
	}
	if !t.manager.IsAvailable() {
		return "LSP server 未启动。", nil
	}

	diags, err := t.manager.GetDiagnostics(p.File)
	if err != nil {
		return formatLSPError(err), nil
	}
	if len(diags) == 0 {
		return "没有诊断问题。", nil
	}

	abs, _ := filepath.Abs(p.File)
	base := filepath.Base(abs)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d 个诊断问题：\n\n", len(diags)))
	for _, d := range diags {
		b.WriteString(fmt.Sprintf("- %s:%d:%d [%s] %s\n",
			base, d.Range.Start.Line+1, d.Range.Start.Character+1,
			d.Severity.String(), d.Message))
	}
	return b.String(), nil
}

// ---------- code_index ----------

// CodeIndexTool 提供轻量级符号索引
type CodeIndexTool struct {
	manager     *lsp.Manager
	allowedDirs []string
}

func NewCodeIndexTool(manager *lsp.Manager, allowedDirs []string) *CodeIndexTool {
	return &CodeIndexTool{manager: manager, allowedDirs: allowedDirs}
}

func (t *CodeIndexTool) ReadOnly() bool { return true }
func (t *CodeIndexTool) Name() string   { return "code_index" }
func (t *CodeIndexTool) Description() string {
	return "轻量级代码符号索引。支持两种操作：outline（列出文件内的符号大纲）和 search（跨项目搜索符号）。"
}

type codeIndexArgs struct {
	Action string `json:"action"`           // "outline" 或 "search"
	Path   string `json:"path,omitempty"`   // outline: 文件路径；search: 可选搜索范围
	Query  string `json:"query,omitempty"`  // search: 搜索关键词
	Kind   string `json:"kind,omitempty"`   // outline: 可选符号类型过滤（func, class, method 等）
	Limit  int    `json:"limit,omitempty"`  // 最大返回数
}

func (t *CodeIndexTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"description": "操作类型：outline（列出文件符号大纲）或 search（跨项目搜索符号）",
				"enum":        []string{"outline", "search"},
			},
			"path": map[string]any{
				"type":        "string",
				"description": "outline 时：文件或目录路径（默认 '.'）；search 时忽略",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "search 时：搜索关键词（符号名或子串）",
			},
			"kind": map[string]any{
				"type":        "string",
				"description": "outline 时：可选符号类型过滤（func, method, class, type, interface, const, var, struct, enum, trait）",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "最大返回符号数（默认 100，最大 200）",
			},
		},
		"required": []string{"action"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

func (t *CodeIndexTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	var p codeIndexArgs
	if err := decode(args, &p); err != nil {
		return "", err
	}
	if p.Path != "" && p.Path != "." {
		if err := checkLSPPath(t.allowedDirs, p.Path); err != nil {
			return "", err
		}
	}
	if t.manager == nil {
		return "", fmt.Errorf("LSP 管理器未初始化")
	}

	switch p.Action {
	case "outline":
		return t.outline(ctx, p)
	case "search":
		return t.search(ctx, p)
	default:
		return "", fmt.Errorf("不支持的操作: %q，请使用 outline 或 search", p.Action)
	}
}

func (t *CodeIndexTool) outline(ctx context.Context, p codeIndexArgs) (string, error) {
	file := p.Path
	if file == "" || file == "." {
		return "", fmt.Errorf("outline 操作需要指定 path 参数（文件路径）")
	}

	syms, err := t.manager.DocumentSymbols(file)
	if err != nil {
		return formatLSPError(err), nil
	}
	if len(syms) == 0 {
		return "文件中没有发现符号。", nil
	}

	limit := p.Limit
	if limit <= 0 || limit > 200 {
		limit = 200
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s 中的符号：\n\n", filepath.Base(file)))
	formatDocSymbols(&b, syms, "", 0, limit, p.Kind)
	return b.String(), nil
}

func (t *CodeIndexTool) search(ctx context.Context, p codeIndexArgs) (string, error) {
	if p.Query == "" {
		return "", fmt.Errorf("search 操作需要指定 query 参数")
	}

	syms, err := t.manager.WorkspaceSymbols(p.Query)
	if err != nil {
		return formatLSPError(err), nil
	}
	if len(syms) == 0 {
		return fmt.Sprintf("未找到匹配 %q 的符号。", p.Query), nil
	}

	limit := p.Limit
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	if len(syms) > limit {
		syms = syms[:limit]
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("找到 %d 个匹配 %q 的符号：\n\n", len(syms), p.Query))
	for _, si := range syms {
		path := lsp.URIToPath(si.Location.URI)
		b.WriteString(fmt.Sprintf("  %s:%d:%d  %s %s\n",
			filepath.Base(path),
			si.Location.Range.Start.Line+1,
			si.Location.Range.Start.Character,
			si.Kind.String(),
			si.Name))
	}
	return b.String(), nil
}

// ---------- helpers ----------

// checkLSPPath 校验 LSP 工具的文件参数是否在白名单内。
// allowedDirs 为空时跳过校验（向后兼容测试等场景）。
func checkLSPPath(allowedDirs []string, file string) error {
	if len(allowedDirs) == 0 {
		return nil
	}
	ok, err := isInAllowedDirs(file, allowedDirs)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("路径 %q 不在允许的目录范围内", file)
	}
	return nil
}

func decode(args map[string]any, target interface{}) error {
	raw, _ := json.Marshal(args)
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("解析参数失败: %w", err)
	}
	return nil
}

func formatLocations(locs []lsp.Location) string {
	if len(locs) == 0 {
		return "未找到定义。"
	}
	var b strings.Builder
	for _, loc := range locs {
		path := lsp.URIToPath(loc.URI)
		b.WriteString(fmt.Sprintf("  %s:%d:%d\n",
			path,
			loc.Range.Start.Line+1,
			loc.Range.Start.Character))
	}
	return b.String()
}

func formatLSPError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.Contains(msg, "no LSP server") {
		return "当前文件类型没有可用的语言服务器。请安装对应语言服务器（如 gopls、typescript-language-server、pyright 等）。"
	}
	return fmt.Sprintf("LSP 错误: %s", msg)
}

func formatDocSymbols(b *strings.Builder, syms []lsp.DocumentSymbol, indent string, depth int, limit int, kindFilter string) {
	if depth > 3 || b.Len() > limit*200 {
		return
	}
	for _, s := range syms {
		kindName := s.Kind.String()
		if kindFilter != "" && !matchKind(kindFilter, kindName) {
			goto children
		}
		b.WriteString(fmt.Sprintf("%s%s  %s:%d  %s",
			indent, kindName, s.Name, s.Range.Start.Line+1, s.Detail))
		if s.Detail != "" {
			b.WriteString(fmt.Sprintf(" (%s)", s.Detail))
		}
		b.WriteString("\n")

	children:
		if len(s.Children) > 0 {
			formatDocSymbols(b, s.Children, indent+"  ", depth+1, limit, kindFilter)
		}
	}
}

func matchKind(filter, kind string) bool {
	// 简单子串匹配，支持用户简写
	f := strings.ToLower(filter)
	return strings.Contains(kind, f) || strings.Contains(f, kind)
}

// colInLine 在给定行的文本中查找符号的列号（0-based）
func colInLine(file string, symbol string, line int) int {
	if symbol == "" || line <= 0 {
		return 0
	}
	content, err := readLine(file, line)
	if err != nil {
		return 0
	}
	idx := strings.Index(content, symbol)
	if idx < 0 {
		return 0
	}
	return idx
}

// readLine 读取文件中指定行的文本（1-based）
func readLine(file string, line int) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	if line-1 >= len(lines) {
		return "", fmt.Errorf("line %d out of range", line)
	}
	return lines[line-1], nil
}
