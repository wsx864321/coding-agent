//go:build e2e

package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	openai "github.com/sashabaranov/go-openai"

	"github.com/wsx864321/coding-agent/internal/agent"
	"github.com/wsx864321/coding-agent/internal/permission"
	"github.com/wsx864321/coding-agent/internal/tools"
)

// =====================================================================
// Fake LLM Server（e2e 自包含）
// =====================================================================

type scriptedResponse struct {
	content   string
	toolCalls []openai.ToolCall
}

type fakeLLMServer struct {
	server *httptest.Server
	queue  []scriptedResponse
	idx    atomic.Int32
}

func newFakeLLM(t *testing.T, responses ...scriptedResponse) *fakeLLMServer {
	t.Helper()
	f := &fakeLLMServer{queue: responses}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(f.idx.Add(1)) - 1
		if i >= len(f.queue) {
			t.Errorf("e2e fake LLM: 第 %d 次请求没有预设响应", i+1)
			http.Error(w, "no scripted response", http.StatusInternalServerError)
			return
		}
		resp := f.queue[i]
		body := openai.ChatCompletionResponse{
			ID: "fake", Model: "fake-model",
			Choices: []openai.ChatCompletionChoice{{
				Index: 0,
				Message: openai.ChatCompletionMessage{
					Role:      openai.ChatMessageRoleAssistant,
					Content:   resp.content,
					ToolCalls: resp.toolCalls,
				},
				FinishReason: openai.FinishReasonStop,
			}},
		}
		if len(resp.toolCalls) > 0 {
			body.Choices[0].FinishReason = openai.FinishReasonToolCalls
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func makeToolCall(id, name, args string) openai.ToolCall {
	return openai.ToolCall{
		ID: id, Type: openai.ToolTypeFunction,
		Function: openai.FunctionCall{Name: name, Arguments: args},
	}
}

// =====================================================================
// Fake Bash 工具：把"执行过的命令"打上 EXECUTED: 前缀
// =====================================================================

type fakeBash struct{}

func (fakeBash) Name() string        { return "bash" }
func (fakeBash) Description() string { return "fake bash for e2e" }
func (fakeBash) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`)
}
func (fakeBash) Execute(_ context.Context, args map[string]any) (string, error) {
	cmd, _ := args["command"].(string)
	return "EXECUTED:" + cmd, nil
}

var _ tools.Tool = fakeBash{}

// =====================================================================
// 辅助：构造测试用 Agent
// =====================================================================

func newTestAgent(t *testing.T, f *fakeLLMServer) *agent.Agent {
	t.Helper()
	registry := tools.NewRegistry()
	registry.Register(fakeBash{})

	// 用一个"通用 client"，BaseURL 指向 fake；Agent 内部 new client 即可打到 fake
	a, err := agent.NewAgent(agent.Config{
		APIKey:   "test-key",
		BaseURL:  f.server.URL + "/v1",
		MaxTurns: 5,
	}, registry)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	return a
}

// =====================================================================
// 场景：permission gate 在 agent loop 中按预期工作
// =====================================================================

func TestE2E_PermissionDeny_BlocksExecute(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []openai.ToolCall{
				makeToolCall("call_1", "bash", `{"command":"rm -rf /"}`),
			},
		},
		scriptedResponse{content: "ok"},
	)
	a := newTestAgent(t, f)

	a.SetChecker(&permission.Pipeline{
		Deny: []permission.Checker{
			&permission.DenyListChecker{Patterns: []permission.DenyPattern{
				{ToolName: "bash", ArgName: "command", Substr: "rm -rf /", Reason: "硬拒绝：删根目录"},
			}},
		},
	})

	out, err := a.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "ok" {
		t.Errorf("output = %q, want ok", out)
	}
	msgs := a.Messages()
	// system + user + assistant(tool_call) + tool(result) + assistant(answer) = 5
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

func TestE2E_PermissionAllow_Executes(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []openai.ToolCall{
				makeToolCall("call_1", "bash", `{"command":"echo hi"}`),
			},
		},
		scriptedResponse{content: "done"},
	)
	a := newTestAgent(t, f)

	a.SetChecker(&permission.Pipeline{
		Deny: []permission.Checker{
			&permission.DenyListChecker{Patterns: permission.DefaultBashDenyList()},
		},
	})

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

func TestE2E_PermissionAsk_Deny(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []openai.ToolCall{
				makeToolCall("call_1", "bash", `{"command":"rm -i foo"}`),
			},
		},
		scriptedResponse{content: "recovered"},
	)
	a := newTestAgent(t, f)

	a.SetChecker(&permission.Pipeline{
		Ask: []permission.Checker{
			&permission.AskRuleChecker{Rules: []permission.AskRule{
				permission.DefaultBashAskRules(),
			}},
		},
		Asker: permission.AskerFunc(func(_ context.Context, _ permission.ToolCall, _ string) bool {
			return false
		}),
	})

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

func TestE2E_PermissionAsk_Allow(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []openai.ToolCall{
				makeToolCall("call_1", "bash", `{"command":"rm -i foo"}`),
			},
		},
		scriptedResponse{content: "ok"},
	)
	a := newTestAgent(t, f)

	a.SetChecker(&permission.Pipeline{
		Ask: []permission.Checker{
			&permission.AskRuleChecker{Rules: []permission.AskRule{
				permission.DefaultBashAskRules(),
			}},
		},
		Asker: permission.AskerFunc(func(_ context.Context, _ permission.ToolCall, _ string) bool {
			return true
		}),
	})

	if _, err := a.Run(context.Background(), "test"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	msgs := a.Messages()
	if !strings.Contains(msgs[3].Content, "EXECUTED:rm -i foo") {
		t.Errorf("bash should execute when user approved, got %q", msgs[3].Content)
	}
}

func TestE2E_NoChecker_AlwaysExecutes(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []openai.ToolCall{
				makeToolCall("call_1", "bash", `{"command":"rm -rf /"}`),
			},
		},
		scriptedResponse{content: "ok"},
	)
	a := newTestAgent(t, f)
	// 显式不设 checker

	if _, err := a.Run(context.Background(), "test"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	msgs := a.Messages()
	if !strings.Contains(msgs[3].Content, "EXECUTED:rm -rf /") {
		t.Errorf("without checker, bash should execute, got %q", msgs[3].Content)
	}
}
