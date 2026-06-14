package agent

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// Config Agent 配置
//
// 所有字段可选；调用 NewAgent 时会自动从环境变量回退。
//
// 注意：权限检查器 / 事件 hook / openai client 这三类"装配期可选、运行期可替换"
// 的依赖已迁出 Config，用 Option 模式注入（见 agent.go 的 Option / WithChecker 等）。
// 好处是 Config 保持"只读、不可变"，新依赖不需要破坏 API。
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

	// ContextWindow 上下文窗口 token 上限；<=0 表示关闭自动压缩
	ContextWindow int

	// SoftCompactRatio 软阈值（0-1），仅提示接近上限，不触发摘要压缩
	SoftCompactRatio float64

	// CompactRatio 摘要压缩触发阈值（0-1）
	CompactRatio float64

	// CompactForceRatio 强制压缩阈值（0-1），高于该值时跳过经济性判断
	CompactForceRatio float64

	// RecentKeep 压缩时至少保留的最近消息条数下限
	RecentKeep int

	// MaxMessagesSnip 消息条数裁剪上限，超过后先做 snip_compact；<=0 表示关闭
	MaxMessagesSnip int

	// ArchiveDir 压缩归档根目录（jsonl），为空时默认：~/.coding-agent/archives
	ArchiveDir string

	// SessionDir session 持久化根目录，为空时默认：~/.coding-agent/sessions 。
	SessionDir string
}

// DefaultMaxTurns 默认最大轮数
const DefaultMaxTurns = 100

// DefaultModel 默认模型
const DefaultModel = openai.GPT4oMini

const (
	DefaultSoftCompactRatio  = 0.50
	DefaultCompactRatio      = 0.80
	DefaultCompactForceRatio = 0.90
	DefaultRecentKeep        = 3
	DefaultMaxMessagesSnip   = 80
	DefaultArchiveRetention  = 14 * 24 * 60 * 60 // 14天（秒）
	DefaultArchiveProjectCap = 1024 * 1024 * 1024
)

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
	if c.MaxTokens == 0 {
		if v, ok, err := readEnvInt("CODING_AGENT_MAX_TOKENS"); err != nil {
			return err
		} else if ok {
			c.MaxTokens = v
		}
	}
	if c.Model == "" {
		c.Model = DefaultModel
	}
	if c.MaxTurns == 0 {
		if v, ok, err := readEnvInt("CODING_AGENT_MAX_TURNS"); err != nil {
			return err
		} else if ok {
			c.MaxTurns = v
		}
	}
	if c.MaxTurns == 0 {
		c.MaxTurns = DefaultMaxTurns
	}
	if c.SystemPrompt == "" {
		c.SystemPrompt = os.Getenv("CODING_AGENT_SYSTEM_PROMPT")
	}
	if c.Temperature == 0 {
		if v, ok, err := readEnvFloat32("CODING_AGENT_TEMPERATURE"); err != nil {
			return err
		} else if ok {
			c.Temperature = v
		}
	}
	if c.ContextWindow <= 0 {
		if v, ok, err := readEnvInt("CODING_AGENT_CONTEXT_WINDOW"); err != nil {
			return err
		} else if ok {
			c.ContextWindow = v
		}
	}
	if c.SoftCompactRatio <= 0 || c.SoftCompactRatio >= 1 {
		if v, ok, err := readEnvFloat64("CODING_AGENT_SOFT_COMPACT_RATIO"); err != nil {
			return err
		} else if ok {
			c.SoftCompactRatio = v
		}
	}
	if c.SoftCompactRatio <= 0 || c.SoftCompactRatio >= 1 {
		c.SoftCompactRatio = DefaultSoftCompactRatio
	}
	if c.CompactRatio <= 0 || c.CompactRatio >= 1 {
		if v, ok, err := readEnvFloat64("CODING_AGENT_COMPACT_RATIO"); err != nil {
			return err
		} else if ok {
			c.CompactRatio = v
		}
	}
	if c.CompactRatio <= 0 || c.CompactRatio >= 1 {
		c.CompactRatio = DefaultCompactRatio
	}
	if c.CompactForceRatio <= 0 || c.CompactForceRatio >= 1 {
		if v, ok, err := readEnvFloat64("CODING_AGENT_COMPACT_FORCE_RATIO"); err != nil {
			return err
		} else if ok {
			c.CompactForceRatio = v
		}
	}
	if c.CompactForceRatio <= 0 || c.CompactForceRatio >= 1 {
		c.CompactForceRatio = DefaultCompactForceRatio
	}
	if c.RecentKeep <= 0 {
		if v, ok, err := readEnvInt("CODING_AGENT_RECENT_KEEP"); err != nil {
			return err
		} else if ok {
			c.RecentKeep = v
		}
	}
	if c.RecentKeep <= 0 {
		c.RecentKeep = DefaultRecentKeep
	}
	if c.MaxMessagesSnip <= 0 {
		if v, ok, err := readEnvInt("CODING_AGENT_MAX_MESSAGES_SNIP"); err != nil {
			return err
		} else if ok {
			c.MaxMessagesSnip = v
		}
	}
	if c.MaxMessagesSnip <= 0 {
		c.MaxMessagesSnip = DefaultMaxMessagesSnip
	}
	if c.ArchiveDir == "" {
		c.ArchiveDir = os.Getenv("CODING_AGENT_ARCHIVE_DIR")
	}
	if c.ArchiveDir == "" {
		c.ArchiveDir = defaultArchiveRootDir()
	}
	c.SessionDir = ResolveSessionDir(c.SessionDir)
	if c.CompactForceRatio < c.CompactRatio {
		c.CompactForceRatio = c.CompactRatio
	}
	return nil
}

