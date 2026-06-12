package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	openai "github.com/sashabaranov/go-openai"

	"github.com/wsx864321/coding-agent/internal/hooks"
	"github.com/wsx864321/coding-agent/internal/permission"
	"github.com/wsx864321/coding-agent/internal/tools"
)

// =====================================================================
// 工具：fake LLM server
// =====================================================================

// scriptedResponse 是 fake LLM 在一次请求中返回的响应
type scriptedResponse struct {
	content   string            // 最终内容（当 toolCalls 为空时）
	toolCalls []openai.ToolCall // tool calls（让 agent 继续 loop）
}

// fakeLLMServer 启动一个 httptest.Server，模拟 OpenAI 兼容接口
// 每次收到 /v1/chat/completions 请求时按顺序返回 queuedResponses 中的下一个
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

		// 构造 ChatCompletionResponse
		body := openai.ChatCompletionResponse{
			ID:      "fake-id",
			Object:  "chat.completion",
			Created: 0,
			Model:   "fake-model",
			Choices: []openai.ChatCompletionChoice{
				{
					Index: 0,
					Message: openai.ChatCompletionMessage{
						Role:      openai.ChatMessageRoleAssistant,
						Content:   resp.content,
						ToolCalls: resp.toolCalls,
					},
					FinishReason: openai.FinishReasonStop,
				},
			},
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

// makeClientWithFake 用 fake LLM URL 构造一个 openai.Client
func makeClientWithFake(t *testing.T, f *fakeLLMServer) *openai.Client {
	t.Helper()
	cfg := openai.DefaultConfig("test-key")
	cfg.BaseURL = f.server.URL + "/v1"
	return openai.NewClientWithConfig(cfg)
}

// =====================================================================
// 工具：简易 echo 工具，用于测试工具调用
// =====================================================================

// echoTool 简单工具，接收 input 字段返回它
type echoTool struct{}

func (echoTool) Name() string        { return "echo" }
func (echoTool) Description() string { return "echo back the input" }
func (echoTool) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"input":{"type":"string"}},"required":["input"]}`)
}
func (echoTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	return "echoed: " + args["input"].(string), nil
}

// failTool 总是返回错误的工具
type failTool struct{}

func (failTool) Name() string { return "fail" }
func (failTool) Description() string {
	return "always fails"
}
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
	a, err := NewAgent(Config{
		APIKey:   "test-key",
		BaseURL:  f.server.URL + "/v1",
		MaxTurns: 5,
	},
		WithRegistry(registry),
	)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	// 替换为 fake client（NewAgent 内部已经用 BaseURL 创建了一个 client，
	// 但 httptest.Server 真实 URL 在调用 NewAgent 时已传入；我们重新
	// 替换为明确知道接 fake 的 client，确保请求一定打到 fake server）
	a.client = makeClientWithFake(t, f)
	return a
}

// makeToolCall 构造一个 openai.ToolCall
func makeToolCall(id, name, args string) openai.ToolCall {
	return openai.ToolCall{
		ID:   id,
		Type: openai.ToolTypeFunction,
		Function: openai.FunctionCall{
			Name:      name,
			Arguments: args,
		},
	}
}

// =====================================================================
// NewAgent 校验
// =====================================================================

func TestNewAgent_NilOption(t *testing.T) {
	// nil Option 会被跳过，不引发 panic
	a, err := NewAgent(Config{APIKey: "x"}, nil)
	if err != nil {
		t.Fatalf("NewAgent with nil option should not error: %v", err)
	}
	if a == nil {
		t.Fatal("agent should not be nil")
	}
}

func TestNewAgent_MissingAPIKey(t *testing.T) {
	// 通过清空 env 来确保既无 cfg.APIKey 也无 env
	t.Setenv("OPENAI_API_KEY", "")
	_, err := NewAgent(Config{}, WithRegistry(tools.NewRegistry()))
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestNewAgent_Defaults(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "ok"})
	a, err := NewAgent(Config{APIKey: "x", BaseURL: f.server.URL + "/v1"}, WithRegistry(tools.NewRegistry()))
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
	a, err := NewAgent(Config{
		APIKey:       "x",
		BaseURL:      f.server.URL + "/v1",
		SystemPrompt: "You are a custom agent.",
	}, WithRegistry(tools.NewRegistry()))
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	if a.cfg.SystemPrompt != "You are a custom agent." {
		t.Errorf("SystemPrompt = %q, want custom", a.cfg.SystemPrompt)
	}
	// 消息历史里应该只有这一条 system
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
	// 验证 schema 也被注入
	if !strings.Contains(got, `"properties"`) {
		t.Errorf("prompt should contain schema json, got %q", got)
	}
}

// =====================================================================
// Run: 简单 LLM 直接答（无 tool calls）
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
	// 消息历史：system + user + assistant = 3
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

func TestRun_OneToolCall(t *testing.T) {
	// 第一轮：LLM 调 echo，第二轮：LLM 拿到 tool 结果后给最终答案
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []openai.ToolCall{
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
	// 消息历史：system + user + assistant(有 tool call) + tool(result) + assistant(answer) = 5
	if len(a.messages) != 5 {
		t.Errorf("messages len = %d, want 5", len(a.messages))
	}
	// 第 4 条应是 tool 消息，含 echo 结果
	if a.messages[3].Role != openai.ChatMessageRoleTool {
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
// Run: 工具执行失败 → 不中断，回填 Error
// =====================================================================

func TestRun_ToolErrorNotFatal(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []openai.ToolCall{
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
	// 验证 tool 消息含 Error
	if !strings.Contains(a.messages[3].Content, "Error:") {
		t.Errorf("tool error should be reported as Error:, got %q", a.messages[3].Content)
	}
}

// =====================================================================
// Run: 未知工具 → 回填 Error 不中断
// =====================================================================

func TestRun_UnknownTool(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []openai.ToolCall{
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
	// 准备 10 个都返回 tool call 的响应，但 MaxTurns=3
	// 每次 loopStep 消耗 1 轮 tool call + 1 轮 LLM 调用
	// 第 3 轮后未拿到 final → 超过 maxTurns
	responses := make([]scriptedResponse, 10)
	for i := range responses {
		responses[i] = scriptedResponse{
			toolCalls: []openai.ToolCall{
				makeToolCall("c", "echo", `{"input":"x"}`),
			},
		}
	}
	f := newFakeLLM(t, responses...)

	f2 := newFakeLLM(t, responses...) // 给 newTestAgent 一个独立的 queue
	_ = f2

	// 单独构造，maxTurns=3
	registry := tools.NewRegistry()
	registry.Register(echoTool{})
	a, err := NewAgent(Config{
		APIKey:   "x",
		BaseURL:  f.server.URL + "/v1",
		MaxTurns: 3,
	}, WithRegistry(registry))
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	a.client = makeClientWithFake(t, f)

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
	if a.messages[0].Role != openai.ChatMessageRoleSystem {
		t.Errorf("after Reset first message role = %q, want system", a.messages[0].Role)
	}
}

// =====================================================================
// Messages 返回副本（防止外部修改内部状态）
// =====================================================================

func TestMessages_ReturnsCopy(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "ok"})
	a := newTestAgent(t, f)
	_, _ = a.Run(context.Background(), "hi")

	msgs := a.Messages()
	msgs[0].Content = "tampered"
	// 内部不受影响
	if a.messages[0].Content == "tampered" {
		t.Error("Messages() should return a copy, not a reference")
	}
}

// =====================================================================
// Hooks 集成测试
// =====================================================================

// TestRun_PreToolUse_HookBlocks 验证 PreToolUse hook 阻断工具调用
func TestRun_PreToolUse_HookBlocks(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []openai.ToolCall{
				makeToolCall("call_1", "echo", `{"input":"hi"}`),
			},
		},
		scriptedResponse{content: "got it"},
	)

	hr := newTestHookRegistry()
	hr.RegisterPreToolUse(func(_ context.Context, _ string, _ map[string]any) (string, string) {
		return "Blocked by hook: not allowed", "test"
	})
	a := newTestAgentWithHooks(t, f, hr)

	out, err := a.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "got it" {
		t.Errorf("output = %q, want 'got it'", out)
	}
	// 工具消息应包含 "Blocked by hook"
	if !strings.Contains(a.messages[3].Content, "Blocked by hook") {
		t.Errorf("expected tool message to contain 'Blocked by hook', got %q", a.messages[3].Content)
	}
}

// TestRun_PreToolUse_HookAllowsButCheckerDenies 验证 hook 放行但 system Checker 仍可 deny
//
// 这是"hook 放水不能绕过 system deny"的安全不变式
func TestRun_PreToolUse_HookAllowsButCheckerDenies(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []openai.ToolCall{
				makeToolCall("call_1", "echo", `{"input":"x"}`),
			},
		},
		scriptedResponse{content: "recovered"},
	)

	// hook 放行所有调用
	hr := newTestHookRegistry()
	hr.RegisterPreToolUse(func(_ context.Context, _ string, _ map[string]any) (string, string) {
		return "", "" // 放行
	})

	// 重造 Agent 并用 Option 注入 hook + denyAll checker
	registry := tools.NewRegistry()
	registry.Register(echoTool{})
	a, err := NewAgent(Config{
		APIKey:   "test-key",
		BaseURL:  f.server.URL + "/v1",
		MaxTurns: 5,
	},
		WithRegistry(registry),
		WithHooks(hr),
		WithChecker(denyAllChecker{}),
	)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	a.client = makeClientWithFake(t, f)

	out, err := a.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "recovered" {
		t.Errorf("output = %q, want 'recovered'", out)
	}
	// 即便 hook 放行，system deny 仍生效
	if !strings.Contains(a.messages[3].Content, "Permission denied") {
		t.Errorf("expected tool message to contain 'Permission denied', got %q", a.messages[3].Content)
	}
}

// TestRun_UserPromptSubmit_Triggered 验证 UserPromptSubmit 事件被触发
func TestRun_UserPromptSubmit_Triggered(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "ok"})

	hr := newTestHookRegistry()
	got := ""
	hr.RegisterUserPromptSubmit(func(_ context.Context, c string) error {
		got = c
		return nil
	})
	a := newTestAgentWithHooks(t, f, hr)

	_, _ = a.Run(context.Background(), "hello world")
	if got != "hello world" {
		t.Errorf("UserPromptSubmit hook did not see input: got %q, want %q", got, "hello world")
	}
}

// TestRun_Stop_ForceContinue 验证 Stop hook 强制续跑
func TestRun_Stop_ForceContinue(t *testing.T) {
	// 第一轮 LLM 给 final → Stop hook 强制注入 user 消息 → 第二轮 LLM 给 final
	f := newFakeLLM(t,
		scriptedResponse{content: "first answer"},
		scriptedResponse{content: "second answer"},
	)

	hr := newTestHookRegistry()
	fired := false
	hr.RegisterStop(func(_ context.Context, _ []openai.ChatCompletionMessage) (string, bool) {
		if !fired {
			fired = true
			return "请继续", true // 首次强制续跑
		}
		return "", false // 后续放行
	})
	a := newTestAgentWithHooks(t, f, hr)

	out, err := a.Run(context.Background(), "test")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "second answer" {
		t.Errorf("output = %q, want 'second answer'", out)
	}
	if f.calls.Load() != 2 {
		t.Errorf("expected 2 LLM calls (Stop forced continuation), got %d", f.calls.Load())
	}
	// messages 长度：system + user + assistant(final) + user(forced) + assistant(2nd) = 5
	if len(a.messages) != 5 {
		t.Errorf("messages len = %d, want 5", len(a.messages))
	}
	if a.messages[3].Role != openai.ChatMessageRoleUser || a.messages[3].Content != "请继续" {
		t.Errorf("messages[3] should be forced user message, got role=%q content=%q",
			a.messages[3].Role, a.messages[3].Content)
	}
}

// TestRun_Stop_NoForce 验证 Stop hook 不返回 force 时正常结束
func TestRun_Stop_NoForce(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "ok"})

	hr := newTestHookRegistry()
	hr.RegisterStop(func(_ context.Context, _ []openai.ChatCompletionMessage) (string, bool) {
		return "", false // 不续跑
	})
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

// TestRun_PostToolUse_Triggered 验证 PostToolUse 事件被触发
func TestRun_PostToolUse_Triggered(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []openai.ToolCall{
				makeToolCall("call_1", "echo", `{"input":"hi"}`),
			},
		},
		scriptedResponse{content: "ok"},
	)

	hr := newTestHookRegistry()
	type seen struct {
		name   string
		output string
	}
	var got seen
	hr.RegisterPostToolUse(func(_ context.Context, name string, _ map[string]any, output string) {
		got.name = name
		got.output = output
	})
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

// newTestHookRegistry 构造一个空的 hooks.Registry
func newTestHookRegistry() *hooks.Registry {
	return hooks.NewRegistry()
}

// newTestAgentWithHooks 是 newTestAgent + WithHooks(hr) 的合并版本
//
// Agent 构造时通过 option 注入 hr，避免调用已删除的 a.SetHooks(hr)
func newTestAgentWithHooks(t *testing.T, f *fakeLLMServer, hr *hooks.Registry) *Agent {
	t.Helper()
	registry := tools.NewRegistry()
	registry.Register(echoTool{})
	registry.Register(failTool{})

	a, err := NewAgent(Config{
		APIKey:   "test-key",
		BaseURL:  f.server.URL + "/v1",
		MaxTurns: 5,
	},
		WithRegistry(registry),
		WithHooks(hr),
	)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	a.client = makeClientWithFake(t, f)
	return a
}

// denyAllChecker 是 permission.Checker 的"全部拒绝"实现（用于测试）
type denyAllChecker struct{}

func (denyAllChecker) Check(_ context.Context, _ string, _ map[string]any) permission.CheckResult {
	return permission.CheckResult{Decision: permission.DecisionDeny, Reason: "test-deny"}
}
