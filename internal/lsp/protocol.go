package lsp

// LSP protocol 最小编码类型，够实现五个工具：
//   - textDocument/definition
//   - textDocument/references
//   - textDocument/hover
//   - textDocument/documentSymbol
//   - workspace/symbol

// Position 表示文件中的一个位置（0-based line/character）
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range 表示文件中的一个区间
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location 表示一个源代码位置
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// TextDocumentIdentifier 通过 URI 标识一个文本文档
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// TextDocumentPositionParams 带位置的文档标识
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// ReferenceContext 控制 references 请求的上下文
type ReferenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

// ReferenceParams 是 references 请求的参数
type ReferenceParams struct {
	TextDocumentPositionParams
	Context ReferenceContext `json:"context"`
}

// MarkupContent 是 hover 响应的 markdown 内容
type MarkupContent struct {
	Kind  string `json:"kind"` // "markdown" 或 "plaintext"
	Value string `json:"value"`
}

// Hover 是 hover 请求的响应
type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

// SymbolKind 是文档符号的种类
type SymbolKind int

const (
	SymbolKindFile      SymbolKind = 1
	SymbolKindModule    SymbolKind = 2
	SymbolKindNamespace SymbolKind = 3
	SymbolKindPackage   SymbolKind = 4
	SymbolKindClass     SymbolKind = 5
	SymbolKindMethod    SymbolKind = 6
	SymbolKindProperty  SymbolKind = 7
	SymbolKindField     SymbolKind = 8
	SymbolKindConst     SymbolKind = 9
	SymbolKindVariable  SymbolKind = 13
	SymbolKindFunction  SymbolKind = 12
	SymbolKindInterface SymbolKind = 11
	SymbolKindEnum      SymbolKind = 10
	SymbolKindString    SymbolKind = 15
	SymbolKindNumber    SymbolKind = 16
	SymbolKindBoolean   SymbolKind = 17
	SymbolKindArray     SymbolKind = 18
	SymbolKindObject    SymbolKind = 19
	SymbolKindStruct    SymbolKind = 22
	SymbolKindTypeParam SymbolKind = 26
)

var symbolKindNames = map[SymbolKind]string{
	SymbolKindFile:      "file",
	SymbolKindModule:    "module",
	SymbolKindNamespace: "namespace",
	SymbolKindPackage:   "package",
	SymbolKindClass:     "class",
	SymbolKindMethod:    "method",
	SymbolKindProperty:  "property",
	SymbolKindField:     "field",
	SymbolKindConst:     "const",
	SymbolKindVariable:  "var",
	SymbolKindFunction:  "func",
	SymbolKindInterface: "interface",
	SymbolKindEnum:      "enum",
	SymbolKindString:    "string",
	SymbolKindNumber:    "number",
	SymbolKindBoolean:   "bool",
	SymbolKindArray:     "array",
	SymbolKindObject:    "object",
	SymbolKindStruct:    "struct",
	SymbolKindTypeParam: "typeParam",
}

// SymbolKindName 返回 SymbolKind 的可读名称
func (k SymbolKind) String() string {
	if name, ok := symbolKindNames[k]; ok {
		return name
	}
	return "unknown"
}

// DocumentSymbol 是 textDocument/documentSymbol 响应的层级符号
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           SymbolKind       `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// SymbolInformation 是 workspace/symbol 响应的扁平符号信息
type SymbolInformation struct {
	Name     string     `json:"name"`
	Kind     SymbolKind `json:"kind"`
	Location Location   `json:"location"`
	// ContainerName 仅部分 server 返回
	ContainerName string `json:"containerName,omitempty"`
}

// DiagnosticSeverity 诊断严重程度
type DiagnosticSeverity int

const (
	SeverityError       DiagnosticSeverity = 1
	SeverityWarning     DiagnosticSeverity = 2
	SeverityInformation DiagnosticSeverity = 3
	SeverityHint        DiagnosticSeverity = 4
)

func (s DiagnosticSeverity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityInformation:
		return "info"
	case SeverityHint:
		return "hint"
	default:
		return "unknown"
	}
}

// Diagnostic 表示一个诊断信息（错误/警告）
type Diagnostic struct {
	Range    Range              `json:"range"`
	Severity DiagnosticSeverity `json:"severity,omitempty"`
	Code     string             `json:"code,omitempty"`
	Source   string             `json:"source,omitempty"`
	Message  string             `json:"message"`
}

// PublishDiagnosticsParams 是 textDocument/publishDiagnostics 通知的参数
type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// InitializeResult 是 initialize 响应的 result
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
}

// ServerCapabilities 是服务器能力声明
type ServerCapabilities struct {
	TextDocumentSync   interface{}     `json:"textDocumentSync,omitempty"`
	DefinitionProvider interface{}     `json:"definitionProvider,omitempty"`
	ReferencesProvider interface{}     `json:"referencesProvider,omitempty"`
	HoverProvider      interface{}     `json:"hoverProvider,omitempty"`
	DocumentSymbolProvider interface{} `json:"documentSymbolProvider,omitempty"`
	WorkspaceSymbolProvider interface{} `json:"workspaceSymbolProvider,omitempty"`
}
