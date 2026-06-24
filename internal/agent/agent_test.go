package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wsx864321/coding-agent/internal/event"
	"github.com/wsx864321/coding-agent/internal/permission"
	"github.com/wsx864321/coding-agent/internal/provider"
	_ "github.com/wsx864321/coding-agent/internal/provider/openai"
	"github.com/wsx864321/coding-agent/internal/tools"
)

// =====================================================================
// 工具：fake LLM server（流式 SSE 响应）
// =====================================================================

type scriptedResponse struct {
	content   string
	toolCalls []provider.ToolCall
}

type fakeLLMServer struct {
	server *httptest.Server
	queue  []scriptedResponse
	idx    atomic.Int32
	calls  atomic.Int32
}

func newFakeLLM(t *testing.T, responses ...scriptedResponse) *fakeLLMServer {
	t.Helper()
	f := &fakeLLMServer{queue: responses}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		i := int(f.idx.Add(1)) - 1
		if i >= len(f.queue) {
			t.Errorf("fakeLLM: 第 %d 次请求没有预设响应（仅准备了 %d 个）", i+1, len(f.queue))
			http.Error(w, "no more scripted responses", http.StatusInternalServerError)
			return
		}
		resp := f.queue[i]

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, _ := w.(http.Flusher)

		if resp.content != "" {
			writeSSE(w, flusher, streamDelta{
				Choices: []streamChoice{{
					Delta: streamDeltaContent{
						Role:    "assistant",
						Content: resp.content,
					},
				}},
			})
		}

		for _, tc := range resp.toolCalls {
			writeSSE(w, flusher, streamDelta{
				Choices: []streamChoice{{
					Delta: streamDeltaContent{
						ToolCalls: []streamToolDelta{{
							ID:   tc.ID,
							Type: "function",
							Function: chatFunction{
								Name:      tc.Name,
								Arguments: tc.Arguments,
							},
						}},
					},
				}},
			})
		}

		finishReason := "stop"
		if len(resp.toolCalls) > 0 {
			finishReason = "tool_calls"
		}
		writeSSE(w, flusher, streamDelta{
			Choices: []streamChoice{{
				FinishReason: finishReason,
			}},
			Usage: &streamUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		})

		fmt.Fprintf(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

type streamDelta struct {
	Choices []streamChoice `json:"choices"`
	Usage   *streamUsage   `json:"usage,omitempty"`
}

type streamChoice struct {
	Delta        streamDeltaContent `json:"delta"`
	FinishReason string             `json:"finish_reason,omitempty"`
}

type streamDeltaContent struct {
	Role      string            `json:"role,omitempty"`
	Content   string            `json:"content,omitempty"`
	ToolCalls []streamToolDelta `json:"tool_calls,omitempty"`
}

type streamToolDelta struct {
	Index    int          `json:"index"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function chatFunction `json:"function,omitempty"`
}

type chatFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type streamUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, delta streamDelta) {
	data, _ := json.Marshal(delta)
	fmt.Fprintf(w, "data: %s\n\n", data)
	if flusher != nil {
		flusher.Flush()
	}
}

// =====================================================================
// 工具：简易 echo / fail 工具
// =====================================================================

type echoTool struct{}

func (echoTool) Name() string        { return "echo" }
func (echoTool) Description() string { return "echo back the input" }
func (echoTool) ReadOnly() bool      { return true }
func (echoTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"input":{"type":"string"}},"required":["input"]}`)
}
func (echoTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	return "echoed: " + args["input"].(string), nil
}

type failTool struct{}

func (failTool) Name() string        { return "fail" }
func (failTool) ReadOnly() bool      { return false }
func (failTool) Description() string { return "always fails" }
func (failTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (failTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	return "", &fakeErr{msg: "intentional failure"}
}

type fakeErr struct{ msg string }

func (e *fakeErr) Error() string { return e.msg }

// =====================================================================
// 辅助
// =====================================================================

func newTestAgent(t *testing.T, f *fakeLLMServer, extraTools ...tools.Tool) *Agent {
	t.Helper()
	registry := tools.NewRegistry()
	registry.Register(echoTool{})
	registry.Register(failTool{})
	for _, tl := range extraTools {
		registry.Register(tl)
	}

	prov, err := provider.New("openai", provider.Config{
		Name:   "openai",
		APIKey: "test-key",
		BaseURL: f.server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("provider.New: %v", err)
	}

	a, err := NewAgent(Config{
		APIKey:   "test-key",
		BaseURL:  f.server.URL + "/v1",
		MaxTurns: 5,
	},
		WithRegistry(registry),
		WithProvider(prov),
	)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	return a
}

func makeToolCall(id, name, args string) provider.ToolCall {
	return provider.ToolCall{
		ID:        id,
		Name:      name,
		Arguments: args,
	}
}

// =====================================================================
// NewAgent 校验
// =====================================================================

func TestNewAgent_NilOption(t *testing.T) {
	a, err := NewAgent(Config{APIKey: "x"}, nil)
	if err != nil {
		t.Fatalf("NewAgent with nil option should not error: %v", err)
	}
	if a == nil {
		t.Fatal("agent should not be nil")
	}
}

func TestNewAgent_MissingAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	_, err := NewAgent(Config{}, WithRegistry(tools.NewRegistry()))
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestNewAgent_Defaults(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "ok"})
	prov, _ := provider.New("openai", provider.Config{
		Name: "openai", APIKey: "x", BaseURL: f.server.URL + "/v1",
	})
	a, err := NewAgent(Config{APIKey: "x", BaseURL: f.server.URL + "/v1"},
		WithRegistry(tools.NewRegistry()), WithProvider(prov))
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	if a.cfg.Model != DefaultModel {
		t.Errorf("Model = %q, want %q", a.cfg.Model, DefaultModel)
	}
	if a.cfg.MaxTurns != DefaultMaxTurns {
		t.Errorf("MaxTurns = %d, want %d", a.cfg.MaxTurns, DefaultMaxTurns)
	}
	if a.cfg.SystemPrompt == "" {
		t.Error("SystemPrompt should be auto-generated when not provided")
	}
}

func TestNewAgent_CustomSystemPrompt(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "ok"})
	prov, _ := provider.New("openai", provider.Config{
		Name: "openai", APIKey: "x", BaseURL: f.server.URL + "/v1",
	})
	a, err := NewAgent(Config{
		APIKey:       "x",
		BaseURL:      f.server.URL + "/v1",
		SystemPrompt: "You are a custom agent.",
	}, WithRegistry(tools.NewRegistry()), WithProvider(prov))
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	if a.cfg.SystemPrompt != "You are a custom agent." {
		t.Errorf("SystemPrompt = %q, want custom", a.cfg.SystemPrompt)
	}
	if len(a.messages) != 1 {
		t.Errorf("messages len = %d, want 1", len(a.messages))
	}
	if a.messages[0].Content != "You are a custom agent." {
		t.Errorf("system content = %q", a.messages[0].Content)
	}
}

