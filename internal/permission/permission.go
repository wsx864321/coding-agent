// Package permission 实现 coding-agent 的"执行前权限判断"管线。
//
//	+-------+    +----------+    +----------+    +-----+    +------+
//	| Tool  | -> | Checker1 | -> | Checker2 | -> | ... | -> | Exec |
//	| call  |    |  (Deny?) |    |  (Deny?) |    |     |    |      |
//	+-------+    +----------+    +----------+    +-----+    +------+
//	                  |              |
//	                  +-> Deny ───────┘（首个返回 Deny 的 Checker 短路；后续不再执行）
//	                  +-> Allow -> 继续下一个 Checker
//	                            全部 Allow -> 进入 Exec
//
// 设计目标：
//   - 与 tools 包解耦：permission 不知道 tool 的存在，只看 (name, args)
//   - 单一调用点：Agent.invokeTool 之前调一次 Pipeline.Check
//   - 可扩展：Checker / Asker 都是接口，用户可注入自定义实现
package permission

import "context"

// Decision 是权限检查的最终裁决
type Decision int

const (
	// DecisionAllow 放行，调用方可以执行工具
	DecisionAllow Decision = iota
	// DecisionDeny  拒绝，调用方应中止本次工具调用
	DecisionDeny
)

// String 返回 Decision 的可读名称
func (d Decision) String() string {
	switch d {
	case DecisionAllow:
		return "allow"
	case DecisionDeny:
		return "deny"
	}
	return "unknown"
}

// CheckResult 把 Decision 与人类可读的 Reason 绑在一起返回
//
// Reason 既会回填到 tool_result 喂给 LLM，也会写到日志 / UI。
type CheckResult struct {
	Decision Decision
	Reason   string
}

// Checker 决定一次工具调用是否应被放行
//
// 语义约定：
//   - DecisionAllow → 放行
//   - DecisionDeny  → 阻断（Pipeline 会短路返回）
type Checker interface {
	Check(ctx context.Context, name string, args map[string]any) CheckResult
}

// Asker 把"需要用户确认"转成 Allow / Deny
//
// 实现可以是命令行交互、Web 弹窗、自动审批器等。
// 返回 true 表示放行，false 表示拒绝。
type Asker interface {
	Ask(ctx context.Context, name string, args map[string]any, reason string) bool
}

// AskerFunc 把函数适配为 Asker
type AskerFunc func(ctx context.Context, name string, args map[string]any, reason string) bool

// Ask 实现 Asker 接口
func (f AskerFunc) Ask(ctx context.Context, name string, args map[string]any, reason string) bool {
	return f(ctx, name, args, reason)
}

// Pipeline 把多个 Checker 串成一个 Checker
//
// 流程：依次执行 Deny 中的 Checker；第一个返回 Deny 的短路整个流程；都没命中 → Allow
type Pipeline struct {
	Deny []Checker
}

// Check 实现 Checker 接口
func (p *Pipeline) Check(ctx context.Context, name string, args map[string]any) CheckResult {
	for _, c := range p.Deny {
		if r := c.Check(ctx, name, args); r.Decision == DecisionDeny {
			return r
		}
	}
	return CheckResult{Decision: DecisionAllow}
}
