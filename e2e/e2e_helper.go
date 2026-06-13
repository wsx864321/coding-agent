//go:build e2e

// e2e 包内部的 mock / 辅助
//
// 约定：
//   - 同包复用（package e2e），无需 import
//   - 加 //go:build e2e 和测试文件保持一致，普通 build 不编译
//   - 命名规则：所有跨测试文件复用的 fake 工具 / 辅助函数都放这里
//   - 不引入外部依赖（不开子包），避免循环引用
package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	openai "github.com/sashabaranov/go-openai"

	"github.com/wsx864321/coding-agent/internal/agent"
	"github.com/wsx864321/coding-agent/internal/tools"
)

// =====================================================================
// Fake LLM Server
// =====================================================================

// ScriptedResponse 是 fake LLM 在一次请求中返回的响应
type ScriptedResponse struct {
	Content   string            // 最终内容（当 ToolCalls 为空时）
	ToolCalls []openai.ToolCall // tool calls（让 agent 继续 loop）
}

// FakeLLMServer 启动一个 httptest.Server，模拟 OpenAI 兼容接口
// 每次收到 /v1/chat/completions 请求时按顺序返回 queue 中的下一个
type FakeLLMServer struct {
	server *httptest.Server
	queue  []ScriptedResponse
	idx    atomic.Int32
}

// NewFakeLLM 构造并启动 fake LLM server；t.Cleanup 自动关闭
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
		body := openai.ChatCompletionResponse{
			ID: "fake", Model: "fake-model",
			Choices: []openai.ChatCompletionChoice{{
				Index: 0,
				Message: openai.ChatCompletionMessage{
					Role:      openai.ChatMessageRoleAssistant,
					Content:   resp.Content,
					ToolCalls: resp.ToolCalls,
				},
				FinishReason: openai.FinishReasonStop,
			}},
		}
		if len(resp.ToolCalls) > 0 {
			body.Choices[0].FinishReason = openai.FinishReasonToolCalls
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(f.server.Close)
	return f
}

// MakeToolCall 构造一个 openai.ToolCall
func MakeToolCall(id, name, args string) openai.ToolCall {
	return openai.ToolCall{
		ID: id, Type: openai.ToolTypeFunction,
		Function: openai.FunctionCall{Name: name, Arguments: args},
	}
}

// =====================================================================
// Fake Bash 工具
// =====================================================================

// FakeBash 把"执行过的命令"打上 EXECUTED: 前缀返回
// 用于断言"permission 阻断后 bash 没执行 / 放行后 bash 执行了"
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

// NewTestAgent 用 fake LLM + FakeBash 构造一个 Agent
//
// opts 用于注入权限 Checker / hooks 等可选依赖
func NewTestAgent(t *testing.T, f *FakeLLMServer, opts ...agent.Option) *agent.Agent {
	t.Helper()
	registry := tools.NewRegistry()
	registry.Register(FakeBash{})

	// 用 WithRegistry 把 registry 插到 opts 头部（避免调用方在 opts 里重复写）
	fullOpts := append([]agent.Option{agent.WithRegistry(registry)}, opts...)

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
