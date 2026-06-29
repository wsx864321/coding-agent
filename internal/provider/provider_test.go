package provider

import (
	"fmt"
	"strings"
	"testing"
)

// chunkStream 便捷构造用于测试的 chunk channel。
func chunkStream(chunks ...Chunk) <-chan Chunk {
	ch := make(chan Chunk, len(chunks))
	for _, c := range chunks {
		ch <- c
	}
	close(ch)
	return ch
}

func TestCollectWithText_StandardToolCall(t *testing.T) {
	// 标准 OpenAI 风格：Start → Delta → Delta → Done
	ch := chunkStream(
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_1", Name: "task", Arguments: `{"`}},
		Chunk{Type: ChunkToolCallDelta, ToolCall: &ToolCall{Arguments: `prompt`}},
		Chunk{Type: ChunkToolCallDelta, ToolCall: &ToolCall{Arguments: `":"hi"}`}},
		Chunk{Type: ChunkDone},
	)
	msg, _, err := CollectWithText(ch, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 ToolCall, got %d: %+v", len(msg.ToolCalls), msg.ToolCalls)
	}
	tc := msg.ToolCalls[0]
	if tc.ID != "call_1" {
		t.Errorf("ID = %q, want call_1", tc.ID)
	}
	if tc.Name != "task" {
		t.Errorf("Name = %q, want task", tc.Name)
	}
	if tc.Arguments != `{"prompt":"hi"}` {
		t.Errorf("Arguments = %q, want {\"prompt\":\"hi\"}", tc.Arguments)
	}
}

func TestCollectWithText_DuplicateIDs(t *testing.T) {
	// DeepSeek 风格：每个 delta 都带 ID 和 Name，全以 ChunkToolCallStart 发出
	// 模拟中文：每个字符一个帧，23 帧
	ch := chunkStream(
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `{`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `"`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `d`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `e`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `s`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `c`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `r`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `i`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `p`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `t`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `i`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `o`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `n`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `"`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `:`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `"`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `探`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `索`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `项`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `目`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `"`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_00", Name: "task", Arguments: `}`}},
		Chunk{Type: ChunkDone},
	)
	msg, _, err := CollectWithText(ch, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 ToolCall, got %d: %+v", len(msg.ToolCalls), msg.ToolCalls)
	}
	tc := msg.ToolCalls[0]
	if tc.ID != "call_00" {
		t.Errorf("ID = %q, want call_00", tc.ID)
	}
	if tc.Name != "task" {
		t.Errorf("Name = %q, want task", tc.Name)
	}
	expected := `{"description":"探索项目"}`
	if tc.Arguments != expected {
		t.Errorf("Arguments = %q, want %q", tc.Arguments, expected)
	}
}

func TestCollectWithText_MultipleToolCalls(t *testing.T) {
	// 两个不同的 tool call，第一个完整，第二个被切成多个带 ID 的帧
	ch := chunkStream(
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_a", Name: "read_file", Arguments: `{"path":"f1"}`}},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "call_b", Name: "bash", Arguments: `{"`}},
		Chunk{Type: ChunkToolCallDelta, ToolCall: &ToolCall{Arguments: `cmd`}},
		Chunk{Type: ChunkToolCallDelta, ToolCall: &ToolCall{Arguments: `":"ls"}`}},
		Chunk{Type: ChunkDone},
	)
	msg, _, err := CollectWithText(ch, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msg.ToolCalls) != 2 {
		t.Fatalf("expected 2 ToolCalls, got %d: %+v", len(msg.ToolCalls), msg.ToolCalls)
	}
	if msg.ToolCalls[0].ID != "call_a" {
		t.Errorf("tc[0].ID = %q, want call_a", msg.ToolCalls[0].ID)
	}
	if msg.ToolCalls[0].Arguments != `{"path":"f1"}` {
		t.Errorf("tc[0].Arguments = %q", msg.ToolCalls[0].Arguments)
	}
	if msg.ToolCalls[1].ID != "call_b" {
		t.Errorf("tc[1].ID = %q, want call_b", msg.ToolCalls[1].ID)
	}
	if msg.ToolCalls[1].Arguments != `{"cmd":"ls"}` {
		t.Errorf("tc[1].Arguments = %q", msg.ToolCalls[1].Arguments)
	}
}

func TestCollectWithText_TextAndToolCalls(t *testing.T) {
	ch := chunkStream(
		Chunk{Type: ChunkText, Text: "Let me "},
		Chunk{Type: ChunkText, Text: "think..."},
		Chunk{Type: ChunkToolCallStart, ToolCall: &ToolCall{ID: "c1", Name: "echo", Arguments: `{"`}},
		Chunk{Type: ChunkToolCallDelta, ToolCall: &ToolCall{Arguments: `x`}},
		Chunk{Type: ChunkToolCallDelta, ToolCall: &ToolCall{Arguments: `":"y"}`}},
		Chunk{Type: ChunkDone},
	)
	msg, _, err := CollectWithText(ch, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Content != "Let me think..." {
		t.Errorf("Content = %q, want 'Let me think...'", msg.Content)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 ToolCall, got %d", len(msg.ToolCalls))
	}
}

func TestCollectWithText_OnText(t *testing.T) {
	var sb strings.Builder
	onText := func(s string) { sb.WriteString(s) }
	ch := chunkStream(
		Chunk{Type: ChunkText, Text: "hello "},
		Chunk{Type: ChunkText, Text: "world"},
		Chunk{Type: ChunkDone},
	)
	msg, _, err := CollectWithText(ch, onText)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sb.String() != "hello world" {
		t.Errorf("onText captured %q, want 'hello world'", sb.String())
	}
	if msg.Content != "hello world" {
		t.Errorf("Content = %q", msg.Content)
	}
}

func TestCollectWithText_ErrorInterrupts(t *testing.T) {
	ch := chunkStream(
		Chunk{Type: ChunkText, Text: "partial"},
		Chunk{Type: ChunkError, Err: fmt.Errorf("boom")},
	)
	_, _, err := CollectWithText(ch, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCollectWithText_Empty(t *testing.T) {
	ch := chunkStream(
		Chunk{Type: ChunkDone},
	)
	msg, _, err := CollectWithText(ch, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Content != "" {
		t.Errorf("Content = %q, want empty", msg.Content)
	}
	if len(msg.ToolCalls) != 0 {
		t.Errorf("expected 0 ToolCalls, got %d", len(msg.ToolCalls))
	}
}

func TestFindToolCallByID(t *testing.T) {
	tcs := []ToolCall{
		{ID: "aaa"},
		{ID: "bbb"},
		{ID: "ccc"},
	}
	if idx := findToolCallByID(tcs, "bbb"); idx != 1 {
		t.Errorf("findToolCallByID(bbb) = %d, want 1", idx)
	}
	if idx := findToolCallByID(tcs, "ddd"); idx != -1 {
		t.Errorf("findToolCallByID(ddd) = %d, want -1", idx)
	}
	if idx := findToolCallByID(tcs, ""); idx != -1 {
		t.Errorf("findToolCallByID('') = %d, want -1", idx)
	}
	if idx := findToolCallByID(nil, "x"); idx != -1 {
		t.Errorf("findToolCallByID nil slice = %d, want -1", idx)
	}
}
