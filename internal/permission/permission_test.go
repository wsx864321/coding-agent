package permission

import (
	"context"
	"strings"
	"testing"
)

// =====================================================================
// Pipeline：基础 Deny 闸门
// =====================================================================

func TestPipeline_AllowByDefault(t *testing.T) {
	p := &Pipeline{}
	got := p.Check(context.Background(), "bash", map[string]any{"command": "echo hi"})
	if got.Decision != DecisionAllow {
		t.Errorf("Decision = %v, want Allow", got.Decision)
	}
}

func TestPipeline_DenyListBlocks(t *testing.T) {
	p := &Pipeline{
		Deny: []Checker{
			NewDenyListCheckerWith(DenyPattern{
				ToolName: "bash", ArgName: "command", Substr: "rm -rf /", Reason: "硬拒绝",
			}),
		},
	}
	got := p.Check(context.Background(), "bash", map[string]any{"command": "rm -rf /"})
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
			NewDenyListCheckerWith(DenyPattern{
				ToolName: "bash", ArgName: "command", Substr: "rm -rf /",
			}),
		},
	}
	// 工具名不匹配
	got := p.Check(context.Background(), "read_file", map[string]any{"command": "rm -rf /"})
	if got.Decision != DecisionAllow {
		t.Errorf("Decision = %v, want Allow (tool mismatch)", got.Decision)
	}
	// 参数名不匹配
	got = p.Check(context.Background(), "bash", map[string]any{"other": "rm -rf /"})
	if got.Decision != DecisionAllow {
		t.Errorf("Decision = %v, want Allow (arg mismatch)", got.Decision)
	}
}

// =====================================================================
// 内置规则
// =====================================================================

func TestDefaultBashDenyList_Blocks(t *testing.T) {
	c := NewDenyListChecker()
	for _, cmd := range []string{
		"rm -rf /",
		"sudo apt install foo",
		"shutdown -h now",
		"reboot",
		"mkfs.ext4 /dev/sda",
		"dd if=/dev/zero of=/dev/sda",
		"echo x > /dev/sda",
	} {
		got := c.Check(context.Background(), "bash", map[string]any{"command": cmd})
		if got.Decision != DecisionDeny {
			t.Errorf("cmd %q expected Deny, got %v", cmd, got.Decision)
		}
	}
}

func TestDefaultBashDenyList_AllowsSafe(t *testing.T) {
	c := NewDenyListChecker()
	for _, cmd := range []string{
		"echo hello",
		"ls -la",
		"cat main.go",
		"go test ./...",
	} {
		got := c.Check(context.Background(), "bash", map[string]any{"command": cmd})
		if got.Decision != DecisionAllow {
			t.Errorf("cmd %q expected Allow, got %v", cmd, got.Decision)
		}
	}
}

// =====================================================================
// BashAskChecker
// =====================================================================

func TestBashAskChecker_NoAsker_Allows(t *testing.T) {
	c := NewBashAskChecker(nil)
	got := c.Check(context.Background(), "bash", map[string]any{"command": "rm foo"})
	if got.Decision != DecisionAllow {
		t.Errorf("Decision = %v, want Allow (no asker)", got.Decision)
	}
}

func TestBashAskChecker_AskerApprove(t *testing.T) {
	c := NewBashAskChecker(AskerFunc(func(_ context.Context, _ string, _ map[string]any, reason string) bool {
		if !strings.Contains(reason, "rm ") {
			t.Errorf("Asker reason = %q, missing kw", reason)
		}
		return true
	}))
	got := c.Check(context.Background(), "bash", map[string]any{"command": "rm foo"})
	if got.Decision != DecisionAllow {
		t.Errorf("Decision = %v, want Allow (user approved)", got.Decision)
	}
	if !strings.Contains(got.Reason, "用户已批准") {
		t.Errorf("Reason = %q, missing user-approved marker", got.Reason)
	}
}

