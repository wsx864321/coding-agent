package agent

import (
	"errors"
	"fmt"
	"os"

	openai "github.com/sashabaranov/go-openai"

	"github.com/wsx864321/coding-agent/internal/permission"
)

// Config Agent 配置
//
// 所有字段可选；调用 NewAgent 时会自动从环境变量回退。
type Config struct {
	// APIKey OpenAI 兼容服务的 API key
	// 留空时从环境变量 OPENAI_API_KEY 读取
	APIKey string

	// BaseURL OpenAI 兼容服务的 base URL（如 "https://api.deepseek.com/v1"）
	// 留空时从环境变量 OPEN_BASE_URL 读取，仍为空则使用 OpenAI 官方地址
	BaseURL string

	// Model 使用的模型名
	// 留空时从环境变量 OPENAI_MODEL 读取，仍为空则使用 openai.GPT4oMini
	Model string

	// MaxTokens 单次响应最大 token 数；0 表示不传该参数
	MaxTokens int

	// MaxTurns Agent loop 最大轮数；0 表示使用默认值 20
	// 超过 MaxTurns 后强制结束，返回 ErrMaxTurnsExceeded
	MaxTurns int

	// SystemPrompt 系统提示
	// 留空时由 prompt.go 根据 registry 中已注册工具自动生成
	SystemPrompt string

	// Temperature 采样温度；0 表示不传该参数（API 默认 1）
	Temperature float32

	// Checker 工具执行前的权限检查器；nil 时放行所有调用
	//
	// 构造时通常与 cmd/cli 提供的 Asker 串联成 Pipeline（deny → ask → allow）。
	// 也可通过 Agent.SetChecker 在运行时替换。
	Checker permission.Checker
}

// DefaultMaxTurns 默认最大轮数
const DefaultMaxTurns = 20

// DefaultModel 默认模型
const DefaultModel = openai.GPT4oMini

// resolve 应用环境变量回退与默认值
func (c *Config) resolve() error {
	if c.APIKey == "" {
		c.APIKey = os.Getenv("OPENAI_API_KEY")
	}
	if c.APIKey == "" {
		return errors.New("APIKey 未设置：传入 cfg.APIKey 或设置环境变量 OPENAI_API_KEY")
	}
	if c.BaseURL == "" {
		c.BaseURL = os.Getenv("OPEN_BASE_URL")
	}

	if c.Model == "" {
		c.Model = os.Getenv("OPENAI_MODEL")
	}
	if c.Model == "" {
		c.Model = DefaultModel
	}
	if c.MaxTurns == 0 {
		c.MaxTurns = DefaultMaxTurns
	}
	return nil
}

// String 返回脱敏后的配置描述
func (c *Config) String() string {
	masked := c.APIKey
	if len(masked) > 8 {
		masked = masked[:4] + "***" + masked[len(masked)-4:]
	}
	return fmt.Sprintf("Config{APIKey=%s, BaseURL=%q, Model=%q, MaxTokens=%d, MaxTurns=%d}",
		masked, c.BaseURL, c.Model, c.MaxTokens, c.MaxTurns)
}
