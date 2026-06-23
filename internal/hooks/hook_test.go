package hooks

import (
	"encoding/json"
	"testing"
)

func TestEventConstants(t *testing.T) {
	want := map[Event]string{
		EventPreToolUse:       "PreToolUse",
		EventPostToolUse:      "PostToolUse",
		EventUserPromptSubmit: "UserPromptSubmit",
		EventStop:             "Stop",
	}
	for ev, s := range want {
		if string(ev) != s {
			t.Errorf("Event %v = %q, want %q", ev, ev, s)
		}
	}
}

func TestScopeConstants(t *testing.T) {
	if ScopeProject != "project" {
		t.Errorf("ScopeProject = %q, want project", ScopeProject)
	}
	if ScopeGlobal != "global" {
		t.Errorf("ScopeGlobal = %q, want global", ScopeGlobal)
	}
}

func TestDecisionConstants(t *testing.T) {
	want := map[Decision]string{
		DecisionPass:  "pass",
		DecisionBlock: "block",
		DecisionWarn:  "warn",
		DecisionError: "error",
	}
	for d, s := range want {
		if string(d) != s {
			t.Errorf("Decision %v = %q, want %q", d, d, s)
		}
	}
}

func TestPayloadJSONSerialization(t *testing.T) {
	p := Payload{
		Event:      EventPreToolUse,
		Cwd:        "/tmp",
		ToolName:   "Read",
		ToolArgs:   map[string]any{"path": "foo.go"},
		ToolResult: "content",
		Prompt:     "hello",
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Payload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Event != EventPreToolUse {
		t.Errorf("Event = %q, want PreToolUse", got.Event)
	}
	if got.Cwd != "/tmp" {
		t.Errorf("Cwd = %q, want /tmp", got.Cwd)
	}
	if got.ToolName != "Read" {
		t.Errorf("ToolName = %q, want Read", got.ToolName)
	}
	if got.ToolArgs["path"] != "foo.go" {
		t.Errorf("ToolArgs[path] = %v, want foo.go", got.ToolArgs["path"])
	}
	if got.ToolResult != "content" {
		t.Errorf("ToolResult = %q, want content", got.ToolResult)
	}
	if got.Prompt != "hello" {
		t.Errorf("Prompt = %q, want hello", got.Prompt)
	}
}

func TestPayloadJSONOmitempty(t *testing.T) {
	p := Payload{Event: EventStop, Cwd: "/tmp"}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"toolName", "toolArgs", "toolResult", "prompt"} {
		if _, ok := raw[key]; ok {
			t.Errorf("expected %q to be omitted", key)
		}
	}
}
