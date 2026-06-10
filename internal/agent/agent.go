package agent

import (
	"context"
	"fmt"

	openai "github.com/sashabaranov/go-openai"

	"github.com/wsx864321/coding-agent/internal/hooks"
	"github.com/wsx864321/coding-agent/internal/permission"
	"github.com/wsx864321/coding-agent/internal/tools"
)

// Agent 是 Coding Agent 的主入口，封装 OpenAI 兼容 client + 工具注册表
//
// 典型用法：
//
//	registry := tools.NewRegistry()
//	registry.Register(tools.NewBashTool())
//	registry.Register(tools.NewReadFileTool())
//	// ... 注册其它工具
//
//	a, err := agent.NewAgent(cfg,
//	    agent.WithRegistry(registry),
//	    agent.WithChecker(checker),
//	    agent.WithHooks(hr),
//	)
//	if err != nil { log.Fatal(err) }
//
//	out, err := a.Run(ctx, "请读取 main.go 并总结")
type Agent struct {
	cfg      Config
	client   *openai.Client
	registry *tools.Registry
	// checker 在每次工具执行前做权限判断；nil 表示放行（等价于 permission.AllowAllChecker）
	checker permission.Checker
	// hooks 是可选的事件回调链；nil 时跳过所有 trigger
	hooks *hooks.Registry
	messages []openai.ChatCompletionMessage
}

// NewAgent 构造 Agent
//
// 参数：
//   - cfg：基础配置（APIKey / Model / MaxTurns / SystemPrompt 等）
//   - opts：可选注入项（见 option 包的 WithRegistry / WithChecker / WithHooks / WithClient）
//
// 内部行为：
//   - 校验 cfg 必填字段（APIKey 缺失时回退到 OPENAI_API_KEY）
//   - 自动构建 openai.Client（支持自定义 BaseURL，兼容 DeepSeek 等服务）
//   - 若 cfg.SystemPrompt 为空，则按当前 registry 自动生成（默认是空 registry）
//   - 初始化时把 system message 放到 messages 头部
//   - 按顺序应用 opts，opts 内可覆盖 registry / client / checker / hooks
//
// 所有依赖（registry / checker / hooks / client）都通过 Option 注入：
//   - agent.WithRegistry(r) → 注入工具注册表
//   - agent.WithChecker(c)  → 注入权限检查器
//   - agent.WithHooks(hr)   → 注入事件回调
//   - agent.WithClient(c)   → 替换 openai.Client
// 不传对应 Option 时使用合理的默认值（空 registry / nil checker / nil hooks / 默认 client）。
func NewAgent(cfg Config, opts ...Option) (*Agent, error) {
	if err := cfg.resolve(); err != nil {
		return nil, err
	}

	oc := openai.DefaultConfig(cfg.APIKey)
	if cfg.BaseURL != "" {
		oc.BaseURL = cfg.BaseURL
	}
	client := openai.NewClientWithConfig(oc)

	a := &Agent{
		cfg: Config{
			APIKey:       cfg.APIKey,
			BaseURL:      cfg.BaseURL,
			Model:        cfg.Model,
			MaxTokens:    cfg.MaxTokens,
			MaxTurns:     cfg.MaxTurns,
			SystemPrompt: cfg.SystemPrompt, // 暂存，下面按 registry 重新计算
			Temperature:  cfg.Temperature,
		},
		client:   client,
		registry: tools.NewRegistry(), // 兜底：默认空 registry
	}

	// 应用 Option：顺序敏感；后注册的覆盖先注册的
	for _, opt := range opts {
		if opt != nil {
			opt.apply(a)
		}
	}

	// SystemPrompt 必须在 registry 注入后才能算（依赖 registry 的工具列表）
	if a.cfg.SystemPrompt == "" {
		a.cfg.SystemPrompt = buildSystemPrompt(a.registry)
	}
	a.messages = []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: a.cfg.SystemPrompt,
		},
	}
	return a, nil
}

// =====================================================================
// Option 模式：装配期可选依赖
// =====================================================================
//
// Option 是 NewAgent 的可选注入项，由 4 个内置工厂构造（WithRegistry / WithChecker
// / WithHooks / WithClient）。Option 实现是私有 interface，外部不能伪造。
//
// 字段写入发生在 apply 方法内部——apply 是 agent 包方法，可以访问小写字段，
// 避免对外暴露 SetXxx 一类的方法。option 子包只做 re-export，业务代码 import
// option 包就能用同名工厂。

// Option 是 NewAgent 的可选注入项
type Option interface {
	apply(*Agent)
}

// optionFunc 把 func(*Agent) 适配为 Option（agent 包内部使用，option 子包不必走此入口）
type optionFunc func(*Agent)

func (f optionFunc) apply(a *Agent) {
	if f != nil {
		f(a)
	}
}

// Hooks 返回底层 Registry（只读，外部不应触发 Trigger）
func (a *Agent) Hooks() *hooks.Registry {
	return a.hooks
}

// Registry 返回底层的工具注册表
func (a *Agent) Registry() *tools.Registry {
	return a.registry
}

// Run 接收用户输入，驱动 Agent loop，最终返回 LLM 的最终回答
//
// 行为：
//   - 触发 UserPromptSubmit hook
//   - 追加 user 消息到历史
//   - 循环调用 LLM（每轮 1 次 API 调用），处理 tool_calls
//   - 终止条件：
//       A. 某轮 LLM 不返回 tool_calls，且无 Stop hook 强制续跑 → 返回 content
//       B. 达到 Config.MaxTurns → 返回 ErrMaxTurnsExceeded
func (a *Agent) Run(ctx context.Context, userInput string) (string, error) {
	if userInput == "" {
		return "", fmt.Errorf("userInput 不能为空")
	}
	// UserPromptSubmit 阶段：先把 user 内容透出，hook 可做日志 / 注入；
	// 此事件为"通知型"，不允许阻断主流程
	if a.hooks != nil {
		a.hooks.TriggerUserPromptSubmit(ctx, userInput)
	}
	a.messages = append(a.messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userInput,
	})

	for turn := 0; turn < a.cfg.MaxTurns; turn++ {
		final, err := a.loopStep(ctx)
		if err != nil {
			return "", err
		}
		if final != "" {
			return final, nil
		}
	}
	return "", fmt.Errorf("%w (limit=%d)", ErrMaxTurnsExceeded, a.cfg.MaxTurns)
}

// Messages 返回当前消息历史（只读拷贝）
func (a *Agent) Messages() []openai.ChatCompletionMessage {
	out := make([]openai.ChatCompletionMessage, len(a.messages))
	copy(out, a.messages)
	return out
}

// Reset 清空除 system message 外的所有消息历史
func (a *Agent) Reset() {
	if len(a.messages) == 0 {
		return
	}
	a.messages = a.messages[:1]
}
