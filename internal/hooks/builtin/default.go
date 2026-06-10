package builtin

import (
	"io"

	"github.com/wsx864321/coding-agent/internal/hooks"
	"github.com/wsx864321/coding-agent/internal/permission"
)

// NewDefault 构造一个新 Registry 并把全部内置 hook 注入其中
//
// 等价于 NewRegistry() + Default(r, workdir, asker, out)，适合一次性拿 Registry 的场景
// （如 cmd/cli 的 once / chat 装配入口）
//
// 参数：
//   - workdir：写入工具越界检查的基准目录；空字符串不启用越界规则
//   - asker：Ask 阶段的用户裁决；nil 时 Ask 视为 Allow
//   - out：所有 hook 的输出目标；nil 时默认 os.Stderr
//
// 默认装配 5 个 hook：Permission / Log / ContextInject / LargeOutput / Summary
// 触发顺序由注册顺序决定（详见 Default 注释）
func NewDefault(workdir string, asker permission.Asker, out io.Writer) *hooks.Registry {
	r := hooks.NewRegistry()
	Default(r, workdir, asker, out)
	return r
}

// Default 把 4 个内置 hook 一次性注册到 r
//
// 适合在已有 Registry 上叠加内置 hook 的场景（如测试 / 自定义 hook 链 + 内置 hook）
//
// 装配顺序（注册顺序）即触发顺序：Permission → Log → LargeOutput → ContextInject → Summary
// 其中 Permission 和 Log 同属 PreToolUse，按"先安全后审计"的原则排列
func Default(r *hooks.Registry, workdir string, asker permission.Asker, out io.Writer) {
	sink := &Sink{W: out}

	ph := NewPermissionHook(workdir, asker)
	ph.Sink = sink
	r.RegisterPreToolUse(ph.Handle)

	lh := NewLogHook()
	lh.Sink = sink
	r.RegisterPreToolUse(lh.Handle)

	loh := NewLargeOutputHook()
	loh.Sink = sink
	r.RegisterPostToolUse(loh.Handle)

	cih := NewContextInjectHook(workdir)
	cih.Sink = sink
	r.RegisterUserPromptSubmit(cih.Handle)

	sh := NewSummaryHook()
	sh.Sink = sink
	r.RegisterStop(sh.Handle)
}
