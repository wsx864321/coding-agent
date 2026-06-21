package tools

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/wsx864321/coding-agent/internal/jobs"
)

// =====================================================================
// helper: build a context carrying a Manager
// =====================================================================

func bgCtx(m *jobs.Manager) context.Context {
	return jobs.WithManager(context.Background(), m)
}

// =====================================================================
// bash_output
// =====================================================================

func TestBashOutputTool_Name(t *testing.T) {
	if NewBashOutputTool().Name() != "bash_output" {
		t.Error("Name != bash_output")
	}
}

func TestBashOutputTool_ReadOnly(t *testing.T) {
	if !NewBashOutputTool().ReadOnly() {
		t.Error("bash_output should be ReadOnly")
	}
}

func TestBashOutputTool_Success(t *testing.T) {
	m := jobs.NewManager()
	defer m.Close()
	ctx := bgCtx(m)

	j := m.Start("bash", "echo", func(ctx context.Context, out io.Writer) (string, error) {
		_, _ = out.Write([]byte("line1\nline2\n"))
		return "", nil
	})
	<-j.Done()

	out, err := NewBashOutputTool().Execute(ctx, map[string]any{"job_id": j.ID})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "line1") || !strings.Contains(out, "line2") {
		t.Errorf("output %q missing lines", out)
	}
	if !strings.Contains(out, "done") {
		t.Errorf("output %q missing status", out)
	}
}

func TestBashOutputTool_Filter(t *testing.T) {
	m := jobs.NewManager()
	defer m.Close()
	ctx := bgCtx(m)

	j := m.Start("bash", "multi", func(ctx context.Context, out io.Writer) (string, error) {
		_, _ = out.Write([]byte("error: foo\ninfo: bar\nerror: baz\n"))
		return "", nil
	})
	<-j.Done()

	out, err := NewBashOutputTool().Execute(ctx, map[string]any{
		"job_id": j.ID,
		"filter": "error",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "foo") || !strings.Contains(out, "baz") {
		t.Errorf("filtered output %q missing error lines", out)
	}
	if strings.Contains(out, "bar") {
		t.Errorf("filtered output %q should not contain info line", out)
	}
}

func TestBashOutputTool_NoNewOutput(t *testing.T) {
	m := jobs.NewManager()
	defer m.Close()
	ctx := bgCtx(m)

	j := m.Start("bash", "quick", func(ctx context.Context, out io.Writer) (string, error) { return "", nil })
	<-j.Done()

	// first read drains all
	_, _ = NewBashOutputTool().Execute(ctx, map[string]any{"job_id": j.ID})
	// second read should report no new output
	out, err := NewBashOutputTool().Execute(ctx, map[string]any{"job_id": j.ID})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "no new output") && !strings.Contains(out, "无新输出") {
		t.Errorf("output %q should mention no new output", out)
	}
}

func TestBashOutputTool_UnknownID(t *testing.T) {
	m := jobs.NewManager()
	defer m.Close()
	ctx := bgCtx(m)

	_, err := NewBashOutputTool().Execute(ctx, map[string]any{"job_id": "nope-999"})
	if err == nil {
		t.Fatal("expected error for unknown job_id")
	}
}

func TestBashOutputTool_NoManager(t *testing.T) {
	_, err := NewBashOutputTool().Execute(context.Background(), map[string]any{"job_id": "bash-1"})
	if err == nil {
		t.Fatal("expected error when no manager in context")
	}
}

func TestBashOutputTool_EmptyJobID(t *testing.T) {
	m := jobs.NewManager()
	defer m.Close()
	ctx := bgCtx(m)

	_, err := NewBashOutputTool().Execute(ctx, map[string]any{"job_id": ""})
	if err == nil {
		t.Fatal("expected error for empty job_id")
	}
}

func TestBashOutputTool_InvalidFilter(t *testing.T) {
	m := jobs.NewManager()
	defer m.Close()
	ctx := bgCtx(m)

	j := m.Start("bash", "x", func(ctx context.Context, out io.Writer) (string, error) {
		_, _ = out.Write([]byte("data"))
		return "", nil
	})
	<-j.Done()

	_, err := NewBashOutputTool().Execute(ctx, map[string]any{
		"job_id": j.ID,
		"filter": "(unclosed",
	})
	if err == nil {
		t.Fatal("expected error for invalid filter regex")
	}
}

