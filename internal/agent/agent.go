package agent

import (
	"context"
	"fmt"

	openai "github.com/sashabaranov/go-openai"

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
//	cfg := agent.Config{Model: openai.GPT4oMini}
//	a, err := agent.NewAgent(cfg, registry)
//	if err != nil { log.Fatal(err) }
//
//	out, err := a.Run(ctx, "请读取 main.go 并总结")
type Agent struct {
	cfg      Config
	client   *openai.Client
	registry *tools.Registry
	messages []openai.ChatCompletionMessage
}

// NewAgent 构造 Agent
//
// 内部行为：
//   - 校验 Config 必填字段（APIKey 缺失时回退到 OPENAI_API_KEY）
//   - 自动构建 openai.Client（支持自定义 BaseURL，兼容 DeepSeek 等服务）
//   - 若 cfg.SystemPrompt 为空，则按 registry 自动生成
//   - 初始化时把 system message 放到 messages 头部
func NewAgent(cfg Config, registry *tools.Registry) (*Agent, error) {
	if registry == nil {
		return nil, fmt.Errorf("registry 不能为 nil")
	}
	if err := cfg.resolve(); err != nil {
		return nil, err
	}

	oc := openai.DefaultConfig(cfg.APIKey)
	if cfg.BaseURL != "" {
		oc.BaseURL = cfg.BaseURL
	}
	client := openai.NewClientWithConfig(oc)

	systemPrompt := cfg.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = buildSystemPrompt(registry)
	}

	a := &Agent{
		cfg: Config{
			APIKey:       cfg.APIKey,
			BaseURL:      cfg.BaseURL,
			Model:        cfg.Model,
			MaxTokens:    cfg.MaxTokens,
			MaxTurns:     cfg.MaxTurns,
			SystemPrompt: systemPrompt,
			Temperature:  cfg.Temperature,
		},
		client:   client,
		registry: registry,
		messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
		},
	}
	return a, nil
}

// Run 接收用户输入，驱动 Agent loop，最终返回 LLM 的最终回答
//
// 行为：
//   - 追加 user 消息到历史
//   - 循环调用 LLM（每轮 1 次 API 调用），处理 tool_calls
//   - 终止条件：
//       A. 某轮 LLM 不返回 tool_calls → 返回 content
//       B. 达到 Config.MaxTurns → 返回 ErrMaxTurnsExceeded
func (a *Agent) Run(ctx context.Context, userInput string) (string, error) {
	if userInput == "" {
		return "", fmt.Errorf("userInput 不能为空")
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

// Registry 返回底层的工具注册表
func (a *Agent) Registry() *tools.Registry {
	return a.registry
}
