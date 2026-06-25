package tools

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/wsx864321/coding-agent/internal/lsp"
)

func TestLSPDefinitionTool_Name(t *testing.T) {
	tool := NewLSPDefinitionTool(nil)
	if tool.Name() != "lsp_definition" {
		t.Errorf("Name: got %q", tool.Name())
	}
	if !tool.ReadOnly() {
		t.Error("lsp_definition should be read-only")
	}
}

func TestLSPDefinitionTool_NoManager(t *testing.T) {
	tool := NewLSPDefinitionTool(nil)
	_, err := tool.Execute(context.Background(), map[string]any{
		"file":   "test.go",
		"line":   10,
		"symbol": "foo",
	})
	if err == nil {
		t.Error("expected error when manager is nil")
	}
}

func TestLSPDefinitionTool_ManagerNotAvailable(t *testing.T) {
	mgr := lsp.NewManager(t.TempDir())
	// Don't call Start() — no LSP server running
	tool := NewLSPDefinitionTool(mgr)
	result, err := tool.Execute(context.Background(), map[string]any{
		"file":   "test.go",
		"line":   10,
		"symbol": "foo",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "未启动") {
		t.Errorf("result should mention server not started: %q", result)
	}
}

func TestLSPReferencesTool_Name(t *testing.T) {
	tool := NewLSPReferencesTool(nil)
	if tool.Name() != "lsp_references" {
		t.Errorf("Name: got %q", tool.Name())
	}
}

func TestLSPHoverTool_Name(t *testing.T) {
	tool := NewLSPHoverTool(nil)
	if tool.Name() != "lsp_hover" {
		t.Errorf("Name: got %q", tool.Name())
	}
}

func TestLSPDiagnosticsTool_Name(t *testing.T) {
	tool := NewLSPDiagnosticsTool(nil)
	if tool.Name() != "lsp_diagnostics" {
		t.Errorf("Name: got %q", tool.Name())
	}
}

func TestLSPDiagnosticsTool_NoManager(t *testing.T) {
	tool := NewLSPDiagnosticsTool(nil)
	_, err := tool.Execute(context.Background(), map[string]any{
		"file": "test.go",
	})
	if err == nil {
		t.Error("expected error when manager is nil")
	}
}

func TestCodeIndexTool_Name(t *testing.T) {
	tool := NewCodeIndexTool(nil)
	if tool.Name() != "code_index" {
		t.Errorf("Name: got %q", tool.Name())
	}
}

func TestCodeIndexTool_InvalidOp(t *testing.T) {
	tool := NewCodeIndexTool(nil)
	_, err := tool.Execute(context.Background(), map[string]any{
		"action": "invalid",
	})
	if err == nil {
		t.Error("expected error for invalid action")
	}
}

func TestCodeIndexTool_OutlineMissingPath(t *testing.T) {
	mgr := lsp.NewManager(t.TempDir())
	tool := NewCodeIndexTool(mgr)
	_, err := tool.Execute(context.Background(), map[string]any{
		"action": "outline",
	})
	if err == nil {
		t.Error("expected error for missing path in outline")
	}
}

func TestCodeIndexTool_SearchMissingQuery(t *testing.T) {
	mgr := lsp.NewManager(t.TempDir())
	tool := NewCodeIndexTool(mgr)
	_, err := tool.Execute(context.Background(), map[string]any{
		"action": "search",
	})
	if err == nil {
		t.Error("expected error for missing query in search")
	}
}

func TestLSPTools_Schema(t *testing.T) {
	tools := []struct {
		name string
		t    Tool
	}{
		{"lsp_definition", NewLSPDefinitionTool(nil)},
		{"lsp_references", NewLSPReferencesTool(nil)},
		{"lsp_hover", NewLSPHoverTool(nil)},
		{"lsp_diagnostics", NewLSPDiagnosticsTool(nil)},
		{"code_index", NewCodeIndexTool(nil)},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			schema := tt.t.Schema()
			if len(schema) == 0 {
				t.Error("schema should not be empty")
			}
			if !strings.Contains(string(schema), "type") {
				t.Error("schema should contain 'type'")
			}
		})
	}
}

func TestColInLine(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.go"
	content := "package main\nfunc foo() {\n\tbar()\n}\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	col := colInLine(path, "foo", 2)
	if col != 5 {
		t.Errorf("colInLine for 'foo' at line 2: got %d, want 5", col)
	}
}

func TestMatchKind(t *testing.T) {
	if !matchKind("func", "function") {
		t.Error("'func' should match 'function'")
	}
	if !matchKind("function", "func") {
		t.Error("'function' should match 'func'")
	}
	if matchKind("class", "func") {
		t.Error("'class' should not match 'func'")
	}
}
