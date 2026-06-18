package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/wsx864321/coding-agent/internal/provider"
	"github.com/wsx864321/coding-agent/internal/tools"
)

func newCompactionAgent(t *testing.T, f *fakeLLMServer) *Agent {
	t.Helper()
	reg := tools.NewRegistry()
	reg.Register(echoTool{})
	reg.Register(failTool{})
	reg.Register(tools.NewCompactTool())

	prov, _ := provider.New("openai", provider.Config{
		Name: "openai", APIKey: "test-key", BaseURL: f.server.URL + "/v1",
	})

	a, err := NewAgent(Config{
		APIKey:            "test-key",
		BaseURL:           f.server.URL + "/v1",
		MaxTurns:          8,
		ContextWindow:     1200,
		SoftCompactRatio:  0.5,
		CompactRatio:      0.8,
		CompactForceRatio: 0.9,
		RecentKeep:        2,
		MaxMessagesSnip:   80,
	}, WithRegistry(reg), WithProvider(prov))
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	return a
}

func seedLongHistory(a *Agent) {
	large := strings.Repeat("implementation detail ", 80)
	a.messages = append(a.messages,
		provider.Message{Role: provider.RoleUser, Content: "task goal"},
		provider.Message{Role: provider.RoleAssistant, Content: large},
		provider.Message{Role: provider.RoleTool, Name: "read_file", ToolCallID: "x1", Content: large},
		provider.Message{Role: provider.RoleUser, Content: "constraint: always use pnpm"},
		provider.Message{Role: provider.RoleAssistant, Content: large},
		provider.Message{Role: provider.RoleUser, Content: "continue"},
		provider.Message{Role: provider.RoleAssistant, Content: "ok"},
	)
}

func TestCompactTool_ManualCompaction(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []provider.ToolCall{
				makeToolCall("call_1", "compact", `{"focus":"保留决策与文件改动"}`),
			},
		},
		scriptedResponse{content: "## Goal\n- continue task\n## Decisions & rationale\n- keep pnpm"},
		scriptedResponse{content: "done"},
	)
	a := newCompactionAgent(t, f)
	a.contextWindow = 0
	a.archiveDir = t.TempDir()
	seedLongHistory(a)

	out, err := a.Run(context.Background(), "trigger compact")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "done" {
		t.Fatalf("out = %q, want done", out)
	}
	joined := make([]string, 0, len(a.messages))
	for _, m := range a.messages {
		joined = append(joined, m.Content)
	}
	all := strings.Join(joined, "\n")
	if !strings.Contains(all, summaryTagOpen) {
		t.Fatalf("expected compaction summary tag in messages, got: %s", all)
	}
}

func TestPruneStaleToolResults_ElidesOldToolOutput(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "ok"})
	a := newCompactionAgent(t, f)
	seedLongHistory(a)
	before := a.messages[3].Content
	if len(before) < minPruneBytes {
		t.Fatal("seed tool output too small for prune test")
	}
	if err := a.pruneStaleToolResults(); err != nil {
		t.Fatalf("pruneStaleToolResults: %v", err)
	}
	got := a.messages[3].Content
	if !strings.Contains(got, "历史工具结果已折叠") {
		t.Fatalf("expected pruned marker, got %q", got)
	}
}

func TestMaybeCompact_AutoCompactsWhenOverThreshold(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "## Goal\n- continue task"})
	a := newCompactionAgent(t, f)
	seedLongHistory(a)
	a.contextWindow = 600

	a.maybeCompact(context.Background(), estimateMessagesTokens(a.messages))

	found := false
	for _, m := range a.messages {
		if strings.Contains(m.Content, summaryTagOpen) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("auto compact did not inject summary message")
	}
}

func TestMaybeCompact_SkipsPruneWhenBelowThreshold(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "ok"})
	a := newCompactionAgent(t, f)
	seedLongHistory(a)
	msgCountBefore := len(a.messages)

	a.maybeCompact(context.Background(), 500)

	if len(a.messages) != msgCountBefore {
		t.Fatalf("below threshold should not touch messages: was %d, now %d", msgCountBefore, len(a.messages))
	}
}

func TestSnipCompact_DoesNotLeaveTailStartingWithTool(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "ok"})
	a := newCompactionAgent(t, f)
	a.maxMessagesSnip = 6

	a.messages = append(a.messages,
		provider.Message{Role: provider.RoleUser, Content: "u1"},
		provider.Message{Role: provider.RoleAssistant, Content: "a1"},
		provider.Message{
			Role: provider.RoleAssistant,
			ToolCalls: []provider.ToolCall{
				makeToolCall("t1", "echo", `{"input":"1"}`),
				makeToolCall("t2", "echo", `{"input":"2"}`),
			},
			Content: " ",
		},
		provider.Message{Role: provider.RoleTool, ToolCallID: "t1", Name: "echo", Content: "r1"},
		provider.Message{Role: provider.RoleTool, ToolCallID: "t2", Name: "echo", Content: "r2"},
		provider.Message{Role: provider.RoleUser, Content: "u2"},
		provider.Message{Role: provider.RoleAssistant, Content: "a2"},
	)

	a.snipCompact()

	for i, m := range a.messages {
		if m.Role != provider.RoleTool {
			continue
		}
		if i == 0 {
			t.Fatalf("tool 消息不应出现在首位")
		}
		j := i - 1
		for j >= 0 && a.messages[j].Role == provider.RoleTool {
			j--
		}
		if j < 0 || a.messages[j].Role != provider.RoleAssistant || len(a.messages[j].ToolCalls) == 0 {
			t.Fatalf("发现孤儿 tool 消息（index=%d）", i)
		}
	}
}
