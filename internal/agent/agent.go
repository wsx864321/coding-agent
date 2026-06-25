package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/wsx864321/coding-agent/internal/event"
	"github.com/wsx864321/coding-agent/internal/evidence"
	"github.com/wsx864321/coding-agent/internal/jobs"
	"github.com/wsx864321/coding-agent/internal/memory"
	"github.com/wsx864321/coding-agent/internal/permission"
	"github.com/wsx864321/coding-agent/internal/provider"
	"github.com/wsx864321/coding-agent/internal/skill"
	"github.com/wsx864321/coding-agent/internal/tools"
)

// Agent 是 Coding Agent 的主入口，封装 Provider + 工具注册表
type Agent struct {
	cfg  Config
	prov provider.Provider
	registry *tools.Registry
	checker  permission.Checker
	hooks    ToolHooks
	messages []provider.Message
	ledger   *evidence.Ledger
	skillStore *skill.Store
	// --- context compaction knobs ---
	contextWindow       int
	softCompactRatio    float64
	compactRatio        float64
	compactForceRatio   float64
	recentKeep          int
	maxMessagesSnip     int
	archiveDir          string
	consecutiveCompacts int
	compactStuck        bool
	softCompactNoticed  bool
	lastPromptTokens    int
	// --- error recovery ---
	hasAttemptedReactiveCompact bool
	// --- session persistence ---
	sessionDir  string
	sessionPath string
	// --- memory ---
	memSet   *memory.Set
	memQueue *memory.Queue
	// --- background jobs ---
	jobMgr *jobs.Manager
	sink   event.Sink
	// --- pre-compact snapshot (for memory extraction) ---
	preCompactSnapshot []provider.Message
	// --- auto-extract throttling ---
	lastExtractTime  time.Time
	extractInterval  time.Duration
	memExtractThresh int
	extractTurnCount int
}

// NewAgent 构造 Agent
func NewAgent(cfg Config, opts ...Option) (*Agent, error) {
	if err := cfg.resolve(); err != nil {
		return nil, err
	}

	a := &Agent{
		cfg: Config{
			ProviderKind:      cfg.ProviderKind,
			APIKey:            cfg.APIKey,
			BaseURL:           cfg.BaseURL,
			Model:             cfg.Model,
			MaxTokens:         cfg.MaxTokens,
			MaxTurns:          cfg.MaxTurns,
			SystemPrompt:      cfg.SystemPrompt,
			Temperature:       cfg.Temperature,
			ContextWindow:     cfg.ContextWindow,
			SoftCompactRatio:  cfg.SoftCompactRatio,
			CompactRatio:      cfg.CompactRatio,
			CompactForceRatio: cfg.CompactForceRatio,
			RecentKeep:        cfg.RecentKeep,
			MaxMessagesSnip:   cfg.MaxMessagesSnip,
			ArchiveDir:        cfg.ArchiveDir,
			SessionDir:        cfg.SessionDir,
		},
		registry: tools.NewRegistry(),
		ledger:   evidence.NewLedger(),
	}

	for _, opt := range opts {
		if opt != nil {
			opt.apply(a)
		}
	}

	// 如果没有通过 Option 注入 Provider，则自动构建
	if a.prov == nil {
		p, err := provider.New(cfg.ProviderKind, provider.Config{
			Name:    cfg.ProviderKind,
			BaseURL: cfg.BaseURL,
			Model:   cfg.Model,
			APIKey:  cfg.APIKey,
			KeyEnv:  cfg.APIKeyEnv(),
		})
		if err != nil {
			return nil, fmt.Errorf("构建 provider 失败: %w", err)
		}
		a.prov = p
	}

	// SystemPrompt 必须在 registry + skillStore 注入后才能算
	if a.cfg.SystemPrompt == "" {
		var skills []skill.Skill
		if a.skillStore != nil {
			skills = a.skillStore.List()
		}
		a.cfg.SystemPrompt = buildSystemPrompt(a.registry, skills)
	}
	if a.memSet != nil {
		a.cfg.SystemPrompt = memory.Compose(a.cfg.SystemPrompt, a.memSet)
		a.memQueue = memory.NewQueue()
	}
	a.messages = []provider.Message{
		{
			Role:    provider.RoleSystem,
			Content: a.cfg.SystemPrompt,
		},
	}
	a.contextWindow = a.cfg.ContextWindow
	a.softCompactRatio = a.cfg.SoftCompactRatio
	a.compactRatio = a.cfg.CompactRatio
	a.compactForceRatio = a.cfg.CompactForceRatio
	a.recentKeep = a.cfg.RecentKeep
	a.maxMessagesSnip = a.cfg.MaxMessagesSnip
	a.archiveDir = a.cfg.ArchiveDir
	a.sessionDir = a.cfg.SessionDir
	a.extractInterval = DefaultMemoryExtractInterval
	a.memExtractThresh = DefaultMemoryExtractThreshold
	if a.sink == nil {
		a.sink = event.Discard
	}
	return a, nil
}

