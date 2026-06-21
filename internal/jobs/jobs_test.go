package jobs

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =====================================================================
// NewManager / 基础结构
// =====================================================================

func TestNewManager(t *testing.T) {
	m := NewManager()
	defer m.Close()
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if len(m.jobs) != 0 {
		t.Errorf("new manager has %d jobs, want 0", len(m.jobs))
	}
	if m.Running() != nil {
		t.Errorf("Running() = %v, want nil", m.Running())
	}
	if note := m.DrainCompletedNote(); note != "" {
		t.Errorf("DrainCompletedNote = %q, want empty", note)
	}
}

// =====================================================================
// Start / 完成
// =====================================================================

func TestStart_Done(t *testing.T) {
	m := NewManager()
	defer m.Close()

	j := m.Start("bash", "echo hello", func(ctx context.Context, out io.Writer) (string, error) {
		_, _ = out.Write([]byte("hello\n"))
		return "", nil
	})
	if j == nil {
		t.Fatal("Start returned nil job")
	}
	if j.ID == "" {
		t.Error("job ID is empty")
	}
	if j.Kind != "bash" {
		t.Errorf("Kind = %q, want bash", j.Kind)
	}
	if j.Label != "echo hello" {
		t.Errorf("Label = %q, want 'echo hello'", j.Label)
	}
	if j.SessionID != "" {
		t.Errorf("SessionID = %q, want empty", j.SessionID)
	}

	<-j.done
	if j.status != Done {
		t.Errorf("status = %q, want done", j.status)
	}
}

func TestStart_Failed(t *testing.T) {
	m := NewManager()
	defer m.Close()

	j := m.Start("bash", "fail", func(ctx context.Context, out io.Writer) (string, error) {
		return "", errors.New("boom")
	})
	<-j.done
	if j.status != Failed {
		t.Errorf("status = %q, want failed", j.status)
	}
	if j.result != "boom" {
		t.Errorf("result = %q, want boom", j.result)
	}
}

func TestStart_FailedWithResult(t *testing.T) {
	m := NewManager()
	defer m.Close()

	j := m.Start("task", "partial", func(ctx context.Context, out io.Writer) (string, error) {
		return "partial answer", errors.New("incomplete")
	})
	<-j.done
	if j.status != Failed {
		t.Errorf("status = %q, want failed", j.status)
	}
	// 有 result 时保留 result，不覆盖成 err
	if j.result != "partial answer" {
		t.Errorf("result = %q, want 'partial answer'", j.result)
	}
}

// =====================================================================
// ID 递增
// =====================================================================

func TestStart_IDIncrement(t *testing.T) {
	m := NewManager()
	defer m.Close()

	j1 := m.Start("bash", "a", func(ctx context.Context, out io.Writer) (string, error) { return "", nil })
	j2 := m.Start("bash", "b", func(ctx context.Context, out io.Writer) (string, error) { return "", nil })
	j3 := m.Start("task", "c", func(ctx context.Context, out io.Writer) (string, error) { return "r", nil })

	<-j1.done
	<-j2.done
	<-j3.done

	if j1.ID == j2.ID {
		t.Errorf("j1 and j2 share ID %q", j1.ID)
	}
	if j2.ID == j3.ID {
		t.Errorf("j2 and j3 share ID %q", j2.ID)
	}
	// ID 前缀应匹配 kind
	if !strings.HasPrefix(j1.ID, "bash-") {
		t.Errorf("j1.ID = %q, want prefix 'bash-'", j1.ID)
	}
	if !strings.HasPrefix(j3.ID, "task-") {
		t.Errorf("j3.ID = %q, want prefix 'task-'", j3.ID)
	}
}

// =====================================================================
// Output 增量读取
// =====================================================================

