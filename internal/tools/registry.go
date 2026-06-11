package tools

import (
	"sort"
	"sync"
)

type Registry struct {
	mu    *sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{
		mu:    &sync.RWMutex{},
		tools: make(map[string]Tool),
	}
}

func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name()] = tool
}

func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
}

func (r *Registry) Get(name string) Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
}

func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}

	// 对tools按照Name进行排序，prompt cache友好
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name() < tools[j].Name()
	})

	return tools
}

// FilterRegistry 从 parent 构建子注册表：复制所有工具，但排除 exclude 中列出的名称。
//
// 典型用途：为 subagent 构建工具集，排除 meta 工具（task / todo_write / complete_step）
// 防止递归 spawn 和状态泄漏。
//
// 返回的 Registry 是独立副本，对其修改不影响 parent。
func FilterRegistry(parent *Registry, exclude ...string) *Registry {
	ex := make(map[string]bool, len(exclude))
	for _, name := range exclude {
		ex[name] = true
	}

	child := NewRegistry()
	parent.mu.RLock()
	defer parent.mu.RUnlock()
	for name, tool := range parent.tools {
		if !ex[name] {
			child.tools[name] = tool
		}
	}
	return child
}
