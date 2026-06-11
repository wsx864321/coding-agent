package agent

import (
	"context"
	"fmt"

	openai "github.com/sashabaranov/go-openai"

	"github.com/wsx864321/coding-agent/internal/evidence"
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
	// checker 在每次工具执行前做权限判断；nil 表示放行
	checker permission.Checker
	// hooks 是可选的事件回调链；nil 时跳过所有 trigger
	hooks    *hooks.Registry
	messages []openai.ChatCompletionMessage
	// ledger 是证据账本，为 todo_write / complete_step 提供工具调用凭证
	ledger *evidence.Ledger
}

// NewAgent 构造 Agent
//
// 参数：
//   - cfg：基础配置（APIKey / Model / MaxTurns / SystemPrompt 等）
//   - opts：可选注入项（见 option 包的 WithRegistry / WithChecker / WithHooks）
//
// 内部行为：
//   - 校验 cfg 必填字段（APIKey 缺失时回退到 OPENAI_API_KEY）
//   - 自动构建 openai.Client（支持自定义 BaseURL，兼容 DeepSeek 等服务）
//   - 若 cfg.SystemPrompt 为空，则按当前 registry 自动生成（默认是空 registry）
//   - 初始化时把 system message 放到 messages 头部
//   - 按顺序应用 opts，opts 内可覆盖 registry / checker / hooks
//
// 所有依赖（registry / checker / hooks）都通过 Option 注入。
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
			SystemPrompt: cfg.SystemPrompt,
			Temperature:  cfg.Temperature,
		},
		client:   client,
		registry: tools.NewRegistry(),
		ledger:   evidence.NewLedger(),
	}

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

// Hooks 返回底层 Registry（只读，外部不应触发 Trigger）
func (a *Agent) Hooks() *hooks.Registry {
	return a.hooks
}

// Registry 返回底层的工具注册表
func (a *Agent) Registry() *tools.Registry {
	return a.registry
}

// WireTaskTool 把 task 工具的 SubagentRunner 连接到当前 Agent 实例。
//
// 必须在 NewAgent 之后调用——TaskTool 的 runner 闭包需要捕获已构造完成的 Agent。
// 若 registry 中没有 task 工具则静默跳过。
//
// 典型调用路径：CLI 入口 → NewAgent → WireTaskTool
func (a *Agent) WireTaskTool() {
	t := a.registry.Get("task")
	if t == nil {
		return
	}
	tt, ok := t.(*tools.TaskTool)
	if !ok {
		return
	}
	tt.SetRunner(func(ctx context.Context, prompt string) (string, error) {
		var subHooks *hooks.Registry
		if a.hooks != nil {
			subHooks = a.hooks.WithoutStopAndPrompt()
		}
		return RunSubAgent(ctx, a, prompt, SubagentOptions{
			Hooks:   subHooks,
			Checker: a.checker,
		})
	})
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

	// 每轮用户输入重置 per-turn 证据（receipts、guardBlocks），保留 currentTodos
	if a.ledger != nil {
		a.ledger.Reset()
		ctx = evidence.WithLedger(ctx, a.ledger)
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
