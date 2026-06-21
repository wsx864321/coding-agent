// Package jobs 实现会话级后台任务管理器，支撑 bash(run_in_background) 和
// task(run_in_background) 以及配套的 bash_output / kill_shell / wait 工具。
//
// 设计要点：
//   - Manager 持有一个 session 级 context（生命周期跨越多个 turn），后台 job
//     在其派生的子 context 上运行，只在 Manager.Close() 或 kill_shell 时取消
//   - 工具通过 call context 访问 Manager（WithManager / FromContext），与
//     evidence.Ledger 的注入模式一致
//   - job 的流式输出写入 per-job buffer，bash_output 用 readOffset 做增量读取
//   - job 完成时把一行摘要入队 completed，controller 在下一 turn 开始时
//     DrainCompletedNote 注入到 user 消息，让模型感知完成
//   - SessionID + *ForSession 方法实现 session 隔离：subagent 启动的 job
//     归属父 session，session 级读取只看到自己的 job
package jobs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// Status 是 job 的生命周期状态
type Status string

const (
	Running Status = "running"
	Done    Status = "done"
	Failed  Status = "failed"
	Killed  Status = "killed"
)

// View 是 job 的只读快照，供状态栏展示
type View struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	Status    string `json:"status"`
	StartedAt int64  `json:"startedAt"` // unix 毫秒
}

// Result 是 Wait 返回的单个 job 终态（或当前态）
type Result struct {
	ID     string
	Kind   string
	Label  string
	Status Status
	Output string // 终态结果文本；若未设置则返回 buffer 内容
}

// maxJobBufferBytes 限制单个 job 的输出 buffer 上限，防止失控命令（如 `yes`）
// 导致 OOM。超出后写入被静默丢弃，kill_shell 仍可终止。
const maxJobBufferBytes = 10 * 1024 * 1024 // 10MB

// Job 是一个后台任务。mu 保护流式 buffer 和终态字段：run goroutine 写，
// Output/Wait 等读者持同一把锁。
type Job struct {
	ID        string
	Kind      string // "bash" | "task"
	Label     string
	SessionID string

	mu         sync.Mutex
	buf        bytes.Buffer
	readOffset int
	status     Status
	result     string
	resultRead bool // result 是否已被 Output 呈现过（task job 不写 buf）
	startedAt  int64
	cancel     context.CancelFunc
	done       chan struct{}
}

// Done 返回 job 完成时关闭的 channel，可用于 select 等待。
func (j *Job) Done() <-chan struct{} {
	return j.done
}

// Status 返回 job 的当前状态（并发安全）。
func (j *Job) Status() Status {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status
}

// Manager 是会话级后台任务注册表，并发安全。
type Manager struct {
	root   context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu        sync.Mutex
	seq       int
	jobs      map[string]*Job
	order     []string
	completed []completion // 已完成 job 的摘要，等待 drain 到下一 turn
}

type completion struct {
	sessionID string
	text      string
}

// NewManager 创建一个 Manager，其 job 运行在全新的 session 级 context 上
// （由 Close 取消）。
func NewManager() *Manager {
	root, cancel := context.WithCancel(context.Background())
	return &Manager{
		root:   root,
		cancel: cancel,
		jobs:   map[string]*Job{},
	}
}

// jobWriter 在 job 锁内追加流式输出，确保并发的 Output 读取不与生产者竞争。
// 超过 maxJobBufferBytes 后丢弃写入，避免 OOM。
type jobWriter struct{ j *Job }

func (w jobWriter) Write(p []byte) (int, error) {
	w.j.mu.Lock()
	defer w.j.mu.Unlock()
	if w.j.buf.Len() >= maxJobBufferBytes {
		return len(p), nil // 静默丢弃，避免命令因写入错误退出
	}
	remaining := maxJobBufferBytes - w.j.buf.Len()
	if len(p) > remaining {
		_, _ = w.j.buf.Write(p[:remaining])
		return len(p), nil
	}
	return w.j.buf.Write(p)
}

// Start 在 goroutine 中以 Manager 的 session context 启动 run，立即返回 job。
// run 把输出流到 writer 并返回终态结果文本（task 的最终回答；bash 把所有
// 输出流到 buffer 并返回 ""）。job 的 context 被取消则标记 killed，其他
// 错误标记 failed，否则 done。
func (m *Manager) Start(kind, label string, run func(ctx context.Context, out io.Writer) (string, error)) *Job {
	return m.StartForSession("", kind, label, run)
}

