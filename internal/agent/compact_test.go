package agent

import (
	"context"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"

	"github.com/wsx864321/coding-agent/internal/tools"
)

func newCompactionAgent(t *testing.T, f *fakeLLMServer) *Agent {
	t.Helper()
	reg := tools.NewRegistry()
	reg.Register(echoTool{})
	reg.Register(failTool{})
	reg.Register(tools.NewCompactTool())

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
	}, WithRegistry(reg))
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	a.client = makeClientWithFake(t, f)
	return a
}

func seedLongHistory(a *Agent) {
	large := strings.Repeat("implementation detail ", 80)
	a.messages = append(a.messages,
		openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: "task goal"},
		openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: large},
		openai.ChatCompletionMessage{Role: openai.ChatMessageRoleTool, Name: "read_file", ToolCallID: "x1", Content: large},
		openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: "constraint: always use pnpm"},
		openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: large},
		openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: "continue"},
		openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: "ok"},
	)
}

func TestCompactTool_ManualCompaction(t *testing.T) {
	f := newFakeLLM(t,
		scriptedResponse{
			toolCalls: []openai.ToolCall{
				makeToolCall("call_1", "compact", `{"focus":"保留决策与文件改动"}`),
			},
		},
		// manual compact 内部 summarize 调用
		scriptedResponse{content: "## Goal\n- continue task\n## Decisions & rationale\n- keep pnpm"},
		// 主循环继续
		scriptedResponse{content: "done"},
	)
	a := newCompactionAgent(t, f)
	// 该用例只验证“手动 compact”链路，避免 maybeCompact 先触发自动压缩打乱脚本响应序列。
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
	// 强制低窗口，确保阈值命中。
	a.contextWindow = 600

	a.maybeCompact(context.Background())

	found := false
	for _, m := range a.messages {
		if strings.Contains(m.Content, summaryTagOpen) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("auto compact did not inject summary message: %+v", a.messages)
	}
}

func TestSnipCompact_DoesNotLeaveTailStartingWithTool(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "ok"})
	a := newCompactionAgent(t, f)
	a.maxMessagesSnip = 6

	// 构造：中间有多条 tool 消息，snip 若从第二条 tool 开始会导致孤儿 tool。
	a.messages = append(a.messages,
		openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: "u1"},
		openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: "a1"},
		openai.ChatCompletionMessage{
			Role: openai.ChatMessageRoleAssistant,
			ToolCalls: []openai.ToolCall{
				makeToolCall("t1", "echo", `{"input":"1"}`),
				makeToolCall("t2", "echo", `{"input":"2"}`),
			},
			Content: " ",
		},
		openai.ChatCompletionMessage{Role: openai.ChatMessageRoleTool, ToolCallID: "t1", Name: "echo", Content: "r1"},
		openai.ChatCompletionMessage{Role: openai.ChatMessageRoleTool, ToolCallID: "t2", Name: "echo", Content: "r2"},
		openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: "u2"},
		openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: "a2"},
	)

	a.snipCompact()

	for i, m := range a.messages {
		if m.Role != openai.ChatMessageRoleTool {
			continue
		}
		if i == 0 {
			t.Fatalf("tool 消息不应出现在首位: %+v", a.messages)
		}
		// 找到前一个非 tool 消息，必须是 assistant 且带 tool_calls。
		j := i - 1
		for j >= 0 && a.messages[j].Role == openai.ChatMessageRoleTool {
			j--
		}
		if j < 0 || a.messages[j].Role != openai.ChatMessageRoleAssistant || len(a.messages[j].ToolCalls) == 0 {
			t.Fatalf("发现孤儿 tool 消息（index=%d）: prev_non_tool=%+v, messages=%+v", i, func() any {
				if j >= 0 {
					return a.messages[j]
				}
				return nil
			}(), a.messages)
		}
	}
}

func TestEnsureToolMessageLinks_RepairsOrphanToolMessage(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "ok"})
	a := newCompactionAgent(t, f)

	a.messages = append(a.messages,
		openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: "u1"},
		openai.ChatCompletionMessage{Role: openai.ChatMessageRoleTool, ToolCallID: "orphan-1", Name: "echo", Content: "tool output"},
	)

	a.ensureToolMessageLinks()

	foundPair := false
	for i := 1; i < len(a.messages); i++ {
		if a.messages[i].Role != openai.ChatMessageRoleTool {
			continue
		}
		prev := a.messages[i-1]
		if prev.Role == openai.ChatMessageRoleAssistant && len(prev.ToolCalls) > 0 && prev.ToolCalls[0].ID == a.messages[i].ToolCallID {
			foundPair = true
			break
		}
	}
	if !foundPair {
		t.Fatalf("孤儿 tool 未被修复为合法 tool 对：%+v", a.messages)
	}
}

func TestEnsureToolMessageLinks_DowngradesToolWithoutID(t *testing.T) {
	f := newFakeLLM(t, scriptedResponse{content: "ok"})
	a := newCompactionAgent(t, f)

	a.messages = append(a.messages,
		openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: "u1"},
		openai.ChatCompletionMessage{Role: openai.ChatMessageRoleTool, Name: "echo", Content: "tool output"},
	)

	a.ensureToolMessageLinks()

	for _, m := range a.messages {
		if m.Role == openai.ChatMessageRoleTool && m.ToolCallID == "" {
			t.Fatalf("无 id 的 tool 消息应被降级，当前仍存在：%+v", a.messages)
		}
	}
}