// =====================================================================
// prompt.go
// =====================================================================

func TestBuildSystemPrompt_NoTools(t *testing.T) {
	r := tools.NewRegistry()
	got := buildSystemPrompt(r, nil)
	if !strings.Contains(got, "未注册任何工具") {
		t.Errorf("empty registry prompt should mention no tools, got %q", got)
	}
}

func TestBuildSystemPrompt_WithTools(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(echoTool{})
	r.Register(failTool{})
	got := buildSystemPrompt(r, nil)
	if !strings.Contains(got, "echo") || !strings.Contains(got, "fail") {
		t.Errorf("prompt should list tools, got %q", got)
	}
	if !strings.Contains(got, `"properties"`) {
		t.Errorf("prompt should contain schema json, got %q", got)
	}
}

// =====================================================================
// Run: 简单 LLM 直接答
// =====================================================================

func TestRun_DirectAnswer(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "hello world"})
	a := newTestAgent(t, f)

	out, err := a.Run(context.Background(), "hi")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "hello world" {
		t.Errorf("output = %q, want %q", out, "hello world")
	}
	if f.calls.Load() != 1 {
		t.Errorf("expected 1 LLM call, got %d", f.calls.Load())
	}
	if len(a.messages) != 3 {
		t.Errorf("messages len = %d, want 3", len(a.messages))
	}
}

