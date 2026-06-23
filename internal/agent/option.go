package agent

import (
	"github.com/wsx864321/coding-agent/internal/jobs"
	"github.com/wsx864321/coding-agent/internal/memory"
	"github.com/wsx864321/coding-agent/internal/permission"
	"github.com/wsx864321/coding-agent/internal/provider"
	"github.com/wsx864321/coding-agent/internal/skill"
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

// hooksOpt 注入 ToolHooks
type hooksOpt struct{ h ToolHooks }

func (o hooksOpt) apply(a *Agent) {
	a.hooks = o.h
}

// providerOpt 注入 Provider 实例
type providerOpt struct{ p provider.Provider }

func (o providerOpt) apply(a *Agent) {
	a.prov = o.p
}

// WithRegistry 注入工具注册表
func WithRegistry(r *tools.Registry) Option {
	return registryOpt{r: r}
}

// WithChecker 注入权限检查器
func WithChecker(c permission.Checker) Option {
	return checkerOpt{c: c}
}

// WithHooks 注入事件 hook 实现（可为 nil 或空 Runner）
func WithHooks(h ToolHooks) Option {
	return hooksOpt{h: h}
}

// WithProvider 注入 Provider 实例（用于 subagent 共享、测试 mock 等场景）
func WithProvider(p provider.Provider) Option {
	return providerOpt{p: p}
}

// skillStoreOpt 注入 skill Store
type skillStoreOpt struct{ s *skill.Store }

func (o skillStoreOpt) apply(a *Agent) {
	a.skillStore = o.s
}

// WithSkillStore 注入 skill Store
func WithSkillStore(s *skill.Store) Option {
	return skillStoreOpt{s: s}
}

// memorySetOpt 注入 memory Set
type memorySetOpt struct{ s *memory.Set }

func (o memorySetOpt) apply(a *Agent) {
	a.memSet = o.s
}

// WithMemory 注入 memory Set
func WithMemory(s *memory.Set) Option {
	return memorySetOpt{s: s}
}

// jobManagerOpt 注入后台任务 Manager
type jobManagerOpt struct{ m *jobs.Manager }

func (o jobManagerOpt) apply(a *Agent) {
	a.jobMgr = o.m
}

// WithJobManager 注入后台任务 Manager。注入后，bash(run_in_background) 和
// task(run_in_background) 启动的后台 job 跨 turn 存活，bash_output/kill_shell/wait
// 可操作它们。nil 表示禁用后台执行。
func WithJobManager(m *jobs.Manager) Option {
	return jobManagerOpt{m: m}
}
