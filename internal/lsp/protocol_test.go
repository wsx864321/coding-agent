package lsp

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestPosition(t *testing.T) {
	p := Position{Line: 10, Character: 5}
	if p.Line != 10 || p.Character != 5 {
		t.Error("position fields mismatch")
	}
}

func TestSymbolKindNames(t *testing.T) {
	if SymbolKindFunction.String() != "func" {
		t.Errorf("Function: got %q", SymbolKindFunction.String())
	}
	if SymbolKindClass.String() != "class" {
		t.Errorf("Class: got %q", SymbolKindClass.String())
	}
	if SymbolKind(999).String() != "unknown" {
		t.Error("unknown kind should return 'unknown'")
	}
}

func TestDiagnosticSeverity(t *testing.T) {
	if SeverityError.String() != "error" {
		t.Error("error severity mismatch")
	}
	if SeverityWarning.String() != "warning" {
		t.Error("warning severity mismatch")
	}
}

func TestMarshalTextDocumentPositionParams(t *testing.T) {
	params := TextDocumentPositionParams{
		TextDocument: TextDocumentIdentifier{URI: "file:///test.go"},
		Position:     Position{Line: 10, Character: 5},
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "file:///test.go") {
		t.Error("missing uri in marshal")
	}
}

func TestUnmarshalLocation(t *testing.T) {
	raw := `{"uri":"file:///test.go","range":{"start":{"line":1,"character":0},"end":{"line":1,"character":10}}}`
	var loc Location
	if err := json.Unmarshal([]byte(raw), &loc); err != nil {
		t.Fatal(err)
	}
	if loc.URI != "file:///test.go" {
		t.Errorf("uri: got %q", loc.URI)
	}
	if loc.Range.Start.Line != 1 {
		t.Errorf("line: got %d", loc.Range.Start.Line)
	}
}

func TestUnmarshalDocumentSymbol(t *testing.T) {
	raw := `{"name":"MyFunc","kind":12,"range":{"start":{"line":1,"character":0},"end":{"line":3,"character":1}},"selectionRange":{"start":{"line":1,"character":0},"end":{"line":1,"character":6}},"children":[]}`
	var sym DocumentSymbol
	if err := json.Unmarshal([]byte(raw), &sym); err != nil {
		t.Fatal(err)
	}
	if sym.Name != "MyFunc" {
		t.Errorf("name: got %q", sym.Name)
	}
	if sym.Kind != SymbolKindFunction {
		t.Errorf("kind: got %d", sym.Kind)
	}
}

func TestUnmarshalDiagnostic(t *testing.T) {
	raw := `{"range":{"start":{"line":1,"character":0},"end":{"line":1,"character":10}},"severity":1,"message":"syntax error"}`
	var d Diagnostic
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatal(err)
	}
	if d.Severity != SeverityError {
		t.Errorf("severity: got %d", d.Severity)
	}
	if d.Message != "syntax error" {
		t.Errorf("message: got %q", d.Message)
	}
}

func TestPathToURI(t *testing.T) {
	// 测试绝对路径转 URI
	abs := t.TempDir()
	uri := pathToURI(abs)
	if !strings.HasPrefix(uri, "file:///") {
		t.Errorf("pathToURI(%q) = %q, want prefix file:///", abs, uri)
	}
	if !strings.Contains(uri, filepath.Base(abs)) {
		t.Errorf("URI %q should contain %q", uri, filepath.Base(abs))
	}
}

func TestURIToPath(t *testing.T) {
	// Unix-style
	p := URIToPath("file:///home/user/test.go")
	if filepath.ToSlash(p) != "/home/user/test.go" {
		t.Errorf("unix: got %q", p)
	}

	// Windows-style (回退 /C: → C:)
	p = URIToPath("file:///C:/Users/test.go")
	if !strings.Contains(filepath.ToSlash(p), "C:/") {
		t.Errorf("windows: got %q", p)
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		uri  string
		want string
	}{
		{"file:///test.go", "go"},
		{"file:///test.ts", "typescript"},
		{"file:///test.tsx", "typescriptreact"},
		{"file:///test.js", "javascript"},
		{"file:///test.py", "python"},
		{"file:///test.rs", "rust"},
		{"file:///test.unknown", "plaintext"},
	}
	for _, tt := range tests {
		got := detectLanguage(tt.uri)
		if got != tt.want {
			t.Errorf("detectLanguage(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}

func TestLanguageConfig(t *testing.T) {
	for _, lang := range defaultLanguages {
		if lang.Name == "" {
			t.Error("language config has empty name")
		}
		if len(lang.Extensions) == 0 {
			t.Errorf("%s: no extensions", lang.Name)
		}
		if lang.Command == "" {
			t.Errorf("%s: no command", lang.Name)
		}
	}
}