func TestRun_EmptyUserInput(t *testing.T) {
	f := newFakeLLM(t)
	a := newTestAgent(t, f)
	_, err := a.Run(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty user input")
	}
	if f.calls.Load() != 0 {
		t.Errorf("should not call LLM for empty input, got %d calls", f.calls.Load())
	}
}

// =====================================================================
// Run: 单轮 tool call
// =====================================================================

func TestRun_EmitsTextAndToolEvents(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []provider.ToolCall{
				makeToolCall("call_1", "echo", `{"input":"hi"}`),
			},
		},
		scriptedResponse{content: "done"},
	)

	var kinds []event.Kind
	registry := tools.NewRegistry()
	registry.Register(echoTool{})
	prov, err := provider.New("openai", provider.Config{
		Name: "openai", APIKey: "test-key", BaseURL: f.server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("provider.New: %v", err)
	}

	a, err := NewAgent(Config{
		APIKey:   "test-key",
		BaseURL:  f.server.URL + "/v1",
		MaxTurns: 5,
	},
		WithRegistry(registry),
		WithProvider(prov),
		WithSink(event.FuncSink(func(e event.Event) {
			kinds = append(kinds, e.Kind)
		})),
	)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}

	_, err = a.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	hasText := false
	hasTool := false
	for _, k := range kinds {
		if k == event.Text {
			hasText = true
		}
		if k == event.ToolDispatch || k == event.ToolResult {
			hasTool = true
		}
	}
	if !hasText {
		t.Errorf("expected Text events, got kinds=%v", kinds)
	}
	if !hasTool {
		t.Errorf("expected ToolDispatch/ToolResult events, got kinds=%v", kinds)
	}
}

func TestRun_OneToolCall(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []provider.ToolCall{
				makeToolCall("call_1", "echo", `{"input":"hi"}`),
			},
		},
		scriptedResponse{content: "final answer"},
	)
	a := newTestAgent(t, f)

	out, err := a.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "final answer" {
		t.Errorf("output = %q, want %q", out, "final answer")
	}
	if f.calls.Load() != 2 {
		t.Errorf("expected 2 LLM calls, got %d", f.calls.Load())
	}
	if len(a.messages) != 5 {
		t.Errorf("messages len = %d, want 5", len(a.messages))
	}
	if a.messages[3].Role != provider.RoleTool {
		t.Errorf("messages[3] role = %q, want tool", a.messages[3].Role)
	}
	if a.messages[3].ToolCallID != "call_1" {
		t.Errorf("messages[3] ToolCallID = %q, want call_1", a.messages[3].ToolCallID)
	}
	if !strings.Contains(a.messages[3].Content, "echoed: hi") {
		t.Errorf("messages[3] content = %q", a.messages[3].Content)
	}
}

// =====================================================================
// Run: 工具执行失败 → 不中断
// =====================================================================

func TestRun_ToolErrorNotFatal(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []provider.ToolCall{
				makeToolCall("call_1", "fail", `{}`),
			},
		},
		scriptedResponse{content: "recovered"},
	)
	a := newTestAgent(t, f)

	out, err := a.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run should not return error for tool failure, got: %v", err)
	}
	if out != "recovered" {
		t.Errorf("output = %q, want %q", out, "recovered")
	}
	if !strings.Contains(a.messages[3].Content, "Error:") {
		t.Errorf("tool error should be reported as Error:, got %q", a.messages[3].Content)
	}
}

// =====================================================================
// Run: 未知工具
// =====================================================================

func TestRun_UnknownTool(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []provider.ToolCall{
				makeToolCall("call_1", "does_not_exist", `{}`),
			},
		},
		scriptedResponse{content: "ok"},
	)
	a := newTestAgent(t, f)

	out, err := a.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "ok" {
		t.Errorf("output = %q, want ok", out)
	}
	if !strings.Contains(a.messages[3].Content, "未注册") {
		t.Errorf("unknown tool should be reported, got %q", a.messages[3].Content)
	}
}

