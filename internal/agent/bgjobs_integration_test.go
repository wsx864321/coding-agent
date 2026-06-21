package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wsx864321/coding-agent/internal/jobs"
	"github.com/wsx864321/coding-agent/internal/provider"
	"github.com/wsx864321/coding-agent/internal/tools"
)

// newTestAgentWithJobs 构造带 JobManager + bash + bgjobs 工具的测试 agent。
func newTestAgentWithJobs(t *testing.T, f *fakeLLMServer) (*Agent, *jobs.Manager) {
	t.Helper()
	registry := tools.NewRegistry()
	bash := tools.NewBashTool("")
	registry.Register(bash)
	registry.Register(tools.NewBashOutputTool())
	registry.Register(tools.NewKillShellTool())
	registry.Register(tools.NewWaitTool())

	prov, err := provider.New("openai", provider.Config{
		Name:    "openai",
		APIKey:  "test-key",
		BaseURL: f.server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("provider.New: %v", err)
	}

	jobMgr := jobs.NewManager()
	t.Cleanup(jobMgr.Close)

	a, err := NewAgent(Config{
		APIKey:   "test-key",
		BaseURL:  f.server.URL + "/v1",
		MaxTurns: 10,
	},
		WithRegistry(registry),
		WithProvider(prov),
		WithJobManager(jobMgr),
	)
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	return a, jobMgr
}

// TestAgent_BackgroundBash_EndToEnd 验证 bash(run_in_background) 通过 agent loop 启动 job，
// 下一轮通过 wait 拿到结果。
func TestAgent_BackgroundBash_EndToEnd(t *testing.T) {
	// 第 1 轮：LLM 调用 bash(run_in_background=true) 启动后台任务
	// 第 2 轮：LLM 调用 wait 等待完成
	// 第 3 轮：LLM 给出最终回答
	f := newFakeLLM(t,
		scriptedResponse{toolCalls: []provider.ToolCall{
			makeToolCall("call-1", "bash", `{"command":"echo hello-bg","run_in_background":true}`),
		}},
		scriptedResponse{toolCalls: []provider.ToolCall{
			makeToolCall("call-2", "wait", `{}`),
		}},
		scriptedResponse{content: "后台任务完成，输出 hello-bg"},
	)

	a, jobMgr := newTestAgentWithJobs(t, f)
	out, err := a.Run(context.Background(), "请后台执行 echo hello-bg 并等待结果")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out, "hello-bg") {
		t.Errorf("final answer %q should mention hello-bg", out)
	}

	// 验证 job 已完成
	running := jobMgr.Running()
	if len(running) != 0 {
		t.Errorf("expected 0 running jobs, got %d", len(running))
	}
}

// TestAgent_DrainCompletedNote 验证后台任务完成通知在下一轮 Run 注入到 user 消息。
func TestAgent_DrainCompletedNote(t *testing.T) {
	// 第 1 个 Run：启动一个快速后台任务
	// 第 2 个 Run：drain note 应被注入到 user 消息中
	f := newFakeLLM(t,
		// Run 1, turn 1: 启动后台任务
		scriptedResponse{toolCalls: []provider.ToolCall{
			makeToolCall("call-1", "bash", `{"command":"echo done-note","run_in_background":true}`),
		}},
		// Run 1, turn 2: 最终回答（无 tool call）
		scriptedResponse{content: "已启动"},
		// Run 2, turn 1: 最终回答（drain note 已注入到 user 消息）
		scriptedResponse{content: "收到完成通知"},
	)

	a, _ := newTestAgentWithJobs(t, f)

	// 第 1 次 Run：启动后台任务
	_, err := a.Run(context.Background(), "后台执行 echo done-note")
	if err != nil {
		t.Fatalf("Run 1: %v", err)
	}

	// 等待后台任务完成
	time.Sleep(100 * time.Millisecond)

	// 第 2 次 Run：drain note 应注入到 user 消息
	_, err = a.Run(context.Background(), "继续")
	if err != nil {
		t.Fatalf("Run 2: %v", err)
	}

	// 验证第 2 次 Run 的 user 消息包含了 drain note
	found := false
	for _, m := range a.Messages() {
		if m.Role == provider.RoleUser && strings.Contains(m.Content, "后台任务已完成") {
			found = true
			break
		}
	}
	if !found {
		t.Error("drain note not found in message history")
	}
}

// TestAgent_BackgroundBash_Kill 验证 kill_shell 通过 agent loop 终止后台任务。
func TestAgent_BackgroundBash_Kill(t *testing.T) {
	// 第 1 轮：启动一个长时间后台任务
	// 第 2 轮：调用 kill_shell 终止
	// 第 3 轮：最终回答
	f := newFakeLLM(t,
		scriptedResponse{toolCalls: []provider.ToolCall{
			makeToolCall("call-1", "bash", `{"command":"ping -n 30 127.0.0.1","run_in_background":true}`),
		}},
		scriptedResponse{toolCalls: []provider.ToolCall{
			makeToolCall("call-2", "kill_shell", `{"job_id":"bash-1"}`),
		}},
		scriptedResponse{content: "已终止"},
	)

	a, jobMgr := newTestAgentWithJobs(t, f)
	out, err := a.Run(context.Background(), "后台执行长命令然后终止它")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out, "终止") {
		t.Errorf("final answer %q should mention kill", out)
	}

	// 等待 job goroutine 退出
	time.Sleep(100 * time.Millisecond)
	running := jobMgr.Running()
	if len(running) != 0 {
		t.Errorf("expected 0 running jobs after kill, got %d", len(running))
	}
}

