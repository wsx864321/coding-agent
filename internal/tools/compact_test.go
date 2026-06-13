package tools

import (
	"context"
	"testing"
)

func TestCompactTool_Basics(t *testing.T) {
	tool := NewCompactTool()
	if tool.Name() != "compact" {
		t.Fatalf("Name() = %q, want compact", tool.Name())
	}
	if len(tool.Schema()) == 0 {
		t.Fatal("Schema() should not be empty")
	}
	out, err := tool.Execute(context.Background(), map[string]any{"focus": "keep decisions"})
	if err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	if out == "" {
		t.Fatal("Execute() output should not be empty")
	}
}
