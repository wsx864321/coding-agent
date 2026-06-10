//go:build e2e

package e2e_test

import (
	"context"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"

	"github.com/wsx864321/coding-agent/e2e"
	"github.com/wsx864321/coding-agent/internal/agent"
	"github.com/wsx864321/coding-agent/internal/permission"
)

// 场景 1：Checker Deny → tool 不执行，tool_result 回填 "Permission denied"
func TestE2E_PermissionDeny_BlocksExecute(t *testing.T) {
	f := e2e.NewFakeLLM(t,
		e2e.ScriptedResponse{
			ToolCalls: []openai.ToolCall{
				e2e.MakeToolCall("call_1", "bash", `{"command":"rm -rf /"}`),
			},
		},
		e2e.ScriptedResponse{Content: "ok"},
	)
	a := e2e.NewTestAgent(t, f, agent.WithChecker(&permission.Pipeline{
		Deny: []permission.Checker{
			permission.NewDenyListCheckerWith(
				permission.DenyPattern{ToolName: "bash", ArgName: "command", Substr: "rm -rf /", Reason: "硬拒绝：删根目录"},
			),
		},
	}))

	out, err := a.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "ok" {
		t.Errorf("output = %q, want ok", out)
	}
	msgs := a.Messages()
	if len(msgs) < 4 {
		t.Fatalf("expected at least 4 messages, got %d", len(msgs))
	}
	if !strings.Contains(msgs[3].Content, "Permission denied") {
		t.Errorf("tool result = %q, want contains Permission denied", msgs[3].Content)
	}
	if strings.Contains(msgs[3].Content, "EXECUTED:") {
		t.Errorf("bash should not execute when denied, got %q", msgs[3].Content)
	}
}

// 场景 2：Checker Allow → tool 正常执行
func TestE2E_PermissionAllow_Executes(t *testing.T) {
	f := e2e.NewFakeLLM(t,
		e2e.ScriptedResponse{
			ToolCalls: []openai.ToolCall{
				e2e.MakeToolCall("call_1", "bash", `{"command":"echo hi"}`),
			},
		},
		e2e.ScriptedResponse{Content: "done"},
	)
	a := e2e.NewTestAgent(t, f, agent.WithChecker(&permission.Pipeline{
		Deny: []permission.Checker{
			permission.NewDenyListChecker(),
		},
	}))

	out, err := a.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "done" {
		t.Errorf("output = %q, want done", out)
	}
	msgs := a.Messages()
	if !strings.Contains(msgs[3].Content, "EXECUTED:echo hi") {
		t.Errorf("bash should execute when allowed, got %q", msgs[3].Content)
	}
}

// 场景 3：BashAskChecker 拒绝 → tool 不执行
func TestE2E_BashAsk_Deny(t *testing.T) {
	f := e2e.NewFakeLLM(t,
		e2e.ScriptedResponse{
			ToolCalls: []openai.ToolCall{
				e2e.MakeToolCall("call_1", "bash", `{"command":"rm -i foo"}`),
			},
		},
		e2e.ScriptedResponse{Content: "recovered"},
	)
	a := e2e.NewTestAgent(t, f, agent.WithChecker(&permission.Pipeline{
		Deny: []permission.Checker{
			permission.NewBashAskChecker(permission.AskerFunc(
				func(_ context.Context, _ string, _ map[string]any, _ string) bool { return false },
			)),
		},
	}))

	if _, err := a.Run(context.Background(), "test"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	msgs := a.Messages()
	if !strings.Contains(msgs[3].Content, "Permission denied") {
		t.Errorf("expected Permission denied, got %q", msgs[3].Content)
	}
	if strings.Contains(msgs[3].Content, "EXECUTED:") {
		t.Errorf("bash should not execute when user rejected, got %q", msgs[3].Content)
	}
}

// 场景 4：BashAskChecker 批准 → tool 执行
func TestE2E_BashAsk_Allow(t *testing.T) {
	f := e2e.NewFakeLLM(t,
		e2e.ScriptedResponse{
			ToolCalls: []openai.ToolCall{
				e2e.MakeToolCall("call_1", "bash", `{"command":"rm -i foo"}`),
			},
		},
		e2e.ScriptedResponse{Content: "ok"},
	)
	a := e2e.NewTestAgent(t, f, agent.WithChecker(&permission.Pipeline{
		Deny: []permission.Checker{
			permission.NewBashAskChecker(permission.AskerFunc(
				func(_ context.Context, _ string, _ map[string]any, _ string) bool { return true },
			)),
		},
	}))

	if _, err := a.Run(context.Background(), "test"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	msgs := a.Messages()
	if !strings.Contains(msgs[3].Content, "EXECUTED:rm -i foo") {
		t.Errorf("bash should execute when user approved, got %q", msgs[3].Content)
	}
}

// 场景 5：nil checker → 全部放行
func TestE2E_NoChecker_AlwaysExecutes(t *testing.T) {
	f := e2e.NewFakeLLM(t,
		e2e.ScriptedResponse{
			ToolCalls: []openai.ToolCall{
				e2e.MakeToolCall("call_1", "bash", `{"command":"rm -rf /"}`),
			},
		},
		e2e.ScriptedResponse{Content: "ok"},
	)
	a := e2e.NewTestAgent(t, f)

	if _, err := a.Run(context.Background(), "test"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	msgs := a.Messages()
	if !strings.Contains(msgs[3].Content, "EXECUTED:rm -rf /") {
		t.Errorf("without checker, bash should execute, got %q", msgs[3].Content)
	}
}
