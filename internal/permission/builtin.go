package permission

import (
	"context"
	"path/filepath"
	"strings"
)

// =====================================================================
// Gate 1: 硬拒绝列表
// =====================================================================

// DenyPattern 是单条硬拒绝规则
//
// 匹配逻辑：
//   - ToolName == "*" 或等于 call.Name
//   - 从 call.Args 中取出 ArgName 对应的字符串值（ArgName=="*" 表示拼接所有 string arg）
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

// Check 实现 Checker 接口
func (d *DenyListChecker) Check(_ context.Context, call ToolCall) CheckResult {
	for _, p := range d.Patterns {
		if p.ToolName != "*" && p.ToolName != call.Name {
			continue
		}
		haystack := argValue(call, p.ArgName)
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

// argValue 取 call.Args 中指定 key 的字符串值；"*" 时拼接所有 string 值
func argValue(call ToolCall, argName string) string {
	if argName == "*" {
		var s []string
		for _, v := range call.Args {
			if str, ok := v.(string); ok {
				s = append(s, str)
			}
		}
		return strings.Join(s, "\n")
	}
	v, _ := call.Args[argName].(string)
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

// =====================================================================
// Gate 2: 规则匹配
// =====================================================================

// AskRule 是单条"需要用户确认"的规则
//
// ToolNames 命中工具（"*" 通配）；Check 返回是否匹配 + 拒绝原因。
type AskRule struct {
	ToolNames []string
	Check     func(call ToolCall) (matched bool, reason string)
}

// AskRuleChecker 实现 Gate 2：上下文相关规则
//
// 第一个匹配的规则短路返回 Ask；都没命中 → Allow。
type AskRuleChecker struct {
	Rules []AskRule
}

// Check 实现 Checker 接口
func (a *AskRuleChecker) Check(_ context.Context, call ToolCall) CheckResult {
	for _, r := range a.Rules {
		if !toolMatches(r.ToolNames, call.Name) {
			continue
		}
		matched, reason := r.Check(call)
		if matched {
			return CheckResult{Decision: DecisionAsk, Reason: reason}
		}
	}
	return CheckResult{Decision: DecisionAllow}
}

func toolMatches(patterns []string, name string) bool {
	for _, p := range patterns {
		if p == "*" || p == name {
			return true
		}
	}
	return false
}

// DefaultBashAskRules 返回 bash 工具的默认 Ask 规则：包含破坏性关键字的命令需要确认
func DefaultBashAskRules() AskRule {
	kws := []string{
		"rm ", "rmdir",
		"> /etc/", "chmod 777",
		"del ", "rd /s", "rd /q",
		"format ",
	}
	return AskRule{
		ToolNames: []string{"bash"},
		Check: func(call ToolCall) (bool, string) {
			cmd := argValue(call, "command")
			if cmd == "" {
				return false, ""
			}
			for _, kw := range kws {
				if strings.Contains(cmd, kw) {
					return true, "潜在破坏性 bash 命令（包含 '" + kw + "'）"
				}
			}
			return false, ""
		},
	}
}

// WriteOutsideWorkdirRule 返回"写入工作区外需要确认"的规则
//
// 适用工具：write_file / edit_file
//
//   - workdir 为空 → 规则不生效
//   - path 解析失败 → 不拦截（让工具自己处理）
//   - path 解析成功但在工作区外 → Ask
//
// 路径语义复用 tools.isInAllowedDirs 的判断：相对 workdir 必须不以 ".." 开头。
func WriteOutsideWorkdirRule(workdir string) AskRule {
	return AskRule{
		ToolNames: []string{"write_file", "edit_file"},
		Check: func(call ToolCall) (bool, string) {
			if workdir == "" {
				return false, ""
			}
			pathVal := argValue(call, "path")
			if pathVal == "" {
				return false, ""
			}
			abs, err := filepath.Abs(pathVal)
			if err != nil {
				return false, ""
			}
			abs = filepath.Clean(abs)
			root, err := filepath.Abs(workdir)
			if err != nil {
				return false, ""
			}
			root = filepath.Clean(root)
			rel, err := filepath.Rel(root, abs)
			if err != nil {
				return true, "写入工作区外（无法解析相对路径）"
			}
			if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return true, "写入工作区外：" + pathVal
			}
			return false, ""
		},
	}
}
