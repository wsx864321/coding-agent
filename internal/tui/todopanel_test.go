package tui

import (
	"strings"
	"testing"
)

func TestParseTodoItemsEmpty(t *testing.T) {
	items := parseTodoItems("")
	if len(items) != 0 {
		t.Fatalf("parseTodoItems(\"\") = %d items, want 0", len(items))
	}
}

func TestParseTodoItemsValid(t *testing.T) {
	raw := `[{"content":"task1","status":"pending","activeForm":"task1"},{"content":"task2","status":"in_progress","activeForm":"task2"},{"content":"task3","status":"completed","activeForm":"task3"}]`
	items := parseTodoItems(raw)
	if len(items) != 3 {
		t.Fatalf("parseTodoItems = %d items, want 3", len(items))
	}
	if items[0].Content != "task1" || items[0].Status != "pending" {
		t.Fatalf("item[0] = %+v", items[0])
	}
	if items[1].Status != "in_progress" {
		t.Fatalf("item[1].Status = %q, want in_progress", items[1].Status)
	}
	if items[2].Status != "completed" {
		t.Fatalf("item[2].Status = %q, want completed", items[2].Status)
	}
}

func TestParseTodoItemsInvalidJSON(t *testing.T) {
	items := parseTodoItems(`not json`)
	if len(items) != 0 {
		t.Fatalf("parseTodoItems(invalid) = %d items, want 0", len(items))
	}
}

func TestRenderTodoPanel(t *testing.T) {
	items := []todoItem{
		{Content: "task1", Status: "pending"},
		{Content: "task2", Status: "in_progress", ActiveForm: "testing"},
		{Content: "task3", Status: "completed"},
	}
	out := renderTodoPanel(items)
	if out == "" {
		t.Fatal("renderTodoPanel returned empty")
	}
	if !strings.Contains(out, "task1") || !strings.Contains(out, "task2") || !strings.Contains(out, "task3") {
		t.Fatalf("renderTodoPanel missing tasks: %s", out)
	}
	if !strings.Contains(out, "⏳") {
		t.Fatalf("renderTodoPanel missing pending icon: %s", out)
	}
	if !strings.Contains(out, "⟳") {
		t.Fatalf("renderTodoPanel missing in_progress icon: %s", out)
	}
	if !strings.Contains(out, "✓") {
		t.Fatalf("renderTodoPanel missing completed icon: %s", out)
	}
}

func TestRenderTodoPanelEmpty(t *testing.T) {
	if out := renderTodoPanel(nil); out != "" {
		t.Fatalf("renderTodoPanel(nil) = %q, want empty", out)
	}
	if out := renderTodoPanel([]todoItem{}); out != "" {
		t.Fatalf("renderTodoPanel([]) = %q, want empty", out)
	}
}

func TestTodoStatusIcon(t *testing.T) {
	tests := []struct {
		status, want string
	}{
		{"pending", "⏳"},
		{"in_progress", "⟳"},
		{"completed", "✓"},
		{"unknown", "⏳"},
		{"", "⏳"},
	}
	for _, tc := range tests {
		got := todoStatusIcon(tc.status)
		if got != tc.want {
			t.Fatalf("todoStatusIcon(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
}