// =====================================================================
// kill_shell
// =====================================================================

func TestKillShellTool_Name(t *testing.T) {
	if NewKillShellTool().Name() != "kill_shell" {
		t.Error("Name != kill_shell")
	}
}

func TestKillShellTool_ReadOnly(t *testing.T) {
	if NewKillShellTool().ReadOnly() {
		t.Error("kill_shell should NOT be ReadOnly")
	}
}

func TestKillShellTool_Running(t *testing.T) {
	m := jobs.NewManager()
	defer m.Close()
	ctx := bgCtx(m)

	j := m.Start("bash", "block", func(ctx context.Context, out io.Writer) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	time.Sleep(20 * time.Millisecond)

	out, err := NewKillShellTool().Execute(ctx, map[string]any{"job_id": j.ID})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "killed") && !strings.Contains(out, "已终止") {
		t.Errorf("output %q should mention killed", out)
	}
	<-j.Done()
}

func TestKillShellTool_AlreadyDone(t *testing.T) {
	m := jobs.NewManager()
	defer m.Close()
	ctx := bgCtx(m)

	j := m.Start("bash", "quick", func(ctx context.Context, out io.Writer) (string, error) { return "", nil })
	<-j.Done()

	out, err := NewKillShellTool().Execute(ctx, map[string]any{"job_id": j.ID})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "not running") && !strings.Contains(out, "未在运行") {
		t.Errorf("output %q should mention not running", out)
	}
}