// Hooks 返回当前注入的 ToolHooks（可能为 nil）
func (a *Agent) Hooks() ToolHooks {
	return a.hooks
}

// Registry 返回底层的工具注册表
func (a *Agent) Registry() *tools.Registry {
	return a.registry
}

// Provider 返回底层的 Provider 实例
func (a *Agent) Provider() provider.Provider {
	return a.prov
}

// WireTaskTool 把 task 工具的 SubagentRunner 连接到当前 Agent 实例。
func (a *Agent) WireTaskTool() {
	t := a.registry.Get("task")
	if t == nil {
		return
	}
	tt, ok := t.(*tools.TaskTool)
	if !ok {
		return
	}
	tt.SetRunner(func(ctx context.Context, prompt string) (string, error) {
		var subHooks ToolHooks
		if a.hooks != nil {
			subHooks = NewSubsetHooks(a.hooks)
		}
		return RunSubAgent(ctx, a, prompt, SubagentOptions{
			Hooks:   subHooks,
			Checker: a.checker,
		})
	})
}

// WireSkillTools 把 run_skill 工具的 SkillRunner 连接到当前 Agent 实例。
func (a *Agent) WireSkillTools() {
	t := a.registry.Get("run_skill")
	if t == nil {
		return
	}
	rst, ok := t.(*skill.RunSkillTool)
	if !ok {
		return
	}
	rst.SetRunner(func(ctx context.Context, sk skill.Skill, task string) (string, error) {
		var subHooks ToolHooks
		if a.hooks != nil {
			subHooks = NewSubsetHooks(a.hooks)
		}
		sysPrompt := sk.Body
		return RunSubAgent(ctx, a, task, SubagentOptions{
			SystemPrompt: sysPrompt,
			Hooks:        subHooks,
			Checker:      a.checker,
		})
	})
}

// WireMemoryTools 把 remember/forget/recall 工具连接到 Agent 的 memory Store/Queue。
func (a *Agent) WireMemoryTools() {
	if a.memSet == nil || a.memSet.Store == nil {
		return
	}
	if t := a.registry.Get("remember"); t != nil {
		if rt, ok := t.(*tools.RememberTool); ok {
			rt.SetStore(a.memSet.Store)
			rt.SetQueue(a.memQueue)
		}
	}
	if t := a.registry.Get("forget"); t != nil {
		if ft, ok := t.(*tools.ForgetTool); ok {
			ft.SetStore(a.memSet.Store)
			ft.SetQueue(a.memQueue)
		}
	}
	if t := a.registry.Get("recall"); t != nil {
		if rct, ok := t.(*tools.RecallTool); ok {
			rct.SetStore(a.memSet.Store)
		}
	}
}

// SkillStore 返回底层的 skill Store
func (a *Agent) SkillStore() *skill.Store {
	return a.skillStore
}

// Run 接收用户输入，驱动 Agent loop，最终返回 LLM 的最终回答
func (a *Agent) Run(ctx context.Context, userInput string) (final string, err error) {
	defer func() {
		a.sink.Emit(event.Event{Kind: event.TurnDone, Err: err})
	}()
	if userInput == "" {
		return "", fmt.Errorf("userInput 不能为空")
	}

	if a.ledger != nil {
		a.ledger.Reset()
		ctx = evidence.WithLedger(ctx, a.ledger)
	}

	// 注入后台任务 Manager 到 context，供 bash/task 的 run_in_background 及
	// bash_output/kill_shell/wait 访问。同时 drain 上一 turn 完成的 job 通知。
	if a.jobMgr != nil {
		ctx = jobs.WithManager(ctx, a.jobMgr)
		if note := a.jobMgr.DrainCompletedNote(); note != "" {
			userInput = note + "\n\n" + userInput
		}
	}

	if a.hooks != nil {
		_ = a.hooks.UserPromptSubmit(ctx, userInput)
	}

	userContent := userInput
	if a.memQueue != nil && a.memQueue.Pending() {
		update := a.memQueue.Flush()
		if update != "" {
			userContent = update + "\n\n" + userInput
		}
	}

	a.messages = append(a.messages, provider.Message{
		Role:    provider.RoleUser,
		Content: userContent,
	})

	// 重置 per-turn 错误恢复状态
	a.hasAttemptedReactiveCompact = false

	for turn := 0; turn < a.cfg.MaxTurns; turn++ {
		final, err := a.loopStep(ctx)
		if err != nil {
			return "", err
		}
		if final != "" {
			_ = a.SaveCurrentSession()
			return final, nil
		}
	}
	return "", fmt.Errorf("%w (limit=%d)", ErrMaxTurnsExceeded, a.cfg.MaxTurns)
}

