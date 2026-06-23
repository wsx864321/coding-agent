package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/wsx864321/coding-agent/internal/provider"
	"github.com/wsx864321/coding-agent/internal/tools"
)

type recordingEmitter struct {
	tools []string
}

func (r *recordingEmitter) OnChunk(string) {}
func (r *recordingEmitter) OnToolStart(name, args string) {
	r.tools = append(r.tools, "start:"+name+":"+args)
}
func (r *recordingEmitter) OnToolEnd(name, result string, isError bool) {
	suffix := ""
	if isError {
		suffix = ":error"
	}
	r.tools = append(r.tools, "end:"+name+suffix)
}
func (r *recordingEmitter) OnApprovalRequest(string, map[string]any, func(bool)) {}
func (r *recordingEmitter) OnDone()                                              {}
func (r *recordingEmitter) OnError(error)                                        {}

func TestWithEmitterRoundTrip(t *testing.T) {
	rec := &recordingEmitter{}
	ctx := WithEmitter(context.Background(), rec)
	got := EmitterFromContext(ctx)
	if got != rec {
		t.Fatal("EmitterFromContext should return the same emitter")
	}
}

func TestEmitterFromContextNilWhenMissing(t *testing.T) {
	if got := EmitterFromContext(context.Background()); got != nil {
		t.Fatalf("EmitterFromContext() = %v, want nil", got)
	}
}

func TestInvokeToolEmitsStartAndEnd(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(&fakeTool{name: "read_file", readOnly: true})

	rec := &recordingEmitter{}
	a := &Agent{registry: r}
	tc := provider.ToolCall{Name: "read_file", Arguments: `{"path":"a.txt"}`}

	result := a.invokeTool(context.Background(), tc, rec)
	if !strings.Contains(result, "read_file_result") {
		t.Fatalf("result = %q, want tool output", result)
	}
	if len(rec.tools) != 2 {
		t.Fatalf("events = %v, want start+end", rec.tools)
	}
	if !strings.HasPrefix(rec.tools[0], "start:read_file:") {
		t.Fatalf("first event = %q, want start:read_file", rec.tools[0])
	}
	if rec.tools[1] != "end:read_file" {
		t.Fatalf("second event = %q, want end:read_file", rec.tools[1])
	}
}

func TestInvokeToolEmitsErrorOnUnknownTool(t *testing.T) {
	rec := &recordingEmitter{}
	a := &Agent{registry: tools.NewRegistry()}
	tc := provider.ToolCall{Name: "missing", Arguments: `{}`}

	result := a.invokeTool(context.Background(), tc, rec)
	if !strings.HasPrefix(result, "Error:") {
		t.Fatalf("result = %q, want error prefix", result)
	}
	if len(rec.tools) != 2 {
		t.Fatalf("events = %v, want start+end", rec.tools)
	}
	if rec.tools[1] != "end:missing:error" {
		t.Fatalf("end event = %q, want end:missing:error", rec.tools[1])
	}
}
