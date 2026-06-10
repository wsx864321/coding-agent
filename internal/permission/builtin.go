package permission

import (
	"context"
	"strings"
)

// =====================================================================
// Gate 1: 硬拒绝列表
// =====================================================================

// DenyPattern 是单条硬拒绝规则
//
// 匹配逻辑：
//   - ToolName == "*" 或等于 name
//   - 从 args 中取出 ArgName 对应的字符串值（ArgName=="*" 表示拼接所有 string arg）
//   - 若值中包含 Substr，则命中
type DenyPattern struct {
	ToolName string // "*" 匹配所有工具
	ArgName  string // 待检查的参数名；"*" 表示拼接所有 string 参数
	Substr   string // 子串匹配
	Reason   string // 命中时返回的拒绝原因
}

// DenyListChecker 实现 Gate 1：硬拒绝
type DenyListChecker struct {
	Patterns []DenyPattern
}

// NewDenyListChecker 构造一个 DenyListChecker，自动装载默认规则（DefaultBashDenyList）
//
// 大多数场景下直接用本函数即可；如需追加自定义规则，用 NewDenyListCheckerWith
func NewDenyListChecker() *DenyListChecker {
	return &DenyListChecker{Patterns: DefaultBashDenyList()}
}

// NewDenyListCheckerWith 构造一个 DenyListChecker，在默认规则之上追加自定义规则
//
// 自定义规则按追加顺序检查：先命中先返回 Deny；默认规则作为兜底。
// 传 nil 等价于 NewDenyListChecker()。
func NewDenyListCheckerWith(extra ...DenyPattern) *DenyListChecker {
	patterns := make([]DenyPattern, 0, len(DefaultBashDenyList())+len(extra))
	patterns = append(patterns, DefaultBashDenyList()...)
	patterns = append(patterns, extra...)
	return &DenyListChecker{Patterns: patterns}
}

// Check 实现 Checker 接口
func (d *DenyListChecker) Check(_ context.Context, name string, args map[string]any) CheckResult {
	for _, p := range d.Patterns {
		if p.ToolName != "*" && p.ToolName != name {
			continue
		}
		haystack := argValue(args, p.ArgName)
		if haystack == "" {
			continue
		}
		if strings.Contains(haystack, p.Substr) {
			reason := p.Reason
			if reason == "" {
				reason = "命中硬拒绝规则"
			}
			return CheckResult{
				Decision: DecisionDeny,
				Reason:   reason + ": '" + p.Substr + "'",
			}
		}
	}
	return CheckResult{Decision: DecisionAllow}
}

// argValue 取 args 中指定 key 的字符串值；"*" 时拼接所有 string 值
func argValue(args map[string]any, argName string) string {
	if argName == "*" {
		var s []string
		for _, v := range args {
			if str, ok := v.(string); ok {
				s = append(s, str)
			}
		}
		return strings.Join(s, "\n")
	}
	v, _ := args[argName].(string)
	return v
}

// DefaultBashDenyList 返回 bash 工具的默认硬拒绝规则
// 实际安全应靠 OS 层沙箱（macOS Seatbelt / Linux bwrap），不在此 PR 范围。
func DefaultBashDenyList() []DenyPattern {
	return []DenyPattern{
		{ToolName: "bash", ArgName: "command", Substr: "rm -rf /", Reason: "硬拒绝：尝试删除根目录"},
		{ToolName: "bash", ArgName: "command", Substr: "sudo", Reason: "硬拒绝：尝试提权"},
		{ToolName: "bash", ArgName: "command", Substr: "shutdown", Reason: "硬拒绝：尝试关机"},
		{ToolName: "bash", ArgName: "command", Substr: "reboot", Reason: "硬拒绝：尝试重启"},
		{ToolName: "bash", ArgName: "command", Substr: "mkfs", Reason: "硬拒绝：尝试格式化磁盘"},
		{ToolName: "bash", ArgName: "command", Substr: "dd if=", Reason: "硬拒绝：尝试直接写磁盘"},
		{ToolName: "bash", ArgName: "command", Substr: "> /dev/sda", Reason: "硬拒绝：尝试写入磁盘设备"},
	}
}
