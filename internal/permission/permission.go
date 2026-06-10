// Package permission 实现 coding-agent 的"执行前权限判断"管线。
//
//	+-------+    +--------+    +--------+    +--------+    +------+
//	| Tool  | -> | Gate 1 | -> | Gate 2 | -> | Gate 3 | -> | Exec |
//	| call  |    | deny?  |    | match? |    | allow? |    |      |
//	+-------+    +--------+    +--------+    +--------+    +------+
//
//	- Gate 1（硬拒绝列表）: 命中 → 直接 Deny，不执行
//	- Gate 2（规则匹配）:   命中 → 进入 Gate 3 询问
//	- Gate 3（用户审批）:   用户决定 Allow / Deny
//	- 三道都没命中 →        Allow，直接执行
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
	// DecisionAsk  需要进一步确认（通常由 Asker 决议）
	// Pipeline 会内部消化 Ask，对外只暴露 Allow / Deny
	DecisionAsk
)

// String 返回 Decision 的可读名称
func (d Decision) String() string {
	switch d {
	case DecisionAllow:
		return "allow"
	case DecisionDeny:
		return "deny"
	case DecisionAsk:
		return "ask"
	}
	return "unknown"
}

// ToolCall 是 Checker 所需的最小信息：工具名 + 参数
//
// Args 保持 map[string]any 形态，与 tool.Execute 签名一致；
// 这样 Checker 既能按"key"精确查（如 bash 的 "command"），
// 也能跨 key 模糊查（"*" 通配）。
type ToolCall struct {
	Name string
	Args map[string]any
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
//   - DecisionAsk   → 把决定权交给 Asker（仅 AskRuleChecker 适用）
type Checker interface {
	Check(ctx context.Context, call ToolCall) CheckResult
}

// Asker 把 Ask 决议转成 Allow / Deny
//
// 实现可以是命令行交互、Web 弹窗、自动审批器等。
// 返回 true 表示放行，false 表示拒绝。
type Asker interface {
	Ask(ctx context.Context, call ToolCall, reason string) bool
}

// AskerFunc 把函数适配为 Asker
type AskerFunc func(ctx context.Context, call ToolCall, reason string) bool

// Ask 实现 Asker 接口
func (f AskerFunc) Ask(ctx context.Context, call ToolCall, reason string) bool {
	return f(ctx, call, reason)
}

// DecisionFunc 把"返回 Decision 的函数"适配为 Checker
type DecisionFunc func(ctx context.Context, call ToolCall) Decision

// Check 实现 Checker 接口
func (f DecisionFunc) Check(ctx context.Context, call ToolCall) CheckResult {
	return CheckResult{Decision: f(ctx, call)}
}

// Pipeline 把"硬拒绝"和"询问规则"两阶段串成一个 Checker
//
// 流程：
//  1. 依次执行 Deny 中的 Checker；第一个返回 Deny 的短路整个流程
//  2. 依次执行 Ask 中的 Checker；第一个返回 Ask 的转交 Asker
//     - Asker 为 nil 时，Ask 被视为 Allow（无法询问 → 跳过规则）
//  3. 都没有命中 → Allow
type Pipeline struct {
	Deny  []Checker
	Ask   []Checker
	Asker Asker
}

// Check 实现 Checker 接口
func (p *Pipeline) Check(ctx context.Context, call ToolCall) CheckResult {
	// Gate 1: 硬拒绝
	for _, c := range p.Deny {
		if r := c.Check(ctx, call); r.Decision == DecisionDeny {
			return r
		}
	}
	// Gate 2 + Gate 3: 规则匹配 → 用户审批
	for _, c := range p.Ask {
		r := c.Check(ctx, call)
		if r.Decision != DecisionAsk {
			continue
		}
		if p.Asker == nil {
			// 无 Asker：把 Ask 视作 Allow（让 Pipeline 继续探测后面的规则）
			continue
		}
		if p.Asker.Ask(ctx, call, r.Reason) {
			return CheckResult{
				Decision: DecisionAllow,
				Reason:   r.Reason + "（用户已批准）",
			}
		}
		return CheckResult{
			Decision: DecisionDeny,
			Reason:   r.Reason + "（用户拒绝）",
		}
	}
	return CheckResult{Decision: DecisionAllow}
}

// AllowAllChecker 放行所有调用的 Checker（用于禁用权限检查 / 测试）
type AllowAllChecker struct{}

// Check 实现 Checker 接口
func (AllowAllChecker) Check(_ context.Context, _ ToolCall) CheckResult {
	return CheckResult{Decision: DecisionAllow}
}
