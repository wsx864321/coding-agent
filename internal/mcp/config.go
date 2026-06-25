package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Transport 表示 MCP server 的通信方式
type Transport string

const (
	TransportStdio Transport = "stdio"
	TransportHTTP  Transport = "http"
	TransportSSE   Transport = "sse"
)

// Tier 表示 MCP server 的启动策略
type Tier string

const (
	TierEager     Tier = "eager"     // Agent 启动时立即连接
	TierBackground Tier = "background" // Agent 启动后异步连接
	TierLazy      Tier = "lazy"      // 首次使用时连接（默认）
)

// Scope 表示配置来源层级
type Scope string

const (
	ScopeProject Scope = "project"
	ScopeGlobal  Scope = "global"
)

// ServerConfig 描述一个 MCP server 的连接配置
type ServerConfig struct {
	Name      string            // 唯一标识（kebab-case）
	Command   string            // stdio: 可执行文件路径
	Args      []string          // stdio: 命令行参数
	Env       map[string]string // stdio: 环境变量
	URL       string            // http/sse: 服务端地址
	Headers   map[string]string // http/sse: 请求头（支持 ${VAR} 占位符）
	Transport Transport         // 传输方式
	Tier      Tier              // 启动策略
	Scope     Scope             // 配置来源
	Source    string            // 配置文件绝对路径
}

// IsStdio 判断是否为 stdio 传输
func (c ServerConfig) IsStdio() bool { return c.Transport == TransportStdio }

// IsHTTP 判断是否为 HTTP 传输
func (c ServerConfig) IsHTTP() bool { return c.Transport == TransportHTTP }

// ManifestFile 是磁盘上 mcp.json 的结构
type ManifestFile struct {
	Servers []ServerConfigFile `json:"servers"`
}

// ServerConfigFile 是 mcp.json 中单个 server 的结构
type ServerConfigFile struct {
	Name    string            `json:"name"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Tier    string            `json:"tier,omitempty"`
}

// LoadOptions 控制 MCP 配置的扫描范围
type LoadOptions struct {
	ProjectRoot string // 项目根目录；空字符串跳过项目级扫描
	HomeDir     string // 用户主目录；空字符串自动检测
}

var userHomeDir = os.UserHomeDir

// Load 从项目级和全局级 mcp.json 加载 MCP server 配置
//
// 合并规则：同名 server 项目级覆盖全局级。
func Load(opts LoadOptions) []ServerConfig {
	var configs []ServerConfig

	// 先加载全局配置（低优先级）
	home := opts.HomeDir
	if home == "" {
		var err error
		home, err = userHomeDir()
		if err != nil {
			home = ""
		}
	}
	if home != "" {
		globalPath := filepath.Join(home, ".coding-agent", "mcp.json")
		configs = append(configs, loadFile(globalPath, ScopeGlobal)...)
	}

	// 再加载项目配置（高优先级）
	if opts.ProjectRoot != "" {
		projPath := filepath.Join(opts.ProjectRoot, ".coding-agent", "mcp.json")
		configs = append(configs, loadFile(projPath, ScopeProject)...)
	}

	// 去重：同名 server 保留后出现的（项目级覆盖全局级）
	return dedupeByName(configs)
}

func loadFile(path string, scope Scope) []ServerConfig {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var manifest ManifestFile
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil
	}

	abs, _ := filepath.Abs(path)

	var out []ServerConfig
	for _, s := range manifest.Servers {
		if s.Name == "" {
			continue
		}
		tier := parseTier(s.Tier)
		transport := inferTransport(s)

		// 无有效连接方式则跳过
		if transport == "" {
			continue
		}

		out = append(out, ServerConfig{
			Name:      s.Name,
			Command:   s.Command,
			Args:      s.Args,
			Env:       s.Env,
			URL:       s.URL,
			Headers:   s.Headers,
			Transport: transport,
			Tier:      tier,
			Scope:     scope,
			Source:    abs,
		})
	}
	return out
}

// inferTransport 根据配置字段推断 transport 类型
func inferTransport(s ServerConfigFile) Transport {
	if s.Command != "" {
		return TransportStdio
	}
	if s.URL != "" {
		return TransportHTTP
	}
	return ""
}

// parseTier 解析 tier 字符串
func parseTier(raw string) Tier {
	switch raw {
	case "eager":
		return TierEager
	case "background":
		return TierBackground
	default:
		return TierBackground // 默认 background
	}
}

// dedupeByName 同名 server 保留最后一个（项目级覆盖全局级）
func dedupeByName(configs []ServerConfig) []ServerConfig {
	seen := make(map[string]int) // name → index in result
	result := make([]ServerConfig, 0, len(configs))
	for _, c := range configs {
		if idx, exists := seen[c.Name]; exists {
			result[idx] = c
		} else {
			seen[c.Name] = len(result)
			result = append(result, c)
		}
	}
	return result
}
