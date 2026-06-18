//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/wsx864321/coding-agent/internal/agent"
	"github.com/wsx864321/coding-agent/internal/provider"
	_ "github.com/wsx864321/coding-agent/internal/provider/openai"
	"github.com/wsx864321/coding-agent/internal/tools"
)

// =====================================================================
// Fake LLM Server (SSE streaming)
// =====================================================================

type ScriptedResponse struct {
	Content   string
	ToolCalls []provider.ToolCall
}

type FakeLLMServer struct {
	server *httptest.Server
	queue  []ScriptedResponse
	idx    atomic.Int32
}

func NewFakeLLM(t *testing.T, responses ...ScriptedResponse) *FakeLLMServer {
	t.Helper()
	f := &FakeLLMServer{queue: responses}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(f.idx.Add(1)) - 1
		if i >= len(f.queue) {
			t.Errorf("e2e fake LLM: 第 %d 次请求没有预设响应", i+1)
			http.Error(w, "no scripted response", http.StatusInternalServerError)
			return
		}
		resp := f.queue[i]

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, _ := w.(http.Flusher)

		if resp.Content != "" {
			writeSSE(w, flusher, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"role":    "assistant",
						"content": resp.Content,
					},
				}},
			})
		}

		for _, tc := range resp.ToolCalls {
			writeSSE(w, flusher, map[string]any{
				"choices": []map[string]any{{
					"delta": map[string]any{
						"tool_calls": []map[string]any{{
							"id":   tc.ID,
							"type": "function",
							"function": map[string]any{
								"name":      tc.Name,
								"arguments": tc.Arguments,
							},
						}},
					},
				}},
			})
		}

		finishReason := "stop"
		if len(resp.ToolCalls) > 0 {
			finishReason = "tool_calls"
		}
		writeSSE(w, flusher, map[string]any{
			"choices": []map[string]any{{
				"finish_reason": finishReason,
			}},
			"usage": map[string]any{
				"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15,
			},
		})

		fmt.Fprintf(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, data any) {
	b, _ := json.Marshal(data)
	fmt.Fprintf(w, "data: %s\n\n", b)
	if flusher != nil {
		flusher.Flush()
	}
}

func MakeToolCall(id, name, args string) provider.ToolCall {
	return provider.ToolCall{
		ID:        id,
		Name:      name,
		Arguments: args,
	}
}

// =====================================================================
// Fake Bash 工具
// =====================================================================

type FakeBash struct{}

func (FakeBash) Name() string        { return "bash" }
func (FakeBash) Description() string { return "fake bash for e2e" }
func (FakeBash) ReadOnly() bool      { return false }
func (FakeBash) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`)
}
func (FakeBash) Execute(_ context.Context, args map[string]any) (string, error) {
	cmd, _ := args["command"].(string)
	return "EXECUTED:" + cmd, nil
}

var _ tools.Tool = FakeBash{}

// =====================================================================
// 辅助：构造测试用 Agent
// =====================================================================

func NewTestAgent(t *testing.T, f *FakeLLMServer, opts ...agent.Option) *agent.Agent {
	t.Helper()
	registry := tools.NewRegistry()
	registry.Register(FakeBash{})

	prov, err := provider.New("openai", provider.Config{
		Name:    "openai",
		APIKey:  "test-key",
		BaseURL: f.server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("provider.New: %v", err)
	}

	fullOpts := append([]agent.Option{
		agent.WithRegistry(registry),
		agent.WithProvider(prov),
	}, opts...)

	a, err := agent.NewAgent(agent.Config{
		APIKey:   "test-key",
		BaseURL:  f.server.URL + "/v1",
		MaxTurns: 5,
	}, fullOpts...)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	return a
}