// String 返回脱敏后的配置描述
func (c *Config) String() string {
	masked := c.APIKey
	if len(masked) > 8 {
		masked = masked[:4] + "***" + masked[len(masked)-4:]
	}
	return fmt.Sprintf("Config{APIKey=%s, BaseURL=%q, Model=%q, MaxTokens=%d, MaxTurns=%d, ContextWindow=%d, CompactRatio=%.2f}",
		masked, c.BaseURL, c.Model, c.MaxTokens, c.MaxTurns, c.ContextWindow, c.CompactRatio)
}

func readEnvInt(name string) (int, bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, false, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, true, fmt.Errorf("%s 不是合法整数: %q", name, raw)
	}
	return v, true, nil
}

func readEnvFloat64(name string) (float64, bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, false, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, true, fmt.Errorf("%s 不是合法浮点数: %q", name, raw)
	}
	return v, true, nil
}

func readEnvFloat32(name string) (float32, bool, error) {
	v, ok, err := readEnvFloat64(name)
	if err != nil || !ok {
		return 0, ok, err
	}
	return float32(v), true, nil
}

func defaultArchiveRootDir() string {
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".coding-agent", "archives")
	}
	return ".coding-agent/archives"
}

// defaultSessionRootDir 返回 session 持久化根目录，与 archive 同级。
func defaultSessionRootDir() string {
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".coding-agent", "sessions")
	}
	return ".coding-agent/sessions"
}

// ResolveSessionDir 解析 SessionDir：raw → env → 默认路径。
//
// 用于需要在 Config.resolve() 之前确定 SessionDir 的外部调用方。
func ResolveSessionDir(raw string) string {
	if raw != "" {
		return raw
	}
	if v := strings.TrimSpace(os.Getenv("CODING_AGENT_SESSION_DIR")); v != "" {
		return v
	}
	return defaultSessionRootDir()
}

func archiveProjectBucket(workdir string) string {
	wd := strings.TrimSpace(workdir)
	if wd == "" {
		if cwd, err := os.Getwd(); err == nil {
			wd = cwd
		}
	}
	if wd == "" {
		wd = "workspace"
	}
	if abs, err := filepath.Abs(wd); err == nil {
		wd = abs
	}
	sum := sha1.Sum([]byte(filepath.Clean(wd)))
	short := hex.EncodeToString(sum[:])[:12]
	name := strings.TrimSpace(filepath.Base(wd))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "workspace"
	}
	return name + "-" + short
}
