package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Options 配置 memory 加载行为
type Options struct {
	// CWD 项目工作目录（必填）
	CWD string
	// UserDir 用户配置根目录，一般为 ~/.coding-agent
	// 为空时自动检测
	UserDir string
	// Workdir 项目根目录（用于分桶），为空时使用 CWD
	Workdir string
}

// DefaultMemoryDir 返回默认的 memory 根目录
func DefaultMemoryDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ".coding-agent/memory"
	}
	return filepath.Join(home, ".coding-agent", "memory")
}

// ResolveUserDir 解析用户配置根目录
func ResolveUserDir(raw string) string {
	if raw != "" {
		return raw
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ".coding-agent"
	}
	return filepath.Join(home, ".coding-agent")
}

// Load 加载全部 memory：层级文档 + 自动记忆存储索引
//
// 返回的 Set 可直接用于 Compose 注入 system prompt。
func Load(opts Options) *Set {
	userDir := ResolveUserDir(opts.UserDir)
	cwd := opts.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	workdir := opts.Workdir
	if workdir == "" {
		workdir = cwd
	}

	// 1. 发现层级文档
	docs := DiscoverDocs(cwd, userDir)

	// 2. 初始化 Store 并读取索引
	store := NewStore(StoreOptions{
		CWD:     cwd,
		Workdir: workdir,
		UserDir: userDir,
	})
	index := store.Index()

	return &Set{
		Docs:    docs,
		Store:   store,
		Index:   index,
		CWD:     cwd,
		UserDir: userDir,
	}
}

// Compose 将 memory 块追加到 system prompt 后面
//
// 布局：
//
//	[base system prompt]        ← 最稳定，可作为缓存前缀
//	[层级文档 memory block]      ← 会话间可能变化
//	[MEMORY.md 索引]             ← 会话间可能变化
//
// 使用说明：即使中会话有新增记忆，也不修改 system prompt（保护前缀缓存），
// 而是通过 Queue 将变更前置到下一轮 user 消息。
func Compose(basePrompt string, s *Set) string {
	var b strings.Builder
	b.WriteString(basePrompt)

	// 追加层级文档
	if len(s.Docs) > 0 {
		b.WriteString("\n\n")
		for i, doc := range s.Docs {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(fmt.Sprintf("<doc scope=%q path=%q>\n", doc.Scope, doc.Path))
			b.WriteString(doc.Body)
			b.WriteString("\n</doc>")
		}
	}

	// 追加 MEMORY.md 索引
	if s.Index != "" {
		b.WriteString("\n\n<memory-index>\n")
		b.WriteString("已保存的记忆（使用 recall/search 工具获取完整内容）：\n\n")
		b.WriteString(s.Index)
		b.WriteString("\n</memory-index>")
	}

	return b.String()
}

// BlockOnly 只返回 memory 部分（不含 base prompt），供增量拼接
func BlockOnly(s *Set) string {
	if s == nil {
		return ""
	}
	return Compose("", s)
}
