package event

import (
	"bytes"
	"strings"
	"testing"
)

func TestTextSink_Text(t *testing.T) {
	var out, err bytes.Buffer
	s := &TextSink{Out: &out, Err: &err}
	s.Emit(Event{Kind: Text, Text: "hello"})
	if out.String() != "hello" {
		t.Fatalf("stdout = %q, want hello", out.String())
	}
	if err.Len() != 0 {
		t.Fatalf("stderr should be empty, got %q", err.String())
	}
}

func TestTextSink_ToolDispatch(t *testing.T) {
	var err bytes.Buffer
	s := &TextSink{Err: &err}
	s.Emit(Event{Kind: ToolDispatch, ToolName: "bash"})
	if !strings.Contains(err.String(), "bash") {
		t.Fatalf("stderr = %q, want tool name", err.String())
	}
}

func TestTextSink_ToolDispatch_WithArgs(t *testing.T) {
	var err bytes.Buffer
	s := &TextSink{Err: &err}
	s.Emit(Event{Kind: ToolDispatch, ToolName: "bash", ToolArgs: `{"command":"echo hello"}`})
	got := err.String()
	if !strings.Contains(got, "bash") {
		t.Fatalf("stderr = %q, want tool name", got)
	}
	if !strings.Contains(got, "echo hello") {
		t.Fatalf("stderr = %q, want args summary", got)
	}
}

func TestTextSink_ToolDispatch_TruncatesLongArgs(t *testing.T) {
	var err bytes.Buffer
	s := &TextSink{Err: &err}
	long := strings.Repeat("x", 100)
	s.Emit(Event{Kind: ToolDispatch, ToolName: "bash", ToolArgs: long})
	got := err.String()
	if !strings.Contains(got, "...") {
		t.Fatalf("stderr = %q, want truncated args", got)
	}
}

func TestTextSink_ToolResult_OK(t *testing.T) {
	var err bytes.Buffer
	s := &TextSink{Err: &err}
	s.Emit(Event{Kind: ToolResult, ToolName: "read_file", ToolIsErr: false})
	if !strings.Contains(err.String(), "read_file") {
		t.Fatalf("stderr = %q", err.String())
	}
}

func TestTextSink_ToolResult_Error(t *testing.T) {
	var err bytes.Buffer
	s := &TextSink{Err: &err}
	s.Emit(Event{Kind: ToolResult, ToolName: "bash", ToolIsErr: true})
	got := err.String()
	if !strings.Contains(got, "bash") {
		t.Fatalf("stderr = %q", got)
	}
}

func TestTextSink_ToolResult_ErrorWithOutput(t *testing.T) {
	var err bytes.Buffer
	s := &TextSink{Err: &err}
	s.Emit(Event{Kind: ToolResult, ToolName: "bash", ToolIsErr: true, ToolOutput: "exit code 1: command not found"})
	got := err.String()
	if !strings.Contains(got, "bash:") {
		t.Fatalf("stderr = %q, want tool name with colon", got)
	}
	if !strings.Contains(got, "exit code 1") {
		t.Fatalf("stderr = %q, want error summary", got)
	}
}

func TestTextSink_ToolResult_ErrorTruncatesLongOutput(t *testing.T) {
	var err bytes.Buffer
	s := &TextSink{Err: &err}
	long := strings.Repeat("e", 100)
	s.Emit(Event{Kind: ToolResult, ToolName: "bash", ToolIsErr: true, ToolOutput: long})
	got := err.String()
	if !strings.Contains(got, "...") {
		t.Fatalf("stderr = %q, want truncated error output", got)
	}
}

func TestTextSink_Notice_Info(t *testing.T) {
	var err bytes.Buffer
	s := &TextSink{Err: &err}
	s.Emit(Event{Kind: Notice, Level: LevelInfo, Text: "todo guard"})
	if !strings.Contains(err.String(), "todo guard") {
		t.Fatalf("stderr = %q", err.String())
	}
}

func TestTextSink_Notice_Warn(t *testing.T) {
	var err bytes.Buffer
	s := &TextSink{Err: &err}
	s.Emit(Event{Kind: Notice, Level: LevelWarn, Text: "hook failed"})
	if !strings.Contains(err.String(), "hook failed") {
		t.Fatalf("stderr = %q", err.String())
	}
}

func TestTextSink_ApprovalRequest_NoOutput(t *testing.T) {
	var out, err bytes.Buffer
	s := &TextSink{Out: &out, Err: &err}
	s.Emit(Event{Kind: ApprovalRequest, ApprovalName: "write_file"})
	if out.Len() != 0 || err.Len() != 0 {
		t.Fatalf("ApprovalRequest should produce no output")
	}
}

func TestTextSink_TurnDone_NoOutput(t *testing.T) {
	var out, err bytes.Buffer
	s := &TextSink{Out: &out, Err: &err}
	s.Emit(Event{Kind: TurnDone, Err: nil})
	if out.Len() != 0 || err.Len() != 0 {
		t.Fatalf("TurnDone should produce no output")
	}
}