func TestKillShellTool_UnknownID(t *testing.T) {
	m := jobs.NewManager()
	defer m.Close()
	ctx := bgCtx(m)

	out, err := NewKillShellTool().Execute(ctx, map[string]any{"job_id": "nope-999"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "not running") && !strings.Contains(out, "未在运行") {
		t.Errorf("output %q should mention not running", out)
	}
}

func TestKillShellTool_NoManager(t *testing.T) {
	_, err := NewKillShellTool().Execute(context.Background(), map[string]any{"job_id": "bash-1"})
	if err == nil {
		t.Fatal("expected error when no manager")
	}
}

func TestKillShellTool_EmptyJobID(t *testing.T) {
	m := jobs.NewManager()
	defer m.Close()
	ctx := bgCtx(m)

	_, err := NewKillShellTool().Execute(ctx, map[string]any{"job_id": ""})
	if err == nil {
		t.Fatal("expected error for empty job_id")
	}
}

// =====================================================================
// wait
// =====================================================================

func TestWaitTool_Name(t *testing.T) {
	if NewWaitTool().Name() != "wait" {
		t.Error("Name != wait")
	}
}

func TestWaitTool_ReadOnly(t *testing.T) {
	if !NewWaitTool().ReadOnly() {
		t.Error("wait should be ReadOnly")
	}
}

func TestWaitTool_Success(t *testing.T) {
	m := jobs.NewManager()
	defer m.Close()
	ctx := bgCtx(m)

	j1 := m.Start("bash", "a", func(ctx context.Context, out io.Writer) (string, error) {
		_, _ = out.Write([]byte("out1"))
		return "", nil
	})
	j2 := m.Start("task", "b", func(ctx context.Context, out io.Writer) (string, error) {
		return "answer2", nil
	})

	out, err := NewWaitTool().Execute(ctx, map[string]any{
		"job_ids": []string{j1.ID, j2.ID},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, j1.ID) {
		t.Errorf("output %q missing %s", out, j1.ID)
	}
	if !strings.Contains(out, j2.ID) {
		t.Errorf("output %q missing %s", out, j2.ID)
	}
	if !strings.Contains(out, "out1") {
		t.Errorf("output %q missing out1", out)
	}
	if !strings.Contains(out, "answer2") {
		t.Errorf("output %q missing answer2", out)
	}
}

func TestWaitTool_AllRunning(t *testing.T) {
	m := jobs.NewManager()
	defer m.Close()
	ctx := bgCtx(m)

	// 短暂阻塞确保 Wait resolve 时两个 job 都在 running
	m.Start("bash", "a", func(ctx context.Context, out io.Writer) (string, error) {
		time.Sleep(50 * time.Millisecond)
		return "", nil
	})
	m.Start("bash", "b", func(ctx context.Context, out io.Writer) (string, error) {
		time.Sleep(50 * time.Millisecond)
		return "", nil
	})

	out, err := NewWaitTool().Execute(ctx, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Count(out, "done") != 2 {
		t.Errorf("output %q should contain 2 done statuses", out)
	}
}

func TestWaitTool_NoJobs(t *testing.T) {
	m := jobs.NewManager()
	defer m.Close()
	ctx := bgCtx(m)

	out, err := NewWaitTool().Execute(ctx, nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "no") || !strings.Contains(out, "wait") {
		if !strings.Contains(out, "没有可等待") {
			t.Errorf("output %q should mention no jobs to wait", out)
		}
	}
}

func TestWaitTool_Timeout(t *testing.T) {
	m := jobs.NewManager()
	defer m.Close()
	ctx := bgCtx(m)

	m.Start("bash", "block", func(ctx context.Context, out io.Writer) (string, error) {
		time.Sleep(2 * time.Second)
		return "", nil
	})

	out, err := NewWaitTool().Execute(ctx, map[string]any{"timeout_seconds": 1})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "running") {
		t.Errorf("output %q should mention running after timeout", out)
	}
}

func TestWaitTool_NoManager(t *testing.T) {
	_, err := NewWaitTool().Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when no manager")
	}
}

// =====================================================================
// filterLines unit tests
// =====================================================================

func TestFilterLines(t *testing.T) {
	cases := []struct {
		s, re, want string
	}{
		{"a\nb\nc", "a", "a"},
		{"err1\ninfo\nerr2", "err", "err1\nerr2"},
		{"x\ny\nz", "q", ""}, // no match
	}
	for _, c := range cases {
		got, err := filterLines(c.s, c.re)
		if err != nil {
			t.Errorf("filterLines(%q,%q) err: %v", c.s, c.re, err)
			continue
		}
		if got != c.want {
			t.Errorf("filterLines(%q,%q) = %q, want %q", c.s, c.re, got, c.want)
		}
	}
}

func TestFilterLines_InvalidRegex(t *testing.T) {
	_, err := filterLines("x", "(unclosed")
	if err == nil {
		t.Error("expected error for invalid regex")
	}
}

// =====================================================================
// session isolation
// =====================================================================

func TestBashOutputTool_SessionIsolation(t *testing.T) {
	m := jobs.NewManager()
	defer m.Close()

	j := m.StartForSession("sess-A", "bash", "a", func(ctx context.Context, out io.Writer) (string, error) {
		_, _ = out.Write([]byte("from-A"))
		return "", nil
	})
	<-j.Done()

	// sess-A can read own job
	ctxA := jobs.WithManager(jobs.WithSession(context.Background(), "sess-A"), m)
	_, err := NewBashOutputTool().Execute(ctxA, map[string]any{"job_id": j.ID})
	if err != nil {
		t.Errorf("sess-A should read own job: %v", err)
	}

	// sess-B cannot
	ctxB := jobs.WithManager(jobs.WithSession(context.Background(), "sess-B"), m)
	_, err = NewBashOutputTool().Execute(ctxB, map[string]any{"job_id": j.ID})
	if err == nil {
		t.Error("sess-B should NOT read sess-A job")
	}
}

func TestKillShellTool_SessionIsolation(t *testing.T) {
	m := jobs.NewManager()
	defer m.Close()

	j := m.StartForSession("sess-A", "bash", "block", func(ctx context.Context, out io.Writer) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	time.Sleep(20 * time.Millisecond)

	// sess-B cannot kill
	ctxB := jobs.WithManager(jobs.WithSession(context.Background(), "sess-B"), m)
	out, _ := NewKillShellTool().Execute(ctxB, map[string]any{"job_id": j.ID})
	if strings.Contains(out, "killed") || strings.Contains(out, "已终止") {
		t.Error("sess-B should not kill sess-A job")
	}

	// sess-A can kill
	ctxA := jobs.WithManager(jobs.WithSession(context.Background(), "sess-A"), m)
	out, _ = NewKillShellTool().Execute(ctxA, map[string]any{"job_id": j.ID})
	if !strings.Contains(out, "killed") && !strings.Contains(out, "已终止") {
		t.Error("sess-A should kill own job")
	}
	<-j.Done()
}
