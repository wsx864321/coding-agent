package permission

import (
	"context"
	"path/filepath"
	"strings"
)

// WorkdirChecker 限制 write_file / edit_file 只能在 workdir 内
//
//   - workdir 为空 → 不启用（直接 Allow）
//   - path 解析失败 → 不拦截（让工具自己处理）
//   - path 解析成功但在工作区外 → 转 Ask；Asker 拒绝则 Deny
type WorkdirChecker struct {
	Workdir string
	Asker   Asker
}

// NewWorkdirChecker 构造一个 WorkdirChecker
func NewWorkdirChecker(workdir string, asker Asker) *WorkdirChecker {
	return &WorkdirChecker{Workdir: workdir, Asker: asker}
}

// Check 实现 Checker 接口
func (w *WorkdirChecker) Check(ctx context.Context, name string, args map[string]any) CheckResult {
	if w.Workdir == "" {
		return CheckResult{Decision: DecisionAllow}
	}
	if name != "write_file" && name != "edit_file" {
		return CheckResult{Decision: DecisionAllow}
	}
	pathVal, _ := args["path"].(string)
	if pathVal == "" {
		return CheckResult{Decision: DecisionAllow}
	}
	abs, err := filepath.Abs(pathVal)
	if err != nil {
		return CheckResult{Decision: DecisionAllow}
	}
	abs = filepath.Clean(abs)
	root, err := filepath.Abs(w.Workdir)
	if err != nil {
		return CheckResult{Decision: DecisionAllow}
	}
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		reason := "写入工作区外（无法解析相对路径）"
		return w.askOrAllow(ctx, name, args, reason)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		reason := "写入工作区外：" + pathVal
		return w.askOrAllow(ctx, name, args, reason)
	}
	return CheckResult{Decision: DecisionAllow}
}

// askOrAllow 命中越界时统一处理：Asker 拒绝则 Deny，否则 Allow
func (w *WorkdirChecker) askOrAllow(ctx context.Context, name string, args map[string]any, reason string) CheckResult {
	if w.Asker == nil {
		return CheckResult{Decision: DecisionAllow}
	}
	if w.Asker.Ask(ctx, name, args, reason) {
		return CheckResult{Decision: DecisionAllow, Reason: reason + "（用户已批准）"}
	}
	return CheckResult{Decision: DecisionDeny, Reason: reason + "（用户拒绝）"}
}
