package permission

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// =====================================================================
// Pipeline：基础三道闸门串联
// =====================================================================

func TestPipeline_AllowByDefault(t *testing.T) {
	p := &Pipeline{}
	got := p.Check(context.Background(), ToolCall{
		Name: "bash", Args: map[string]any{"command": "echo hi"},
	})
	if got.Decision != DecisionAllow {
		t.Errorf("Decision = %v, want Allow", got.Decision)
	}
}

func TestPipeline_DenyListBlocks(t *testing.T) {
	p := &Pipeline{
		Deny: []Checker{
			&DenyListChecker{Patterns: []DenyPattern{
				{ToolName: "bash", ArgName: "command", Substr: "rm -rf /", Reason: "硬拒绝"},
			}},
		},
	}
	got := p.Check(context.Background(), ToolCall{
		Name: "bash", Args: map[string]any{"command": "rm -rf /"},
	})
	if got.Decision != DecisionDeny {
		t.Errorf("Decision = %v, want Deny", got.Decision)
	}
	if !strings.Contains(got.Reason, "硬拒绝") {
		t.Errorf("Reason = %q, want contains 硬拒绝", got.Reason)
	}
}

func TestPipeline_DenyListNoMatchForOtherArgs(t *testing.T) {
	p := &Pipeline{
		Deny: []Checker{
			&DenyListChecker{Patterns: []DenyPattern{
				{ToolName: "bash", ArgName: "command", Substr: "rm -rf /"},
			}},
		},
	}
	// 工具名不匹配
	got := p.Check(context.Background(), ToolCall{
		Name: "read_file", Args: map[string]any{"command": "rm -rf /"},
	})
	if got.Decision != DecisionAllow {
		t.Errorf("Decision = %v, want Allow (tool mismatch)", got.Decision)
	}
	// 参数名不匹配
	got = p.Check(context.Background(), ToolCall{
		Name: "bash", Args: map[string]any{"other": "rm -rf /"},
	})
	if got.Decision != DecisionAllow {
		t.Errorf("Decision = %v, want Allow (arg mismatch)", got.Decision)
	}
}

func TestPipeline_AskRule_NoAsker_Allows(t *testing.T) {
	// 没装 Asker 时，Ask 视作 Allow
	p := &Pipeline{
		Ask: []Checker{
			&AskRuleChecker{Rules: []AskRule{
				{ToolNames: []string{"bash"}, Check: func(call ToolCall) (bool, string) {
					return true, "potential damage"
				}},
			}},
		},
	}
	got := p.Check(context.Background(), ToolCall{
		Name: "bash", Args: map[string]any{"command": "rm /tmp/a"},
	})
	if got.Decision != DecisionAllow {
		t.Errorf("Decision = %v, want Allow (no asker)", got.Decision)
	}
}

func TestPipeline_AskRule_AskerApprove(t *testing.T) {
	p := &Pipeline{
		Ask: []Checker{
			&AskRuleChecker{Rules: []AskRule{
				{ToolNames: []string{"bash"}, Check: func(call ToolCall) (bool, string) {
					return true, "potential damage"
				}},
			}},
		},
		Asker: AskerFunc(func(_ context.Context, _ ToolCall, reason string) bool {
			if !strings.Contains(reason, "potential damage") {
				t.Errorf("Asker reason = %q, missing rule reason", reason)
			}
			return true
		}),
	}
	got := p.Check(context.Background(), ToolCall{
		Name: "bash", Args: map[string]any{"command": "rm /tmp/a"},
	})
	if got.Decision != DecisionAllow {
		t.Errorf("Decision = %v, want Allow (user approved)", got.Decision)
	}
	if !strings.Contains(got.Reason, "用户已批准") {
		t.Errorf("Reason = %q, missing user-approved marker", got.Reason)
	}
}

func TestPipeline_AskRule_AskerReject(t *testing.T) {
	p := &Pipeline{
		Ask: []Checker{
			&AskRuleChecker{Rules: []AskRule{
				{ToolNames: []string{"bash"}, Check: func(call ToolCall) (bool, string) {
					return true, "potential damage"
				}},
			}},
		},
		Asker: AskerFunc(func(_ context.Context, _ ToolCall, _ string) bool { return false }),
	}
	got := p.Check(context.Background(), ToolCall{
		Name: "bash", Args: map[string]any{"command": "rm /tmp/a"},
	})
	if got.Decision != DecisionDeny {
		t.Errorf("Decision = %v, want Deny (user rejected)", got.Decision)
	}
	if !strings.Contains(got.Reason, "用户拒绝") {
		t.Errorf("Reason = %q, missing user-rejected marker", got.Reason)
	}
}

