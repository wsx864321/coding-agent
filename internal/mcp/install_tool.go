package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InstallSourceTool 是 install_source 工具，用于在运行时安装/卸载 MCP server
type InstallSourceTool struct {
	manager     *Manager
	projectRoot string
}

// NewInstallSourceTool 创建 install_source 工具
func NewInstallSourceTool(manager *Manager, projectRoot string) *InstallSourceTool {
	return &InstallSourceTool{
		manager:     manager,
		projectRoot: projectRoot,
	}
}

func (t *InstallSourceTool) ReadOnly() bool { return false }

func (t *InstallSourceTool) Name() string { return "install_source" }

func (t *InstallSourceTool) Description() string {
	return "安装或卸载 MCP server。支持从 URL、本地路径或配置文件安装。" +
		" install 操作：添加新的 MCP server 配置并立即连接。" +
		" uninstall 操作：断开并移除 MCP server。"
}

func (t *InstallSourceTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"op": map[string]any{
				"type":        "string",
				"description": "操作类型：install 或 uninstall",
				"enum":        []string{"install", "uninstall"},
			},
			"name": map[string]any{
				"type":        "string",
				"description": "MCP server 名称（kebab-case），uninstall 时必填",
			},
			"command": map[string]any{
				"type":        "string",
				"description": "install 时：stdio 模式下的可执行文件路径",
			},
			"args": map[string]any{
				"type":        "array",
				"description": "install 时：命令行参数",
				"items":       map[string]string{"type": "string"},
			},
			"env": map[string]any{
				"type":        "object",
				"description": "install 时：环境变量",
			},
			"url": map[string]any{
				"type":        "string",
				"description": "install 时：HTTP/SSE 模式的 MCP server URL",
			},
			"headers": map[string]any{
				"type":        "object",
				"description": "install 时：HTTP 请求头",
			},
			"transport": map[string]any{
				"type":        "string",
				"description": "传输方式：stdio（默认，当有 command 时）或 http",
				"enum":        []string{"stdio", "http", "sse"},
			},
			"tier": map[string]any{
				"type":        "string",
				"description": "启动策略：eager（Agent 启动时立即连接）、background（默认，后台异步连接）",
				"enum":        []string{"eager", "background"},
			},
		},
		"required": []string{"op"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

type installArgs struct {
	Op        string            `json:"op"`
	Name      string            `json:"name"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	Env       map[string]string `json:"env"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	Transport string            `json:"transport"`
	Tier      string            `json:"tier"`
}

func (t *InstallSourceTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	raw, _ := json.Marshal(args)
	var p installArgs
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", fmt.Errorf("解析参数失败: %w", err)
	}

	switch p.Op {
	case "install":
		return t.install(ctx, p)
	case "uninstall":
		return t.uninstall(p)
	default:
		return "", fmt.Errorf("不支持的操作: %q，请使用 install 或 uninstall", p.Op)
	}
}

func (t *InstallSourceTool) install(ctx context.Context, p installArgs) (string, error) {
	if strings.TrimSpace(p.Name) == "" {
		return "", fmt.Errorf("server name 不能为空")
	}

	cfg := ServerConfig{
		Name:      p.Name,
		Command:   p.Command,
		Args:      p.Args,
		Env:       p.Env,
		URL:       p.URL,
		Headers:   p.Headers,
		Tier:      parseTier(p.Tier),
	}

	// 推断或显式指定 transport
	switch {
	case p.Transport == "http":
		cfg.Transport = TransportHTTP
	case p.Transport == "sse":
		cfg.Transport = TransportSSE
	case p.Command != "":
		cfg.Transport = TransportStdio
	case p.URL != "":
		cfg.Transport = TransportHTTP
	default:
		return "", fmt.Errorf("必须指定 command（stdio）或 url（http）")
	}

	// 保存配置到文件
	if err := t.saveConfig(cfg); err != nil {
		return "", fmt.Errorf("保存配置失败: %w", err)
	}

	// 动态连接
	if err := t.manager.AddServer(cfg); err != nil {
		return fmt.Sprintf("配置已保存，但连接失败: %v", err), nil
	}

	return fmt.Sprintf("MCP server %q 已安装并连接成功。配置已保存到 .coding-agent/mcp.json", p.Name), nil
}

func (t *InstallSourceTool) uninstall(p installArgs) (string, error) {
	if strings.TrimSpace(p.Name) == "" {
		return "", fmt.Errorf("server name 不能为空")
	}

	// 从 Manager 移除
	if err := t.manager.RemoveServer(p.Name); err != nil {
		return "", fmt.Errorf("移除失败: %w", err)
	}

	// 从配置文件移除
	if err := t.removeConfig(p.Name); err != nil {
		return fmt.Sprintf("已断开连接，但更新配置文件失败: %v", err), nil
	}

	return fmt.Sprintf("MCP server %q 已卸载。配置已从 .coding-agent/mcp.json 移除。", p.Name), nil
}

// saveConfig 将 server 配置保存到 .coding-agent/mcp.json
func (t *InstallSourceTool) saveConfig(cfg ServerConfig) error {
	manifest, err := t.loadManifest()
	if err != nil {
		return err
	}

	// 查找并替换或追加
	found := false
	for i, s := range manifest.Servers {
		if s.Name == cfg.Name {
			manifest.Servers[i] = serverConfigToFile(cfg)
			found = true
			break
		}
	}
	if !found {
		manifest.Servers = append(manifest.Servers, serverConfigToFile(cfg))
	}

	return t.writeManifest(manifest)
}

// removeConfig 从 .coding-agent/mcp.json 中移除 server
func (t *InstallSourceTool) removeConfig(name string) error {
	manifest, err := t.loadManifest()
	if err != nil {
		return err
	}

	filtered := manifest.Servers[:0]
	for _, s := range manifest.Servers {
		if s.Name != name {
			filtered = append(filtered, s)
		}
	}
	manifest.Servers = filtered

	return t.writeManifest(manifest)
}

func (t *InstallSourceTool) configPath() string {
	return filepath.Join(t.projectRoot, ".coding-agent", "mcp.json")
}

func (t *InstallSourceTool) loadManifest() (*ManifestFile, error) {
	path := t.configPath()

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &ManifestFile{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败: %w", path, err)
	}

	var manifest ManifestFile
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	return &manifest, nil
}

func (t *InstallSourceTool) writeManifest(manifest *ManifestFile) error {
	path := t.configPath()

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建目录失败: %w", err)
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", path, err)
	}

	return nil
}

// serverConfigToFile 将 ServerConfig 转为 Manifest 文件中的结构
func serverConfigToFile(cfg ServerConfig) ServerConfigFile {
	tier := string(cfg.Tier)
	if tier == "" {
		tier = string(TierBackground)
	}
	return ServerConfigFile{
		Name:    cfg.Name,
		Command: cfg.Command,
		Args:    cfg.Args,
		Env:     cfg.Env,
		URL:     cfg.URL,
		Headers: cfg.Headers,
		Tier:    tier,
	}
}
