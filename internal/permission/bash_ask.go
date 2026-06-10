package permission

import (
	"context"
	"strings"
)

// BashAskChecker 对 bash 工具的"破坏性关键字"做 Ask
//
// 命中任一关键字后转交 Asker：
//   - Asker 批准 → Allow（reason 标记"用户已批准"）
//   - Asker 拒绝 → Deny
//   - Asker 为 nil → Allow（与 Pipeline 语义对齐）
//
// 与 DenyListChecker 的区别：硬拒绝是不可逾越的安全边界；Ask 是"需要确认的危险操作"。
type BashAskChecker struct {
	Asker Asker
}

// NewBashAskChecker 构造一个 bash Ask checker
func NewBashAskChecker(asker Asker) *BashAskChecker {
	return &BashAskChecker{Asker: asker}
}

// Check 实现 Checker 接口
func (b *BashAskChecker) Check(ctx context.Context, name string, args map[string]any) CheckResult {
	if name != "bash" {
		return CheckResult{Decision: DecisionAllow}
	}
	cmd, _ := args["command"].(string)
	if cmd == "" {
		return CheckResult{Decision: DecisionAllow}
	}
	kws := []string{
		"rm ", "rmdir",
		"> /etc/", "chmod 777",
		"del ", "rd /s", "rd /q",
		"format ",
	}
	for _, kw := range kws {
		if strings.Contains(cmd, kw) {
			reason := "潜在破坏性 bash 命令（包含 '" + kw + "'）"
			if b.Asker == nil {
				// 无 Asker：放行（与 Pipeline 行为对齐）
				return CheckResult{Decision: DecisionAllow}
			}
			if b.Asker.Ask(ctx, name, args, reason) {
				return CheckResult{Decision: DecisionAllow, Reason: reason + "（用户已批准）"}
			}
			return CheckResult{Decision: DecisionDeny, Reason: reason + "（用户拒绝）"}
		}
	}
	return CheckResult{Decision: DecisionAllow}
}