// StartForSession 启动归属 parentSession 的 job。session 级读者只能看到
// owner 匹配的 job。空 parentSession 保持无 session 行为。
func (m *Manager) StartForSession(parentSession, kind, label string, run func(ctx context.Context, out io.Writer) (string, error)) *Job {
	parentSession = strings.TrimSpace(parentSession)
	m.mu.Lock()
	m.seq++
	id := fmt.Sprintf("%s-%d", kind, m.seq)
	ctx, cancel := context.WithCancel(m.root)
	j := &Job{
		ID:        id,
		Kind:      kind,
		Label:     label,
		SessionID: parentSession,
		status:    Running,
		startedAt: nowMs(),
		cancel:    cancel,
		done:      make(chan struct{}),
	}
	m.jobs[id] = j
	m.order = append(m.order, id)
	m.mu.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		result, err := run(ctx, jobWriter{j})

		var st Status
		switch {
		case ctx.Err() != nil:
			st = Killed
		case err != nil:
			st = Failed
			if result == "" {
				result = err.Error()
			}
		default:
			st = Done
		}
		// 先入队 drain note 再翻转终态。Wait(nil)/resolve 只阻塞 Running job，
		// 若先翻终态再入队，Wait 可能观察到完成却跳过 j.done，导致
		// DrainCompletedNote 抢跑（TestDrainMultiple -race flake）。
		m.recordCompletion(parentSession, id, kind, label, st, err)

		j.mu.Lock()
		j.result = result
		if j.status != Killed { // 并发 Kill 已发布 Killed，保留之
			j.status = st
		}
		j.mu.Unlock()
		close(j.done)
	}()
	return j
}

// recordCompletion 把完成摘要入队 completed，供 DrainCompletedNote 使用。
func (m *Manager) recordCompletion(parentSession, id, kind, label string, st Status, err error) {
	tag := id
	if label != "" {
		tag = fmt.Sprintf("%s (%s)", id, label)
	}
	m.mu.Lock()
	m.completed = append(m.completed, completion{
		sessionID: strings.TrimSpace(parentSession),
		text:      fmt.Sprintf("%s — %s", tag, st),
	})
	m.mu.Unlock()
}

func (m *Manager) get(id string) *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.jobs[id]
}

// Output 返回自上次 Output 调用以来产生的输出及当前状态。ok 为 false 表示 id 未知。
func (m *Manager) Output(id string) (text string, status Status, ok bool) {
	return m.OutputForSession("", id)
}

// OutputForSession 仅当 id 归属 parentSession 时返回输出。空 parentSession
// 保持无 session 行为。
func (m *Manager) OutputForSession(parentSession, id string) (text string, status Status, ok bool) {
	j := m.get(id)
	if j == nil || !sessionMatches(parentSession, j.SessionID) {
		return "", "", false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	full := j.buf.String()
	text = full[j.readOffset:]
	j.readOffset = len(full)
	// task job 不写 buffer，其 answer 在 result；终态后呈现一次
	if text == "" && j.status != Running && j.result != "" && !j.resultRead {
		text = j.result
		j.resultRead = true
	}
	return text, j.status, true
}

// Kill 取消运行中的 job。id 未知或已结束时返回 false。
func (m *Manager) Kill(id string) bool {
	return m.KillForSession("", id)
}

// KillForSession 仅当 job 归属 parentSession 时取消它。空 parentSession
// 保持无 session 行为。
func (m *Manager) KillForSession(parentSession, id string) bool {
	j := m.get(id)
	if j == nil || !sessionMatches(parentSession, j.SessionID) {
		return false
	}
	j.mu.Lock()
	running := j.status == Running
	if running {
		// 同步翻成 Killed，让 Output/Wait 立刻反映 kill，而非等 run goroutine
		// 的 cmd.Run 返回（落后 WaitDelay）。goroutine 返回后仍会 set Killed +
		// record completion；此分支只在确实 Running 时触发，刚结束的 job 保留
		// 真实终态。
		j.status = Killed
	}
	j.mu.Unlock()
	if !running {
		return false
	}
	j.cancel()
	return true
}

// Wait 阻塞直到指定的 job（或所有运行中 job，当 ids 为空）到达终态，或 ctx
// 取消，或 timeoutSec 到期（0=不超时）。无论因何返回都给出每个目标的快照，
// 超时也能报告部分进度。
func (m *Manager) Wait(ctx context.Context, ids []string, timeoutSec int) []Result {
	return m.WaitForSession(ctx, "", ids, timeoutSec)
}

// WaitForSession 只等待归属 parentSession 的 job。空 parentSession 保持无 session 行为。
func (m *Manager) WaitForSession(ctx context.Context, parentSession string, ids []string, timeoutSec int) []Result {
	targets := m.resolve(parentSession, ids)
	if len(targets) == 0 {
		return nil
	}
	var timeout <-chan time.Time
	if timeoutSec > 0 {
		t := time.NewTimer(time.Duration(timeoutSec) * time.Second)
		defer t.Stop()
		timeout = t.C
	}
	for _, j := range targets {
		select {
		case <-j.done:
		case <-ctx.Done():
			return m.results(targets)
		case <-timeout:
			return m.results(targets)
		}
	}
	return m.results(targets)
}

// resolve 把请求的 id 映射到 job；空列表选择所有运行中的 job。
func (m *Manager) resolve(parentSession string, ids []string) []*Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Job
	if len(ids) == 0 {
		for _, id := range m.order {
			j := m.jobs[id]
			if !sessionMatches(parentSession, j.SessionID) {
				continue
			}
			j.mu.Lock()
			running := j.status == Running
			j.mu.Unlock()
			if running {
				out = append(out, j)
			}
		}
		return out
	}
	for _, id := range ids {
		if j := m.jobs[id]; j != nil && sessionMatches(parentSession, j.SessionID) {
			out = append(out, j)
		}
	}
	return out
}

