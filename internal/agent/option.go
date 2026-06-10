package agent

import (
	openai "github.com/sashabaranov/go-openai"

	"github.com/wsx864321/coding-agent/internal/hooks"
	"github.com/wsx864321/coding-agent/internal/permission"
	"github.com/wsx864321/coding-agent/internal/tools"
)

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

// clientOpt 替换 openai.Client
type clientOpt struct{ c *openai.Client }

func (o clientOpt) apply(a *Agent) {
	if o.c != nil {
		a.client = o.c
	}
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
//   - 传 nil：放行所有调用（等价于 permission.AllowAllChecker）
func WithChecker(c permission.Checker) Option {
	return checkerOpt{c: c}
}

// WithHooks 注入事件回调 Registry
//
//   - 传 nil：禁用所有 hook trigger（等价于 NewRegistry 后不注册任何 hook）
func WithHooks(r *hooks.Registry) Option {
	return hooksOpt{r: r}
}

// WithClient 替换 openai.Client（主要给 fake LLM 测试用）
//
//   - 传 nil：保留 NewAgent 默认构造的 client
//   - 默认 baseURL 构造的 client 仍能跑通大多数 fake 场景
//     （URL 写的是 httptest 的真实 URL）；只有需要"绝对确保打到 fake"时才替换
func WithClient(c *openai.Client) Option {
	return clientOpt{c: c}
}
