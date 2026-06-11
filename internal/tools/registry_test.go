package tools

import (
	"testing"
)

func TestFilterRegistry_ExcludesSpecified(t *testing.T) {
	parent := NewRegistry()
	parent.Register(NewReadFileTool("."))
	parent.Register(NewTodoWriteTool())
	parent.Register(NewCompleteStepTool())
	parent.Register(NewTaskTool(nil))

	child := FilterRegistry(parent, "task", "todo_write", "complete_step")

	if child.Get("read_file") == nil {
		t.Error("child should have read_file")
	}
	for _, name := range []string{"task", "todo_write", "complete_step"} {
		if child.Get(name) != nil {
			t.Errorf("child should NOT have %q", name)
		}
	}
}

func TestFilterRegistry_EmptyExclude(t *testing.T) {
	parent := NewRegistry()
	parent.Register(NewReadFileTool("."))
	parent.Register(NewTodoWriteTool())

	child := FilterRegistry(parent)

	if len(child.List()) != len(parent.List()) {
		t.Errorf("empty exclude: child=%d, parent=%d", len(child.List()), len(parent.List()))
	}
}

func TestFilterRegistry_Independence(t *testing.T) {
	parent := NewRegistry()
	parent.Register(NewReadFileTool("."))

	child := FilterRegistry(parent, "nonexistent")

	child.Register(NewTodoWriteTool())
	if parent.Get("todo_write") != nil {
		t.Error("modifying child should not affect parent")
	}
}
