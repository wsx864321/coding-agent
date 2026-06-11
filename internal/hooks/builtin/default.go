package builtin

import (
	"io"

	"github.com/wsx864321/coding-agent/internal/hooks"
)

// NewDefault 构造一个新 Registry 并把全部内置 hook 注入其中
//
// 等价于 NewRegistry() + Default(r, out)，适合一次性拿 Registry 的场景
// （如 cmd/cli 的 once / chat 装配入口）
//
// 默认装配 4 个 hook：Log → LargeOutput → ContextInject → Summary
// 触发顺序由注册顺序决定（详见 Default 注释）
//
// 注意：权限相关检查（bash deny / bash Ask / 越界）由 internal/permission 的
// Pipeline + DenyListChecker / BashAskChecker / WorkdirChecker 承担，详见
// agent.WithChecker 的注入路径。
func NewDefault(out io.Writer, workdir string) *hooks.Registry {
	r := hooks.NewRegistry()
	sink := &Sink{W: out}

	lh := NewLogHook()
	lh.Sink = sink
	r.RegisterPreToolUse(lh.Handle)

	loh := NewLargeOutputHook()
	loh.Sink = sink
	r.RegisterPostToolUse(loh.Handle)

	cih := NewContextInjectHook(workdir)
	cih.Sink = sink
	r.RegisterUserPromptSubmit(cih.Handle)

	// TodoGuard 必须在 Summary 之前注册：
	// StopHook 链中首个返回 force 的 hook 短路后续，
	// TodoGuard 阻断时 Summary 不会执行（符合预期：会话尚未结束）
	tgh := NewTodoGuardHook()
	tgh.Sink = sink
	r.RegisterStop(tgh.Handle)

	sh := NewSummaryHook()
	sh.Sink = sink
	r.RegisterStop(sh.Handle)

	return r
}