// Messages 返回当前消息历史（只读拷贝）
func (a *Agent) Messages() []provider.Message {
	out := make([]provider.Message, len(a.messages))
	copy(out, a.messages)
	return out
}

// AppendMessage 追加一条消息到历史（用于 session 恢复）。
func (a *Agent) AppendMessage(m provider.Message) {
	a.messages = append(a.messages, m)
}

// Reset 清空除 system message 外的所有消息历史。
func (a *Agent) Reset() {
	if len(a.messages) == 0 {
		return
	}
	a.messages = a.messages[:1]
	a.consecutiveCompacts = 0
	a.compactStuck = false
	a.softCompactNoticed = false
}

// CompactNow 立即执行一次手动压缩（用于 /compact 或 compact 工具）
func (a *Agent) CompactNow(ctx context.Context, focus string) error {
	_, err := a.compactHistory(ctx, "manual", strings.TrimSpace(focus), true)
	return err
}

// ContextStats 返回当前上下文压缩状态，供 CLI 展示或调试。
func (a *Agent) ContextStats() string {
	if a.contextWindow <= 0 {
		return "compact=已关闭"
	}
	return fmt.Sprintf("窗口=%d 阈值(soft=%.0f%% trigger=%.0f%% force=%.0f%%) stuck=%v",
		a.contextWindow, a.softCompactRatio*100, a.compactRatio*100, a.compactForceRatio*100, a.compactStuck)
}

// ContextSnapshot 返回当前上下文用量快照（已用 tokens，窗口上限）。
func (a *Agent) ContextSnapshot() (used int, window int) {
	return a.lastPromptTokens, a.contextWindow
}

// SetSessionPath 绑定当前 session 的文件路径。
func (a *Agent) SetSessionPath(path string) {
	a.sessionPath = path
}

// SessionPath 返回当前绑定的 session 文件路径。
func (a *Agent) SessionPath() string {
	return a.sessionPath
}

// SaveCurrentSession 将当前消息历史写入 sessionPath。
func (a *Agent) SaveCurrentSession() error {
	if a.sessionPath == "" || a.sessionDir == "" {
		return nil
	}
	hasContent := false
	for _, m := range a.messages {
		if m.Role != provider.RoleSystem {
			hasContent = true
			break
		}
	}
	if !hasContent {
		return nil
	}
	return SaveSession(a.sessionPath, a.messages)
}

// SessionDir 返回 session 持久化根目录。
func (a *Agent) SessionDir() string {
	return a.sessionDir
}

// Balance 查询当前 provider 的余额信息。
// 返回格式化后的余额字符串（如 "¥110.00"），错误时返回空串。
func (a *Agent) Balance(ctx context.Context) (string, error) {
	// 尝试将 provider 断言为支持余额查询的扩展接口。
	type balanceQuerier interface {
		QueryBalance(ctx context.Context) (string, error)
	}
	if bq, ok := a.prov.(balanceQuerier); ok {
		return bq.QueryBalance(ctx)
	}
	return "", nil
}

// MemorySet 返回底层的 memory Set
func (a *Agent) MemorySet() *memory.Set {
	return a.memSet
}

// MemoryQueue 返回记忆变更通知队列
func (a *Agent) MemoryQueue() *memory.Queue {
	return a.memQueue
}

// JobManager 返回后台任务 Manager（可能为 nil）。
func (a *Agent) JobManager() *jobs.Manager {
	return a.jobMgr
}

// PreCompactSnapshot 返回当前压缩前快照
func (a *Agent) PreCompactSnapshot() []provider.Message {
	return a.preCompactSnapshot
}

// SetPreCompactSnapshot 设置压缩前快照
func (a *Agent) SetPreCompactSnapshot(snapshot []provider.Message) {
	a.preCompactSnapshot = snapshot
}
