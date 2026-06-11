package agent

import (
	"github.com/wsx864321/coding-agent/internal/hooks"
	"github.com/wsx864321/coding-agent/internal/permission"
	"github.com/wsx864321/coding-agent/internal/tools"
)

// Option 是 NewAgent 的可选注入项
type Option interface {
	apply(*Agent)
}

// registryOpt 注入工具注册表
type registryOpt struct{ r *tools.Registry }

func (o registryOpt) apply(a *Agent) {
	if o.r != nil {
		a.registry = o.r
	}
}

// checkerOpt 注入权限检查器
type checkerOpt struct{ c permission.Checker }

func (o checkerOpt) apply(a *Agent) {
	a.checker = o.c
}

// hooksOpt 注入事件回调 Registry
type hooksOpt struct{ r *hooks.Registry }

func (o hooksOpt) apply(a *Agent) {
	a.hooks = o.r
}

// WithRegistry 注入工具注册表
//
//   - 传 nil：保留 NewAgent 默认构造的空注册表
//   - 不传：NewAgent 内部会用 tools.NewRegistry() 自动构造一个空注册表
func WithRegistry(r *tools.Registry) Option {
	return registryOpt{r: r}
}

// WithChecker 注入权限检查器
//
//   - 传 nil：放行所有调用
func WithChecker(c permission.Checker) Option {
	return checkerOpt{c: c}
}

// WithHooks 注入事件回调 Registry
//
//   - 传 nil：禁用所有 hook trigger（等价于 NewRegistry 后不注册任何 hook）
func WithHooks(r *hooks.Registry) Option {
	return hooksOpt{r: r}
}
