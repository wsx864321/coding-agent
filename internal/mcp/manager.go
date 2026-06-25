package mcp

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/wsx864321/coding-agent/internal/tools"
)

// Manager 管理所有 MCP server 的连接生命周期和工具注册
type Manager struct {
	configs []ServerConfig

	mu      sync.RWMutex
	clients map[string]Client       // serverName → Client
	tools   map[string]*Tool        // fullName → Tool wrapper
	servers map[string]*serverState // serverName → state

	registry *tools.Registry // 工具注册表引用

	ctx    context.Context
	cancel context.CancelFunc
}

type serverState struct {
	Config  ServerConfig
	Client  Client
	Tools   []*Tool
	Running bool
	Err     error
}

// NewManager 创建一个 MCP Manager
func NewManager(configs []ServerConfig, registry *tools.Registry) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		configs:  configs,
		clients:  make(map[string]Client),
		tools:    make(map[string]*Tool),
		servers:  make(map[string]*serverState),
		registry: registry,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start 启动所有 MCP server 连接
//
// Eager tier: 同步连接，失败会记录错误但不阻塞其他 server
// Background tier: 异步连接
func (m *Manager) Start() {
	var eagerConfigs, bgConfigs []ServerConfig
	for _, cfg := range m.configs {
		if cfg.Tier == TierEager {
			eagerConfigs = append(eagerConfigs, cfg)
		} else {
			bgConfigs = append(bgConfigs, cfg)
		}
	}

	// Eager: 同步连接
	for _, cfg := range eagerConfigs {
		m.connectServer(cfg)
	}

	// Background: 异步连接
	if len(bgConfigs) > 0 {
		go func() {
			for _, cfg := range bgConfigs {
				if m.ctx.Err() != nil {
					return
				}
				m.connectServer(cfg)
			}
		}()
	}
}

// connectServer 连接单个 MCP server 并注册其工具
func (m *Manager) connectServer(cfg ServerConfig) {
	log.Printf("[MCP] connecting to server %q (%s)...", cfg.Name, cfg.Transport)

	var client Client
	switch {
	case cfg.IsStdio():
		client = NewStdioClient(cfg)
	case cfg.IsHTTP():
		client = NewHTTPClient(cfg)
	default:
		log.Printf("[MCP] unsupported transport for server %q", cfg.Name)
		return
	}

	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		log.Printf("[MCP] failed to connect to server %q: %v", cfg.Name, err)
		m.mu.Lock()
		m.servers[cfg.Name] = &serverState{
			Config: cfg,
			Err:    err,
		}
		m.mu.Unlock()
		return
	}

	// 发现工具
	toolInfos, err := client.ListTools(ctx)
	if err != nil {
		log.Printf("[MCP] failed to list tools from server %q: %v", cfg.Name, err)
		client.Close()
		m.mu.Lock()
		m.servers[cfg.Name] = &serverState{
			Config: cfg,
			Client: client,
			Err:    err,
		}
		m.mu.Unlock()
		return
	}

	// 创建工具包装器并注册
	m.mu.Lock()
	defer m.mu.Unlock()

	state := &serverState{
		Config:  cfg,
		Client:  client,
		Running: true,
	}
	m.clients[cfg.Name] = client
	m.servers[cfg.Name] = state

	for _, info := range toolInfos {
		tool := NewTool(cfg.Name, info, client)
		fullName := tool.FullName()

		// 检查是否有命名冲突
		if existing := m.registry.Get(fullName); existing != nil {
			log.Printf("[MCP] tool %q from server %q conflicts with existing tool, skipping", fullName, cfg.Name)
			continue
		}

		m.tools[fullName] = tool
		state.Tools = append(state.Tools, tool)
		m.registry.Register(tool)
	}

	log.Printf("[MCP] server %q connected with %d tools", cfg.Name, len(toolInfos))
}

// Stop 关闭所有 MCP 连接并注销工具
func (m *Manager) Stop() {
	m.cancel()

	m.mu.Lock()
	defer m.mu.Unlock()

	for fullName, tool := range m.tools {
		m.registry.Unregister(fullName)
		_ = tool
	}
	m.tools = make(map[string]*Tool)

	for name, client := range m.clients {
		if err := client.Close(); err != nil {
			log.Printf("[MCP] error closing server %q: %v", name, err)
		}
	}
	m.clients = make(map[string]Client)
	m.servers = make(map[string]*serverState)

	log.Printf("[MCP] all servers stopped")
}

// AddServer 动态添加一个 MCP server
func (m *Manager) AddServer(cfg ServerConfig) error {
	m.mu.RLock()
	_, exists := m.servers[cfg.Name]
	m.mu.RUnlock()
	if exists {
		return fmt.Errorf("server %q already exists", cfg.Name)
	}
	m.connectServer(cfg)
	return nil
}

// RemoveServer 动态移除一个 MCP server
func (m *Manager) RemoveServer(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.servers[name]
	if !ok {
		return fmt.Errorf("server %q not found", name)
	}

	// 注销工具
	for _, tool := range state.Tools {
		m.registry.Unregister(tool.FullName())
		delete(m.tools, tool.FullName())
	}

	// 关闭连接
	if state.Client != nil {
		state.Client.Close()
	}
	delete(m.clients, name)
	delete(m.servers, name)

	log.Printf("[MCP] server %q removed", name)
	return nil
}

// Tools 返回所有已注册的 MCP 工具
func (m *Manager) Tools() []*Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]*Tool, 0, len(m.tools))
	for _, t := range m.tools {
		out = append(out, t)
	}
	return out
}

// Servers 返回所有 server 的状态摘要
func (m *Manager) Servers() map[string]*serverState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make(map[string]*serverState, len(m.servers))
	for k, v := range m.servers {
		out[k] = v
	}
	return out
}