func (m *Manager) results(targets []*Job) []Result {
	out := make([]Result, 0, len(targets))
	for _, j := range targets {
		j.mu.Lock()
		text := j.result
		if text == "" {
			text = j.buf.String()
		}
		out = append(out, Result{ID: j.ID, Kind: j.Kind, Label: j.Label, Status: j.status, Output: text})
		j.mu.Unlock()
	}
	return out
}

// Running 返回仍在运行的 job 快照（供状态栏）。
func (m *Manager) Running() []View {
	return m.RunningForSession("")
}

// RunningForSession 返回归属 parentSession 的运行中 job。空 parentSession 保持无 session 行为。
func (m *Manager) RunningForSession(parentSession string) []View {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []View
	for _, id := range m.order {
		j := m.jobs[id]
		if !sessionMatches(parentSession, j.SessionID) {
			continue
		}
		j.mu.Lock()
		if j.status == Running {
			out = append(out, View{ID: j.ID, Kind: j.Kind, Label: j.Label, Status: string(j.status), StartedAt: j.startedAt})
		}
		j.mu.Unlock()
	}
	return out
}

// DrainCompletedNote 返回（并清除）自上次 drain 以来完成的 job 的一行摘要，
// 供 controller 折入下一 turn，让模型感知完成。无完成时返回 ""。
func (m *Manager) DrainCompletedNote() string {
	return m.DrainCompletedNoteForSession("")
}

// DrainCompletedNoteForSession 只 drain parentSession 的完成摘要。其他 session
// 的摘要留在队列里等该 session 再次激活。空 parentSession 保持无 session 行为。
func (m *Manager) DrainCompletedNoteForSession(parentSession string) string {
	m.mu.Lock()
	var c []string
	if strings.TrimSpace(parentSession) == "" {
		for _, item := range m.completed {
			c = append(c, item.text)
		}
		m.completed = nil
	} else {
		remaining := m.completed[:0]
		for _, item := range m.completed {
			if item.sessionID == parentSession {
				c = append(c, item.text)
			} else {
				remaining = append(remaining, item)
			}
		}
		m.completed = remaining
	}
	m.mu.Unlock()
	if len(c) == 0 {
		return ""
	}
	return "后台任务已完成（自上一条消息以来）: " + strings.Join(c, "; ") +
		"。可用 bash_output 读取输出，或 wait 等待其他任务。"
}

// Close 取消 session context 并等待所有后台 job goroutine 返回。
// job 通过 run context 观察到取消（exec.CommandContext 会 kill bash job 的进程），
// 所以等待是有界的。这对调用方清理 t.TempDir 很重要：不等待的话 RemoveAll 可能
// 与仍在持有该目录下文件的 job goroutine 竞争。
func (m *Manager) Close() {
	m.cancel()
	m.wg.Wait()
}

func nowMs() int64 { return time.Now().UnixMilli() }

func sessionMatches(filter, jobSession string) bool {
	filter = strings.TrimSpace(filter)
	return filter == "" || strings.TrimSpace(jobSession) == filter
}

// --- call-context 注入（与 evidence.Ledger 模式一致）---

type ctxKey struct{}
type sessionCtxKey struct{}

// WithManager 把 Manager 印入 ctx，工具可通过 FromContext 取得。
// agent 在每次工具调用的 context 上设置。
func WithManager(ctx context.Context, m *Manager) context.Context {
	return context.WithValue(ctx, ctxKey{}, m)
}

// FromContext 返回 agent 设置的 job manager。普通 context（无 manager 的
// headless 测试、run loop 外的调用）返回 ok=false。
func FromContext(ctx context.Context) (*Manager, bool) {
	m, ok := ctx.Value(ctxKey{}).(*Manager)
	return m, ok && m != nil
}

// WithSession 把当前父 session ID 印入 ctx，用于 session 级 job 操作。
func WithSession(ctx context.Context, parentSession string) context.Context {
	return context.WithValue(ctx, sessionCtxKey{}, strings.TrimSpace(parentSession))
}

// SessionFromContext 返回当前父 session ID（job 归属与过滤用）。空表示无 session 作用域。
func SessionFromContext(ctx context.Context) string {
	session, _ := ctx.Value(sessionCtxKey{}).(string)
	return strings.TrimSpace(session)
}
