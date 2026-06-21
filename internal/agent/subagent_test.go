package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/wsx864321/coding-agent/internal/provider"
	"github.com/wsx864321/coding-agent/internal/tools"
)

// =====================================================================
// RunSubAgent 测试
// =====================================================================

func newParentAgent(t *testing.T, subLLM *fakeLLMServer) *Agent {
	t.Helper()
	registry := tools.NewRegistry()
	registry.Register(echoTool{})
	registry.Register(failTool{})

	prov, _ := provider.New("openai", provider.Config{
		Name: "openai", APIKey: "test-key", BaseURL: subLLM.server.URL + "/v1",
	})

	a, err := NewAgent(Config{
		APIKey:   "test-key",
		BaseURL:  subLLM.server.URL + "/v1",
		MaxTurns: 10,
	}, WithRegistry(registry), WithProvider(prov))
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	return a
}

func TestRunSubAgent_BasicSuccess(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{content: "子 agent 完成任务"},
	)
	parent := newParentAgent(t, f)

	answer, err := RunSubAgent(context.Background(), parent, "请列出所有文件", SubagentOptions{})
	if err != nil {
		t.Fatalf("RunSubAgent: %v", err)
	}
	if !strings.Contains(answer, "子 agent 完成任务") {
		t.Errorf("answer = %q, want contains '子 agent 完成任务'", answer)
	}
}

func TestRunSubAgent_EmptyPrompt(t *testing.T) {
	f := newFakeLLM(t)
	parent := newParentAgent(t, f)

	_, err := RunSubAgent(context.Background(), parent, "  ", SubagentOptions{})
	if err == nil {
		t.Fatal("expected error for empty prompt")
	}
	if !strings.Contains(err.Error(), "不能为空") {
		t.Errorf("error = %q, want contains '不能为空'", err.Error())
	}
}

func TestRunSubAgent_CustomMaxTurns(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{content: "sub ok"},
	)
	parent := newParentAgent(t, f)

	answer, err := RunSubAgent(context.Background(), parent, "test", SubagentOptions{
		MaxTurns: 3,
	})
	if err != nil {
		t.Fatalf("RunSubAgent: %v", err)
	}
	if answer != "sub ok" {
		t.Errorf("answer = %q, want 'sub ok'", answer)
	}
}

func TestRunSubAgent_CustomSystemPrompt(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{content: "custom sub"},
	)
	parent := newParentAgent(t, f)

	answer, err := RunSubAgent(context.Background(), parent, "test", SubagentOptions{
		SystemPrompt: "你是一个代码审查助手",
	})
	if err != nil {
		t.Fatalf("RunSubAgent: %v", err)
	}
	if answer != "custom sub" {
		t.Errorf("answer = %q, want 'custom sub'", answer)
	}
}

func TestRunSubAgent_ToolCallInSubagent(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []provider.ToolCall{
				makeToolCall("sub_call_1", "echo", `{"input":"sub-hello"}`),
			},
		},
		scriptedResponse{content: "子 agent: 发现了 echoed: sub-hello"},
	)
	parent := newParentAgent(t, f)

	answer, err := RunSubAgent(context.Background(), parent, "请调用 echo 工具", SubagentOptions{})
	if err != nil {
		t.Fatalf("RunSubAgent: %v", err)
	}
	if !strings.Contains(answer, "echoed: sub-hello") {
		t.Errorf("answer = %q, want contains 'echoed: sub-hello'", answer)
	}
}

func TestRunSubAgent_IsolatedMessages(t *testing.T) {
	parentLLM := newFakeLLM(t,
		scriptedResponse{content: "parent answer"},
	)
	parent := newParentAgent(t, parentLLM)

	_, err := parent.Run(context.Background(), "parent question")
	if err != nil {
		t.Fatalf("parent.Run: %v", err)
	}
	parentMsgCount := len(parent.messages)

	subLLM := newFakeLLM(t,
		scriptedResponse{content: "sub answer"},
	)
	// subagent 使用独立的 provider
	subProv, _ := provider.New("openai", provider.Config{
		Name: "openai", APIKey: "test-key", BaseURL: subLLM.server.URL + "/v1",
	})
	parent.prov = subProv

	_, err = RunSubAgent(context.Background(), parent, "sub question", SubagentOptions{})
	if err != nil {
		t.Fatalf("RunSubAgent: %v", err)
	}

	if len(parent.messages) != parentMsgCount {
		t.Errorf("parent messages changed: before=%d, after=%d", parentMsgCount, len(parent.messages))
	}
}

func TestRunSubAgent_CustomRegistry(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{content: "sub with custom tools"},
	)
	parent := newParentAgent(t, f)

	customReg := tools.NewRegistry()
	customReg.Register(echoTool{})

	answer, err := RunSubAgent(context.Background(), parent, "test", SubagentOptions{
		Registry: customReg,
	})
	if err != nil {
		t.Fatalf("RunSubAgent: %v", err)
	}
	if answer != "sub with custom tools" {
		t.Errorf("answer = %q", answer)
	}
}

func TestRunSubAgent_DefaultMaxTurns(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{content: "sub ok"},
	)
	parent := newParentAgent(t, f)

	answer, err := RunSubAgent(context.Background(), parent, "test", SubagentOptions{})
	if err != nil {
		t.Fatalf("RunSubAgent: %v", err)
	}
	if answer != "sub ok" {
		t.Errorf("answer = %q", answer)
	}
}

// =====================================================================
// SubagentMetaTools
// =====================================================================

func TestSubagentMetaTools(t *testing.T) {
	meta := SubagentMetaTools()
	expected := map[string]bool{
		"task":          true,
		"todo_write":    true,
		"complete_step": true,
		"run_skill":     true,
		"install_skill": true,
		"bash_output":   true,
		"kill_shell":    true,
		"wait":          true,
	}
	for _, name := range meta {
		if !expected[name] {
			t.Errorf("unexpected meta tool: %q", name)
		}
	}
	if len(meta) != len(expected) {
		t.Errorf("meta tools count=%d, want %d", len(meta), len(expected))
	}
}

// =====================================================================
// WireTaskTool
// =====================================================================

func TestWireTaskTool_NoTaskTool(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "ok"})
	a := newTestAgent(t, f)
	a.WireTaskTool()
}

func TestWireTaskTool_WithTaskTool(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{content: "wired sub answer"},
	)
	tt := tools.NewTaskTool(nil)
	a := newTestAgent(t, f, tt)
	a.WireTaskTool()

	answer, err := tt.Execute(context.Background(), map[string]any{
		"prompt": "test task",
	})
	if err != nil {
		t.Fatalf("TaskTool.Execute after WireTaskTool: %v", err)
	}
	if !strings.Contains(answer, "wired sub answer") {
		t.Errorf("answer = %q, want contains 'wired sub answer'", answer)
	}
}