func TestBashAskChecker_AskerReject(t *testing.T) {
	c := NewBashAskChecker(AskerFunc(func(_ context.Context, _ string, _ map[string]any, _ string) bool {
		return false
	}))
	got := c.Check(context.Background(), "bash", map[string]any{"command": "rm foo"})
	if got.Decision != DecisionDeny {
		t.Errorf("Decision = %v, want Deny (user rejected)", got.Decision)
	}
	if !strings.Contains(got.Reason, "用户拒绝") {
		t.Errorf("Reason = %q, missing user-rejected marker", got.Reason)
	}
}

func TestBashAskChecker_SafeCommand_Allows(t *testing.T) {
	c := NewBashAskChecker(AskerFunc(func(_ context.Context, _ string, _ map[string]any, _ string) bool {
		t.Error("Asker should not be called for safe commands")
		return false
	}))
	got := c.Check(context.Background(), "bash", map[string]any{"command": "echo hi"})
	if got.Decision != DecisionAllow {
		t.Errorf("Decision = %v, want Allow", got.Decision)
	}
}

func TestBashAskChecker_NotBash_Allows(t *testing.T) {
	c := NewBashAskChecker(AskerFunc(func(_ context.Context, _ string, _ map[string]any, _ string) bool {
		t.Error("Asker should not be called for non-bash tools")
		return false
	}))
	got := c.Check(context.Background(), "read_file", map[string]any{"command": "rm foo"})
	if got.Decision != DecisionAllow {
		t.Errorf("Decision = %v, want Allow", got.Decision)
	}
}

// =====================================================================
// WorkdirChecker
// =====================================================================

func TestWorkdirChecker_NoWorkdir_Allows(t *testing.T) {
	c := NewWorkdirChecker("", nil)
	got := c.Check(context.Background(), "write_file", map[string]any{"path": "/etc/passwd"})
	if got.Decision != DecisionAllow {
		t.Errorf("Decision = %v, want Allow (no workdir)", got.Decision)
	}
}

func TestWorkdirChecker_Inside_Allows(t *testing.T) {
	dir := t.TempDir()
	c := NewWorkdirChecker(dir, nil)
	got := c.Check(context.Background(), "write_file", map[string]any{"path": dir + "/inside.txt"})
	if got.Decision != DecisionAllow {
		t.Errorf("Decision = %v, want Allow (inside)", got.Decision)
	}
}

func TestWorkdirChecker_Outside_AskerApprove(t *testing.T) {
	dir := t.TempDir()
	c := NewWorkdirChecker(dir, AskerFunc(func(_ context.Context, _ string, _ map[string]any, _ string) bool {
		return true
	}))
	got := c.Check(context.Background(), "write_file", map[string]any{"path": "/etc/passwd"})
	if got.Decision != DecisionAllow {
		t.Errorf("Decision = %v, want Allow (user approved)", got.Decision)
	}
}

func TestWorkdirChecker_Outside_AskerReject(t *testing.T) {
	dir := t.TempDir()
	c := NewWorkdirChecker(dir, AskerFunc(func(_ context.Context, _ string, _ map[string]any, _ string) bool {
		return false
	}))
	got := c.Check(context.Background(), "write_file", map[string]any{"path": "/etc/passwd"})
	if got.Decision != DecisionDeny {
		t.Errorf("Decision = %v, want Deny (user rejected)", got.Decision)
	}
}

func TestWorkdirChecker_Outside_NoAsker_Allows(t *testing.T) {
	// 无 Asker 时越界也放行（与 Pipeline 行为一致）
	dir := t.TempDir()
	c := NewWorkdirChecker(dir, nil)
	got := c.Check(context.Background(), "write_file", map[string]any{"path": "/etc/passwd"})
	if got.Decision != DecisionAllow {
		t.Errorf("Decision = %v, want Allow (no asker)", got.Decision)
	}
}

func TestWorkdirChecker_NotWrite_Allows(t *testing.T) {
	dir := t.TempDir()
	c := NewWorkdirChecker(dir, AskerFunc(func(_ context.Context, _ string, _ map[string]any, _ string) bool {
		t.Error("Asker should not be called for non-write tools")
		return false
	}))
	got := c.Check(context.Background(), "bash", map[string]any{"path": "/etc/passwd"})
	if got.Decision != DecisionAllow {
		t.Errorf("Decision = %v, want Allow", got.Decision)
	}
}