// =====================================================================
// Run: 超过 MaxTurns
// =====================================================================

func TestRun_MaxTurnsExceeded(t *testing.T) {
	responses := make([]scriptedResponse, 10)
	for i := range responses {
		responses[i] = scriptedResponse{
			toolCalls: []provider.ToolCall{
				makeToolCall("c", "echo", `{"input":"x"}`),
			},
		}
	}
	f := newFakeLLM(t, responses...)

	registry := tools.NewRegistry()
	registry.Register(echoTool{})
	prov, _ := provider.New("openai", provider.Config{
		Name: "openai", APIKey: "x", BaseURL: f.server.URL + "/v1",
	})
	a, err := NewAgent(Config{
		APIKey:   "x",
		BaseURL:  f.server.URL + "/v1",
		MaxTurns: 3,
	}, WithRegistry(registry), WithProvider(prov))
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}

	_, err = a.Run(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for exceeding max turns")
	}
	if !strings.Contains(err.Error(), "超过最大轮数") {
		t.Errorf("error = %q, expected to mention max turns", err.Error())
	}
	if f.calls.Load() != 3 {
		t.Errorf("expected 3 LLM calls, got %d", f.calls.Load())
	}
}

// =====================================================================
// Reset
// =====================================================================

func TestReset(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "ok"})
	a := newTestAgent(t, f)
	_, _ = a.Run(context.Background(), "hi")

	if len(a.messages) <= 1 {
		t.Fatalf("messages should have grown, got %d", len(a.messages))
	}
	a.Reset()
	if len(a.messages) != 1 {
		t.Errorf("after Reset messages len = %d, want 1", len(a.messages))
	}
	if a.messages[0].Role != provider.RoleSystem {
		t.Errorf("after Reset first message role = %q, want system", a.messages[0].Role)
	}
}

// =====================================================================
// Messages 返回副本
// =====================================================================

func TestMessages_ReturnsCopy(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "ok"})
	a := newTestAgent(t, f)
	_, _ = a.Run(context.Background(), "hi")

	msgs := a.Messages()
	msgs[0].Content = "tampered"
	if a.messages[0].Content == "tampered" {
		t.Error("Messages() should return a copy, not a reference")
	}
}

// =====================================================================
// Hooks 集成测试
// =====================================================================

func TestRun_PreToolUse_HookBlocks(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []provider.ToolCall{
				makeToolCall("call_1", "echo", `{"input":"hi"}`),
			},
		},
		scriptedResponse{content: "got it"},
	)

	hr := &stubToolHooks{
		preToolUse: func(_ context.Context, _ string, _ map[string]any) (bool, string) {
			return true, "not allowed"
		},
	}
	a := newTestAgentWithHooks(t, f, hr)

	out, err := a.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "got it" {
		t.Errorf("output = %q, want 'got it'", out)
	}
	if !strings.Contains(a.messages[3].Content, "Blocked by hook") {
		t.Errorf("expected tool message to contain 'Blocked by hook', got %q", a.messages[3].Content)
	}
}

func TestRun_PreToolUse_HookAllowsButCheckerDenies(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []provider.ToolCall{
				makeToolCall("call_1", "echo", `{"input":"x"}`),
			},
		},
		scriptedResponse{content: "recovered"},
	)

	hr := &stubToolHooks{
		preToolUse: func(_ context.Context, _ string, _ map[string]any) (bool, string) {
			return false, ""
		},
	}

	registry := tools.NewRegistry()
	registry.Register(echoTool{})
	prov, _ := provider.New("openai", provider.Config{
		Name: "openai", APIKey: "test-key", BaseURL: f.server.URL + "/v1",
	})
	a, err := NewAgent(Config{
		APIKey:   "test-key",
		BaseURL:  f.server.URL + "/v1",
		MaxTurns: 5,
	},
		WithRegistry(registry),
		WithHooks(hr),
		WithChecker(denyAllChecker{}),
		WithProvider(prov),
	)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}

	out, err := a.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "recovered" {
		t.Errorf("output = %q, want 'recovered'", out)
	}
	if !strings.Contains(a.messages[3].Content, "Permission denied") {
		t.Errorf("expected tool message to contain 'Permission denied', got %q", a.messages[3].Content)
	}
}