func TestOutput_Incremental(t *testing.T) {
	m := NewManager()
	defer m.Close()

	j := m.Start("bash", "stream", func(ctx context.Context, out io.Writer) (string, error) {
		_, _ = out.Write([]byte("part1\n"))
		time.Sleep(50 * time.Millisecond)
		_, _ = out.Write([]byte("part2\n"))
		return "", nil
	})

	// 轮询直到拿到非空输出（part1 已写），避免与 goroutine 调度竞态
	var text1 string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		t1, _, ok := m.Output(j.ID)
		if ok && t1 != "" {
			text1 = t1
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if text1 == "" {
		t.Fatal("first Output returned empty text before deadline")
	}

	<-j.done

	// 第二次读：拿到自上次以来的增量
	text2, status2, _ := m.Output(j.ID)
	if status2 != Done {
		t.Errorf("status2 = %q, want done", status2)
	}
	// text2 + text1 应覆盖全部输出
	combined := text1 + text2
	if !strings.Contains(combined, "part1") {
		t.Errorf("combined %q missing part1", combined)
	}
	if !strings.Contains(combined, "part2") {
		t.Errorf("combined %q missing part2", combined)
	}

	// 第三次读：无新输出
	text3, _, _ := m.Output(j.ID)
	if text3 != "" {
		t.Errorf("third Output = %q, want empty", text3)
	}
}

func TestOutput_UnknownID(t *testing.T) {
	m := NewManager()
	defer m.Close()

	_, _, ok := m.Output("nope-999")
	if ok {
		t.Error("Output unknown id returned ok=true")
	}
}

func TestOutput_TaskResultSurfacedOnce(t *testing.T) {
	m := NewManager()
	defer m.Close()

	j := m.Start("task", "answer", func(ctx context.Context, out io.Writer) (string, error) {
		return "the answer", nil
	})
	<-j.done

	// task job 不写 buffer，终态后首次 Output 应呈现 result
	text1, status, ok := m.Output(j.ID)
	if !ok {
		t.Fatal("ok=false")
	}
	if status != Done {
		t.Errorf("status = %q, want done", status)
	}
	if text1 != "the answer" {
		t.Errorf("first Output = %q, want 'the answer'", text1)
	}

	// 第二次不应重复呈现
	text2, _, _ := m.Output(j.ID)
	if text2 != "" {
		t.Errorf("second Output = %q, want empty (already read)", text2)
	}
}

// =====================================================================
// Kill
// =====================================================================

func TestKill_Running(t *testing.T) {
	m := NewManager()
	defer m.Close()

	killed := make(chan struct{})
	j := m.Start("bash", "sleep", func(ctx context.Context, out io.Writer) (string, error) {
		<-ctx.Done()
		close(killed)
		return "", ctx.Err()
	})

	// 确保进入 running
	time.Sleep(20 * time.Millisecond)

	if !m.Kill(j.ID) {
		t.Fatal("Kill returned false for running job")
	}
	<-j.done
	if j.status != Killed {
		t.Errorf("status = %q, want killed", j.status)
	}
	<-killed
}

func TestKill_AlreadyDone(t *testing.T) {
	m := NewManager()
	defer m.Close()

	j := m.Start("bash", "quick", func(ctx context.Context, out io.Writer) (string, error) {
		return "", nil
	})
	<-j.done

	if m.Kill(j.ID) {
		t.Error("Kill returned true for already-done job")
	}
	if j.status != Done {
		t.Errorf("status = %q, want done", j.status)
	}
}

func TestKill_UnknownID(t *testing.T) {
	m := NewManager()
	defer m.Close()
	if m.Kill("nope-999") {
		t.Error("Kill unknown id returned true")
	}
}

// =====================================================================
// Wait
// =====================================================================

func TestWait_SpecificIDs(t *testing.T) {
	m := NewManager()
	defer m.Close()

	j1 := m.Start("bash", "a", func(ctx context.Context, out io.Writer) (string, error) {
		_, _ = out.Write([]byte("out1"))
		return "", nil
	})
	j2 := m.Start("bash", "b", func(ctx context.Context, out io.Writer) (string, error) {
		time.Sleep(20 * time.Millisecond)
		_, _ = out.Write([]byte("out2"))
		return "", nil
	})

	results := m.Wait(context.Background(), []string{j1.ID, j2.ID}, 0)
	if len(results) != 2 {
		t.Fatalf("Wait returned %d results, want 2", len(results))
	}
	for _, r := range results {
		if r.Status != Done {
			t.Errorf("result %s status = %q, want done", r.ID, r.Status)
		}
	}
}

func TestWait_AllRunning(t *testing.T) {
	m := NewManager()
	defer m.Close()

	// 短暂阻塞确保 Wait resolve 时两个 job 都在 running
	m.Start("bash", "a", func(ctx context.Context, out io.Writer) (string, error) {
		time.Sleep(50 * time.Millisecond)
		return "", nil
	})
	m.Start("bash", "b", func(ctx context.Context, out io.Writer) (string, error) {
		time.Sleep(50 * time.Millisecond)
		return "", nil
	})

	results := m.Wait(context.Background(), nil, 0)
	if len(results) != 2 {
		t.Fatalf("Wait all returned %d results, want 2", len(results))
	}
}

func TestWait_Timeout(t *testing.T) {
	m := NewManager()
	defer m.Close()

	j := m.Start("bash", "sleep", func(ctx context.Context, out io.Writer) (string, error) {
		time.Sleep(2 * time.Second)
		return "", nil
	})

	start := time.Now()
	results := m.Wait(context.Background(), []string{j.ID}, 1) // 1s 超时
	elapsed := time.Since(start)

	if len(results) != 1 {
		t.Fatalf("Wait returned %d results, want 1", len(results))
	}
	// 超时时 job 仍在 running
	if results[0].Status != Running {
		t.Errorf("status = %q, want running (timed out)", results[0].Status)
	}
	if elapsed >= 2*time.Second {
		t.Errorf("Wait took %v, should have timed out ~1s", elapsed)
	}
}

func TestWait_Empty(t *testing.T) {
	m := NewManager()
	defer m.Close()
	results := m.Wait(context.Background(), nil, 0)
	if results != nil {
		t.Errorf("Wait with no jobs = %v, want nil", results)
	}
}

// =====================================================================
// Running 快照
// =====================================================================

func TestRunning(t *testing.T) {
	m := NewManager()
	defer m.Close()

	started := make(chan struct{})
	j := m.Start("bash", "block", func(ctx context.Context, out io.Writer) (string, error) {
		close(started)
		time.Sleep(100 * time.Millisecond)
		return "", nil
	})
	<-started

	views := m.Running()
	if len(views) != 1 {
		t.Fatalf("Running = %d views, want 1", len(views))
	}
	if views[0].ID != j.ID {
		t.Errorf("view ID = %q, want %q", views[0].ID, j.ID)
	}
	if views[0].Status != "running" {
		t.Errorf("view Status = %q, want running", views[0].Status)
	}

	<-j.done
	if len(m.Running()) != 0 {
		t.Errorf("Running after done = %d views, want 0", len(m.Running()))
	}
}

// =====================================================================
// DrainCompletedNote
// =====================================================================

func TestDrainCompletedNote(t *testing.T) {
	m := NewManager()
	defer m.Close()

	// 短暂阻塞确保 Wait resolve 时 job 在 running，从而 Wait 等到 j.done
	// （j.done 在 recordCompletion 之后才 close）
	m.Start("bash", "a", func(ctx context.Context, out io.Writer) (string, error) {
		time.Sleep(50 * time.Millisecond)
		return "", nil
	})
	m.Start("bash", "b", func(ctx context.Context, out io.Writer) (string, error) {
		time.Sleep(50 * time.Millisecond)
		return "", nil
	})

	// 等待完成（通过 Wait all）
	_ = m.Wait(context.Background(), nil, 0)

	note := m.DrainCompletedNote()
	if note == "" {
		t.Fatal("DrainCompletedNote empty after completions")
	}
	if !strings.Contains(note, "bash-1") {
		t.Errorf("note %q missing bash-1", note)
	}
	if !strings.Contains(note, "bash-2") {
		t.Errorf("note %q missing bash-2", note)
	}

	// 再次 drain 应为空
	if note2 := m.DrainCompletedNote(); note2 != "" {
		t.Errorf("second DrainCompletedNote = %q, want empty", note2)
	}
}

func TestDrainCompletedNote_Empty(t *testing.T) {
	m := NewManager()
	defer m.Close()
	if note := m.DrainCompletedNote(); note != "" {
		t.Errorf("DrainCompletedNote = %q, want empty", note)
	}
}

// =====================================================================
// Close 取消所有 job
// =====================================================================

func TestClose_CancelsJobs(t *testing.T) {
	m := NewManager()

	cancelled := atomic.Int32{}
	j := m.Start("bash", "block", func(ctx context.Context, out io.Writer) (string, error) {
		<-ctx.Done()
		cancelled.Add(1)
		return "", ctx.Err()
	})

	m.Close() // 应取消并等待 goroutine 退出

	if cancelled.Load() != 1 {
		t.Error("job context not cancelled by Close")
	}
	if j.status != Killed {
		t.Errorf("status = %q, want killed", j.status)
	}
}

// =====================================================================
// session 隔离
// =====================================================================

func TestStartForSession(t *testing.T) {
	m := NewManager()
	defer m.Close()

	j := m.StartForSession("sess-A", "bash", "task", func(ctx context.Context, out io.Writer) (string, error) {
		return "", nil
	})
	<-j.done
	if j.SessionID != "sess-A" {
		t.Errorf("SessionID = %q, want sess-A", j.SessionID)
	}
}

func TestOutputForSession_Isolation(t *testing.T) {
	m := NewManager()
	defer m.Close()

	j := m.StartForSession("sess-A", "bash", "a", func(ctx context.Context, out io.Writer) (string, error) {
		_, _ = out.Write([]byte("from-A"))
		return "", nil
	})
	<-j.done

	// sess-A 能看到
	_, _, ok := m.OutputForSession("sess-A", j.ID)
	if !ok {
		t.Error("OutputForSession(sess-A) returned ok=false")
	}
	// sess-B 看不到
	_, _, okB := m.OutputForSession("sess-B", j.ID)
	if okB {
		t.Error("OutputForSession(sess-B) returned ok=true for sess-A job")
	}
	// 空 session（全局）能看到
	_, _, okGlobal := m.OutputForSession("", j.ID)
	if !okGlobal {
		t.Error("OutputForSession(empty) returned ok=false")
	}
}

func TestKillForSession_Isolation(t *testing.T) {
	m := NewManager()
	defer m.Close()

	j := m.StartForSession("sess-A", "bash", "block", func(ctx context.Context, out io.Writer) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	time.Sleep(20 * time.Millisecond)

	// sess-B 杀不了 sess-A 的 job
	if m.KillForSession("sess-B", j.ID) {
		t.Error("KillForSession(sess-B) returned true for sess-A job")
	}
	// sess-A 能杀
	if !m.KillForSession("sess-A", j.ID) {
		t.Error("KillForSession(sess-A) returned false")
	}
	<-j.done
}

func TestWaitForSession_Isolation(t *testing.T) {
	m := NewManager()
	defer m.Close()

	jA := m.StartForSession("sess-A", "bash", "a", func(ctx context.Context, out io.Writer) (string, error) {
		return "A", nil
	})
	jB := m.StartForSession("sess-B", "bash", "b", func(ctx context.Context, out io.Writer) (string, error) {
		return "B", nil
	})
	<-jA.done
	<-jB.done

	// sess-A 只等 jA
	resultsA := m.WaitForSession(context.Background(), "sess-A", []string{jA.ID, jB.ID}, 0)
	if len(resultsA) != 1 {
		t.Errorf("sess-A got %d results, want 1 (only own job)", len(resultsA))
	}
}

func TestDrainCompletedNoteForSession(t *testing.T) {
	m := NewManager()
	defer m.Close()

	m.StartForSession("sess-A", "bash", "a", func(ctx context.Context, out io.Writer) (string, error) { return "", nil })
	m.StartForSession("sess-B", "bash", "b", func(ctx context.Context, out io.Writer) (string, error) { return "", nil })
	_ = m.Wait(context.Background(), nil, 0)

	// drain sess-A 只拿到 sess-A 的
	noteA := m.DrainCompletedNoteForSession("sess-A")
	if !strings.Contains(noteA, "bash-1") {
		t.Errorf("noteA %q missing bash-1", noteA)
	}
	if strings.Contains(noteA, "bash-2") {
		t.Errorf("noteA %q should not contain bash-2", noteA)
	}

	// sess-B 的还在队列
	noteB := m.DrainCompletedNoteForSession("sess-B")
	if !strings.Contains(noteB, "bash-2") {
		t.Errorf("noteB %q missing bash-2", noteB)
	}
}

func TestRunningForSession(t *testing.T) {
	m := NewManager()
	defer m.Close()

	started := make(chan struct{})
	m.StartForSession("sess-A", "bash", "a", func(ctx context.Context, out io.Writer) (string, error) {
		close(started)
		time.Sleep(100 * time.Millisecond)
		return "", nil
	})
	m.StartForSession("sess-B", "bash", "b", func(ctx context.Context, out io.Writer) (string, error) {
		time.Sleep(100 * time.Millisecond)
		return "", nil
	})
	<-started

	if len(m.RunningForSession("sess-A")) != 1 {
		t.Error("sess-A should see 1 running job")
	}
	if len(m.RunningForSession("sess-B")) != 1 {
		t.Error("sess-B should see 1 running job")
	}
	if len(m.RunningForSession("")) != 2 {
		t.Error("global should see 2 running jobs")
	}
}

// =====================================================================
// jobWriter 缓冲上限
// =====================================================================

func TestJobWriter_BufferCap(t *testing.T) {
	m := NewManager()
	defer m.Close()

	j := m.Start("bash", "flood", func(ctx context.Context, out io.Writer) (string, error) {
		// 写入远超上限的数据
		chunk := strings.Repeat("x", 1024)
		for i := 0; i < 20*1024; i++ { // 20MB worth
			_, _ = out.Write([]byte(chunk))
		}
		return "", nil
	})
	<-j.done

	j.mu.Lock()
	bufLen := j.buf.Len()
	j.mu.Unlock()

	if bufLen > maxJobBufferBytes {
		t.Errorf("buffer len = %d, exceeds cap %d", bufLen, maxJobBufferBytes)
	}
}

// =====================================================================
// context 注入
// =====================================================================

func TestWithManager_FromContext(t *testing.T) {
	m := NewManager()
	defer m.Close()

	ctx := context.Background()
	_, ok := FromContext(ctx)
	if ok {
		t.Error("FromContext plain ctx returned ok=true")
	}

	ctx2 := WithManager(ctx, m)
	got, ok := FromContext(ctx2)
	if !ok {
		t.Fatal("FromContext after WithManager returned ok=false")
	}
	if got != m {
		t.Error("retrieved manager != injected")
	}
}

func TestWithSession_SessionFromContext(t *testing.T) {
	ctx := context.Background()
	if s := SessionFromContext(ctx); s != "" {
		t.Errorf("plain ctx session = %q, want empty", s)
	}

	ctx2 := WithSession(ctx, "  sess-X  ")
	if s := SessionFromContext(ctx2); s != "sess-X" {
		t.Errorf("session = %q, want sess-X (trimmed)", s)
	}
}

// =====================================================================
// 并发安全
// =====================================================================

func TestConcurrent_StartOutput(t *testing.T) {
	m := NewManager()
	defer m.Close()

	const n = 20
	var wg sync.WaitGroup
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			j := m.Start("bash", "c", func(ctx context.Context, out io.Writer) (string, error) {
				_, _ = out.Write([]byte("x"))
				return "", nil
			})
			ids[i] = j.ID
		}(i)
	}
	wg.Wait()

	// 并发 Output
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, _, _ = m.Output(id)
		}(id)
	}
	wg.Wait()

	// 全部应能 drain
	_ = m.Wait(context.Background(), nil, 0)
	if note := m.DrainCompletedNote(); note == "" {
		t.Error("DrainCompletedNote empty after concurrent jobs")
	}
}

// =====================================================================
// recordCompletion 在 Kill 后仍记录
// =====================================================================

func TestKill_RecordsCompletion(t *testing.T) {
	m := NewManager()
	defer m.Close()

	j := m.Start("bash", "block", func(ctx context.Context, out io.Writer) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	time.Sleep(20 * time.Millisecond)
	m.Kill(j.ID)
	<-j.done

	note := m.DrainCompletedNote()
	if !strings.Contains(note, j.ID) {
		t.Errorf("note %q missing killed job %q", note, j.ID)
	}
	if !strings.Contains(note, "killed") {
		t.Errorf("note %q missing 'killed' status", note)
	}
}