// TestAgent_BashOutput_ReadIncremental 验证 bash_output 通过 agent loop 增量读取。
func TestAgent_BashOutput_ReadIncremental(t *testing.T) {
	// 第 1 轮：启动后台任务
	// 第 2 轮：调 bash_output 读取输出
	// 第 3 轮：最终回答
	f := newFakeLLM(t,
		scriptedResponse{toolCalls: []provider.ToolCall{
			makeToolCall("call-1", "bash", `{"command":"echo output-line","run_in_background":true}`),
		}},
		scriptedResponse{toolCalls: []provider.ToolCall{
			makeToolCall("call-2", "bash_output", `{"job_id":"bash-1"}`),
		}},
		scriptedResponse{content: "已读取输出 output-line"},
	)

	a, _ := newTestAgentWithJobs(t, f)
	out, err := a.Run(context.Background(), "后台执行并读取输出")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out, "output-line") {
		t.Errorf("final answer %q should mention output-line", out)
	}
}

// TestAgent_BackgroundNoManager 验证无 JobManager 时 run_in_background 降级为错误。
func TestAgent_BackgroundNoManager(t *testing.T) {
	// 不注入 JobManager
	registry := tools.NewRegistry()
	registry.Register(tools.NewBashTool(""))

	f := newFakeLLM(t,
		scriptedResponse{toolCalls: []provider.ToolCall{
			makeToolCall("call-1", "bash", `{"command":"echo x","run_in_background":true}`),
		}},
		scriptedResponse{content: "后台不可用，改用同步"},
	)

	prov, err := provider.New("openai", provider.Config{
		Name: "openai", APIKey: "test-key", BaseURL: f.server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("provider.New: %v", err)
	}

	a, err := NewAgent(Config{
		APIKey: "test-key", BaseURL: f.server.URL + "/v1", MaxTurns: 5,
	}, WithRegistry(registry), WithProvider(prov))
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}

	_, err = a.Run(context.Background(), "后台执行 echo x")
	if err != nil {
		t.Fatalf("Run should not fail: %v", err)
	}

	// 第 1 轮的 tool result 应包含错误信息
	for _, m := range a.Messages() {
		if m.Role == provider.RoleTool && m.Name == "bash" {
			if !strings.Contains(m.Content, "Error") {
				t.Errorf("bash result %q should contain error about no manager", m.Content)
			}
			return
		}
	}
	t.Error("bash tool result not found in messages")
}

// TestAgent_PromptIncludesBackgroundGuidance 验证 system prompt 包含后台任务引导。
func TestAgent_PromptIncludesBackgroundGuidance(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(tools.NewBashTool(""))
	registry.Register(tools.NewBashOutputTool())
	registry.Register(tools.NewWaitTool())

	f := newFakeLLM(t, scriptedResponse{content: "ok"})
	prov, _ := provider.New("openai", provider.Config{
		Name: "openai", APIKey: "test-key", BaseURL: f.server.URL + "/v1",
	})

	a, err := NewAgent(Config{
		APIKey: "test-key", BaseURL: f.server.URL + "/v1", MaxTurns: 5,
	}, WithRegistry(registry), WithProvider(prov))
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}

	sysMsg := a.Messages()[0].Content
	if !strings.Contains(sysMsg, "run_in_background") {
		t.Error("system prompt should mention run_in_background")
	}
	if !strings.Contains(sysMsg, "bash_output") {
		t.Error("system prompt should mention bash_output")
	}
}

// TestWithJobManager 验证 WithJobManager Option 注入。
func TestWithJobManager(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "ok"})
	prov, _ := provider.New("openai", provider.Config{
		Name: "openai", APIKey: "test-key", BaseURL: f.server.URL + "/v1",
	})

	mgr := jobs.NewManager()
	defer mgr.Close()

	a, err := NewAgent(Config{
		APIKey: "test-key", BaseURL: f.server.URL + "/v1", MaxTurns: 5,
	}, WithProvider(prov), WithJobManager(mgr))
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	if a.JobManager() != mgr {
		t.Error("JobManager() != injected manager")
	}
}

// TestWithJobManager_Nil 验证 nil JobManager 不 panic。
func TestWithJobManager_Nil(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "ok"})
	prov, _ := provider.New("openai", provider.Config{
		Name: "openai", APIKey: "test-key", BaseURL: f.server.URL + "/v1",
	})

	a, err := NewAgent(Config{
		APIKey: "test-key", BaseURL: f.server.URL + "/v1", MaxTurns: 5,
	}, WithProvider(prov), WithJobManager(nil))
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	if a.JobManager() != nil {
		t.Error("JobManager() should be nil")
	}
}

// 防止 json import 被优化掉（某些测试可能不直接用 json）
var _ = json.RawMessage(nil)