func TestRun_UserPromptSubmit_Triggered(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "ok"})

	got := ""
	hr := &stubToolHooks{
		userPromptSubmit: func(_ context.Context, c string) error {
			got = c
			return nil
		},
	}
	a := newTestAgentWithHooks(t, f, hr)

	_, _ = a.Run(context.Background(), "hello world")
	if got != "hello world" {
		t.Errorf("UserPromptSubmit hook did not see input: got %q, want %q", got, "hello world")
	}
}

func TestRun_Stop_ForceContinue(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{content: "first answer"},
		scriptedResponse{content: "second answer"},
	)

	fired := false
	hr := &stubToolHooks{
		stop: func(_ context.Context, _ []provider.Message) (string, bool) {
			if !fired {
				fired = true
				return "请继续", true
			}
			return "", false
		},
	}
	a := newTestAgentWithHooks(t, f, hr)

	out, err := a.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "second answer" {
		t.Errorf("output = %q, want 'second answer'", out)
	}
	if f.calls.Load() != 2 {
		t.Errorf("expected 2 LLM calls, got %d", f.calls.Load())
	}
	if len(a.messages) != 5 {
		t.Errorf("messages len = %d, want 5", len(a.messages))
	}
	if a.messages[3].Role != provider.RoleUser || a.messages[3].Content != "请继续" {
		t.Errorf("messages[3] should be forced user message, got role=%q content=%q",
			a.messages[3].Role, a.messages[3].Content)
	}
}

func TestRun_Stop_NoForce(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "ok"})

	hr := &stubToolHooks{
		stop: func(_ context.Context, _ []provider.Message) (string, bool) {
			return "", false
		},
	}
	a := newTestAgentWithHooks(t, f, hr)

	out, err := a.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "ok" {
		t.Errorf("output = %q, want 'ok'", out)
	}
	if f.calls.Load() != 1 {
		t.Errorf("expected 1 LLM call, got %d", f.calls.Load())
	}
}

func TestRun_PostToolUse_Triggered(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []provider.ToolCall{
				makeToolCall("call_1", "echo", `{"input":"hi"}`),
			},
		},
		scriptedResponse{content: "ok"},
	)

	type seen struct {
		name   string
		output string
	}
	var got seen
	hr := &stubToolHooks{
		postToolUse: func(_ context.Context, name string, _ map[string]any, output string) {
			got.name = name
			got.output = output
		},
	}
	a := newTestAgentWithHooks(t, f, hr)

	_, err := a.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.name != "echo" {
		t.Errorf("PostToolUse saw name=%q, want 'echo'", got.name)
	}
	if !strings.Contains(got.output, "echoed: hi") {
		t.Errorf("PostToolUse saw output=%q, want 'echoed: hi'", got.output)
	}
}

// =====================================================================
// 测试辅助
// =====================================================================

func newTestAgentWithHooks(t *testing.T, f *fakeLLMServer, hr ToolHooks) *Agent {
	t.Helper()
	registry := tools.NewRegistry()
	registry.Register(echoTool{})
	registry.Register(failTool{})

	prov, _ := provider.New("openai", provider.Config{
		Name: "openai", APIKey: "test-key", BaseURL: f.server.URL + "/v1",
	})
	a, err := NewAgent(Config{
		APIKey:   "test-key",
		BaseURL:  f.server.URL + "/v1",
		MaxTurns: 5,
	},
		WithRegistry(registry),
		WithHooks(hr),
		WithProvider(prov),
	)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	return a
}

type denyAllChecker struct{}

func (denyAllChecker) Check(_ context.Context, _ string, _ map[string]any) permission.CheckResult {
	return permission.CheckResult{Decision: permission.DecisionDeny, Reason: "test-deny"}
}
