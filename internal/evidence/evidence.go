// Package evidence 实现证据账本（Evidence Ledger），为 todo_write / complete_step 工具提供
// 工具调用凭证记录和完成状态验证。
//
// 设计参考 DeepSeek-Reasonix 的 evidence 包，核心思路：
//   - 每轮用户输入（Run 调用）开始时重置 receipts 和 completedSteps
//   - currentTodos 跨轮保留（用户多轮对话中 todo 列表持续有效）
//   - todo_write 标记 completed 前必须有对应的 complete_step 凭证
//   - complete_step 的证据可追溯本轮内的 bash / write_file / edit_file 等工具调用
package evidence

import (
	"context"
	"sync"
)

type ctxKey struct{}

// Receipt 记录单次工具调用的凭证
type Receipt struct {
	ToolName string
	Args     map[string]any
	Output   string
	Success  bool
}

// TodoItem 是 todo 列表中的单个条目
type TodoItem struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"activeForm,omitempty"`
}

// Ledger 是每个 Agent 实例持有的证据账本
//
// 生命周期：
//   - 创建于 NewAgent，跟随 Agent 实例存活
//   - 每次 Run() 开始时调用 Reset()，清除 per-turn 状态
//   - currentTodos 跨 Run() 保留，直到被新的 SetTodos 覆盖
type Ledger struct {
	mu sync.Mutex

	// per-turn 状态（每次 Reset 清空）
	receipts       []Receipt
	completedSteps map[string]bool // step content → 已签收
	guardBlocks    int             // 终答守卫本轮阻断次数

	// cross-turn 状态（Reset 不清空）
	currentTodos  []TodoItem
	previousTodos []TodoItem
}

// NewLedger 创建一个空的 Ledger
func NewLedger() *Ledger {
	return &Ledger{
		completedSteps: make(map[string]bool),
	}
}

// WithLedger 把 Ledger 注入 context
func WithLedger(ctx context.Context, l *Ledger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext 从 context 中取出 Ledger
func FromContext(ctx context.Context) (*Ledger, bool) {
	l, ok := ctx.Value(ctxKey{}).(*Ledger)
	return l, ok
}

// Reset 清除 per-turn 状态（receipts、completedSteps、guardBlocks），保留 currentTodos
//
// 应在每次 agent.Run() 开始时调用
func (l *Ledger) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.receipts = l.receipts[:0]
	l.completedSteps = make(map[string]bool)
	l.guardBlocks = 0
}

// Record 记录一次工具调用凭证
func (l *Ledger) Record(name string, args map[string]any, output string, success bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.receipts = append(l.receipts, Receipt{
		ToolName: name,
		Args:     args,
		Output:   output,
		Success:  success,
	})
}

// SetTodos 更新当前 todo 快照（仅由成功的 todo_write 调用）
func (l *Ledger) SetTodos(todos []TodoItem) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.previousTodos = l.currentTodos
	cp := make([]TodoItem, len(todos))
	copy(cp, todos)
	l.currentTodos = cp
}

// CurrentTodos 返回当前 todo 列表的只读拷贝
func (l *Ledger) CurrentTodos() []TodoItem {
	l.mu.Lock()
	defer l.mu.Unlock()
	cp := make([]TodoItem, len(l.currentTodos))
	copy(cp, l.currentTodos)
	return cp
}

// MarkStepCompleted 标记某个 step 已通过 complete_step 签收
func (l *Ledger) MarkStepCompleted(step string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.completedSteps[step] = true
}

// IsStepCompleted 检查某个 step 是否已签收
func (l *Ledger) IsStepCompleted(step string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.completedSteps[step]
}

// UnverifiedCompletedTodos 检查 newTodos 中哪些条目被新标记为 completed
// 但缺少 complete_step 凭证。
//
// 返回值：
//   - missing：缺少凭证的 step content 列表
//   - hasBaseline：是否有上一次的 todo 快照用于比较
//
// 若无基线（首次 todo_write），返回 nil, false —— 不做约束
func (l *Ledger) UnverifiedCompletedTodos(newTodos []TodoItem) (missing []string, hasBaseline bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.currentTodos) == 0 {
		return nil, false
	}

	prevCompleted := make(map[string]bool, len(l.currentTodos))
	for _, t := range l.currentTodos {
		if t.Status == "completed" {
			prevCompleted[t.Content] = true
		}
	}

	for _, t := range newTodos {
		if t.Status == "completed" && !prevCompleted[t.Content] {
			if !l.completedSteps[t.Content] {
				missing = append(missing, t.Content)
			}
		}
	}
	return missing, true
}

// HasSuccessfulReceipt 检查本轮是否存在指定工具的成功调用凭证
func (l *Ledger) HasSuccessfulReceipt(toolName string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range l.receipts {
		if r.ToolName == toolName && r.Success {
			return true
		}
	}
	return false
}

// HasAnyWriteReceipt 检查本轮是否存在任何写操作的成功凭证（write_file 或 edit_file）
func (l *Ledger) HasAnyWriteReceipt() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range l.receipts {
		if r.Success && (r.ToolName == "write_file" || r.ToolName == "edit_file") {
			return true
		}
	}
	return false
}

// IncrementGuardBlock 递增终答守卫阻断计数，返回阻断后的计数值
func (l *Ledger) IncrementGuardBlock() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.guardBlocks++
	return l.guardBlocks
}

// GuardBlocks 返回当前终答守卫阻断次数
func (l *Ledger) GuardBlocks() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.guardBlocks
}