func TestPipeline_DenyShortCircuitsAsk(t *testing.T) {
	// 即使有 Ask 规则命中，先 Deny 仍然短路
	called := false
	p := &Pipeline{
		Deny: []Checker{
			&DenyListChecker{Patterns: []DenyPattern{
				{ToolName: "bash", ArgName: "command", Substr: "rm -rf /"},
			}},
		},
		Ask: []Checker{
			&AskRuleChecker{Rules: []AskRule{
				{ToolNames: []string{"bash"}, Check: func(call ToolCall) (bool, string) {
					called = true
					return true, "should not reach"
				}},
			}},
		},
		Asker: AskerFunc(func(_ context.Context, _ ToolCall, _ string) bool { return true }),
	}
	got := p.Check(context.Background(), ToolCall{
		Name: "bash", Args: map[string]any{"command": "rm -rf /"},
	})
	if got.Decision != DecisionDeny {
		t.Errorf("Decision = %v, want Deny", got.Decision)
	}
	if called {
		t.Error("Ask rule should not be evaluated when Deny hits")
	}
}

// =====================================================================
// 内置规则
// =====================================================================

func TestDefaultBashDenyList_Blocks(t *testing.T) {
	c := &DenyListChecker{Patterns: DefaultBashDenyList()}
	for _, cmd := range []string{
		"rm -rf /",
		"sudo apt install foo",
		"shutdown -h now",
		"reboot",
		"mkfs.ext4 /dev/sda",
		"dd if=/dev/zero of=/dev/sda",
		"echo x > /dev/sda",
	} {
		got := c.Check(context.Background(), ToolCall{
			Name: "bash", Args: map[string]any{"command": cmd},
		})
		if got.Decision != DecisionDeny {
			t.Errorf("cmd %q expected Deny, got %v", cmd, got.Decision)
		}
	}
}

func TestDefaultBashDenyList_AllowsSafe(t *testing.T) {
	c := &DenyListChecker{Patterns: DefaultBashDenyList()}
	for _, cmd := range []string{
		"echo hello",
		"ls -la",
		"cat main.go",
		"go test ./...",
	} {
		got := c.Check(context.Background(), ToolCall{
			Name: "bash", Args: map[string]any{"command": cmd},
		})
		if got.Decision != DecisionAllow {
			t.Errorf("cmd %q expected Allow, got %v", cmd, got.Decision)
		}
	}
}

func TestDefaultBashAskRules_Triggers(t *testing.T) {
	r := DefaultBashAskRules()
	c := &AskRuleChecker{Rules: []AskRule{r}}
	for _, cmd := range []string{
		"rm foo.txt",
		"rmdir empty_dir",
		"echo x > /etc/passwd",
		"chmod 777 /tmp/a",
		"del file.txt",
		"format c:",
	} {
		got := c.Check(context.Background(), ToolCall{
			Name: "bash", Args: map[string]any{"command": cmd},
		})
		if got.Decision != DecisionAsk {
			t.Errorf("cmd %q expected Ask, got %v", cmd, got.Decision)
		}
	}
}

func TestDefaultBashAskRules_AllowsSafe(t *testing.T) {
	r := DefaultBashAskRules()
	c := &AskRuleChecker{Rules: []AskRule{r}}
	for _, cmd := range []string{
		"echo hello",
		"ls -la",
		"go build ./...",
	} {
		got := c.Check(context.Background(), ToolCall{
			Name: "bash", Args: map[string]any{"command": cmd},
		})
		if got.Decision != DecisionAllow {
			t.Errorf("cmd %q expected Allow, got %v", cmd, got.Decision)
		}
	}
}

func TestWriteOutsideWorkdirRule_Triggers(t *testing.T) {
	rule := WriteOutsideWorkdirRule(t.TempDir())
	c := &AskRuleChecker{Rules: []AskRule{rule}}
	got := c.Check(context.Background(), ToolCall{
		Name: "write_file",
		Args: map[string]any{"path": "/etc/passwd"},
	})
	if got.Decision != DecisionAsk {
		t.Errorf("Decision = %v, want Ask", got.Decision)
	}
}

func TestWriteOutsideWorkdirRule_AllowsInside(t *testing.T) {
	dir := t.TempDir()
	rule := WriteOutsideWorkdirRule(dir)
	c := &AskRuleChecker{Rules: []AskRule{rule}}
	got := c.Check(context.Background(), ToolCall{
		Name: "write_file",
		Args: map[string]any{"path": dir + "/inside.txt"},
	})
	if got.Decision != DecisionAllow {
		t.Errorf("Decision = %v, want Allow", got.Decision)
	}
}

func TestWriteOutsideWorkdirRule_NoWorkdir_NoAsk(t *testing.T) {
	rule := WriteOutsideWorkdirRule("")
	c := &AskRuleChecker{Rules: []AskRule{rule}}
	got := c.Check(context.Background(), ToolCall{
		Name: "write_file",
		Args: map[string]any{"path": "/etc/passwd"},
	})
	if got.Decision != DecisionAllow {
		t.Errorf("Decision = %v, want Allow (workdir empty)", got.Decision)
	}
}

// =====================================================================
// 工具 / Asker 适配
// =====================================================================

type recordingAsker struct {
	asked  int
	answer bool
	err    error
}

func (r *recordingAsker) Ask(_ context.Context, _ ToolCall, _ string) bool {
	r.asked++
	if r.err != nil {
		// 实际中我们不返回 error；这里仅用于演示接口形态
		return false
	}
	return r.answer
}

type fakeErrAsker struct{ err error }

func (f *fakeErrAsker) Ask(_ context.Context, _ ToolCall, _ string) bool {
	return false
}

var _ = errors.New // 保持 import
