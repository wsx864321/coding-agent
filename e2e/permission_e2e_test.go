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
			&permission.DenyListChecker{Patterns: []permission.DenyPattern{
				{ToolName: "bash", ArgName: "command", Substr: "rm -rf /", Reason: "硬拒绝：删根目录"},
			}},
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
			&permission.DenyListChecker{Patterns: permission.DefaultBashDenyList()},
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

// 场景 3：Ask 规则命中 + Asker deny → tool 不执行
func TestE2E_PermissionAsk_Deny(t *testing.T) {
	f := e2e.NewFakeLLM(t,
		e2e.ScriptedResponse{
			ToolCalls: []openai.ToolCall{
				e2e.MakeToolCall("call_1", "bash", `{"command":"rm -i foo"}`),
			},
		},
		e2e.ScriptedResponse{Content: "recovered"},
	)
	a := e2e.NewTestAgent(t, f, agent.WithChecker(&permission.Pipeline{
		Ask: []permission.Checker{
			&permission.AskRuleChecker{Rules: []permission.AskRule{
				permission.DefaultBashAskRules(),
			}},
		},
		Asker: permission.AskerFunc(func(_ context.Context, _ permission.ToolCall, _ string) bool {
			return false
		}),
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

// 场景 4：Ask 规则命中 + Asker allow → tool 执行
func TestE2E_PermissionAsk_Allow(t *testing.T) {
	f := e2e.NewFakeLLM(t,
		e2e.ScriptedResponse{
			ToolCalls: []openai.ToolCall{
				e2e.MakeToolCall("call_1", "bash", `{"command":"rm -i foo"}`),
			},
		},
		e2e.ScriptedResponse{Content: "ok"},
	)
	a := e2e.NewTestAgent(t, f, agent.WithChecker(&permission.Pipeline{
		Ask: []permission.Checker{
			&permission.AskRuleChecker{Rules: []permission.AskRule{
				permission.DefaultBashAskRules(),
			}},
		},
		Asker: permission.AskerFunc(func(_ context.Context, _ permission.ToolCall, _ string) bool {
			return true
		}),
	}))

	if _, err := a.Run(context.Background(), "test"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	msgs := a.Messages()
	if !strings.Contains(msgs[3].Content, "EXECUTED:rm -i foo") {
		t.Errorf("bash should execute when user approved, got %q", msgs[3].Content)
	}
}

// 场景 5：nil checker → 走老路径，全部放行
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
	// 显式不设 checker

	if _, err := a.Run(context.Background(), "test"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	msgs := a.Messages()
	if !strings.Contains(msgs[3].Content, "EXECUTED:rm -rf /") {
		t.Errorf("without checker, bash should execute, got %q", msgs[3].Content)
	}
}
