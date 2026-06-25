package lsp

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LanguageConfig 描述一个语言服务器
type LanguageConfig struct {
	Name        string   // 用户可读名称
	Extensions  []string // 特征文件扩展名（如 ".go", "go.mod"）
	Files       []string // 特征文件名（如 "tsconfig.json", "Cargo.toml"）
	Command     string   // LSP server 可执行文件名
	Args        []string // 命令行参数
	InstallHint string   // 安装提示
}

// 内置支持的语言服务器列表
var defaultLanguages = []LanguageConfig{
	{
		Name:       "Go",
		Extensions: []string{".go"},
		Files:      []string{"go.mod", "go.work"},
		Command:    "gopls",
		InstallHint: "go install golang.org/x/tools/gopls@latest",
	},
	{
		Name:       "TypeScript",
		Extensions: []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs"},
		Files:      []string{"tsconfig.json", "jsconfig.json", "package.json"},
		Command:    "typescript-language-server",
		Args:       []string{"--stdio"},
		InstallHint: "npm install -g typescript-language-server typescript",
	},
	{
		Name:       "Python",
		Extensions: []string{".py"},
		Files:      []string{"pyproject.toml", "requirements.txt", "setup.py", "setup.cfg"},
		Command:    "pyright-langserver",
		Args:       []string{"--stdio"},
		InstallHint: "pip install pyright",
	},
	{
		Name:       "Rust",
		Extensions: []string{".rs"},
		Files:      []string{"Cargo.toml"},
		Command:    "rust-analyzer",
		InstallHint: "rustup component add rust-analyzer",
	},
}

// Manager 管理多语言 LSP server 的生命周期
type Manager struct {
	rootPath  string
	languages []LanguageConfig
	logger    *log.Logger

	mu      sync.RWMutex
	clients map[string]*Client // language name → client
}

// NewManager 创建一个 LSP Manager
func NewManager(rootPath string) *Manager {
	return &Manager{
		rootPath:  rootPath,
		languages: defaultLanguages,
		clients:   make(map[string]*Client),
		logger:    log.Default(),
	}
}

// SetLogger 设置日志记录器；nil 恢复默认
func (m *Manager) SetLogger(l *log.Logger) {
	if l == nil {
		l = log.Default()
	}
	m.logger = l
}

// SetLanguages 覆盖默认语言列表（用于测试）
func (m *Manager) SetLanguages(langs []LanguageConfig) {
	m.languages = langs
}

// Start 检测项目语言并尝试启动对应的 LSP server
//
// 对每个检测到的语言异步启动 server，失败不阻塞。
func (m *Manager) Start() {
	detected := m.detect()
	if len(detected) == 0 {
		m.logger.Printf("[LSP] no supported language detected in %s", m.rootPath)
		return
	}

	var wg sync.WaitGroup
	for _, lang := range detected {
		lang := lang
		wg.Add(1)
		go func() {
			defer wg.Done()

			if !m.isCommandAvailable(lang.Command) {
				m.logger.Printf("[LSP] %s detected but %s not found in PATH", lang.Name, lang.Command)
				m.logger.Printf("[LSP] %s install: %s", lang.Name, lang.InstallHint)
				return
			}

			client, err := NewClient(lang.Command, lang.Args, m.rootPath)
			if err != nil {
				m.logger.Printf("[LSP] %s server (%s) failed to start: %v", lang.Name, lang.Command, err)
				return
			}

			m.mu.Lock()
			m.clients[lang.Name] = client
			m.mu.Unlock()

			m.logger.Printf("[LSP] %s server started (%s)", lang.Name, lang.Command)
		}()
	}
	wg.Wait()
}

// Stop 关闭所有 LSP 连接
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, client := range m.clients {
		if err := client.Close(); err != nil {
			m.logger.Printf("[LSP] error closing %s: %v", name, err)
		}
	}
	m.clients = make(map[string]*Client)
	m.logger.Printf("[LSP] all servers stopped")
}

// ClientForFile 返回处理指定文件的语言客户端
func (m *Manager) ClientForFile(file string) (*Client, string) {
	ext := strings.ToLower(filepath.Ext(file))

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, lang := range m.languages {
		for _, lext := range lang.Extensions {
			if ext == lext {
				if c, ok := m.clients[lang.Name]; ok {
					return c, lang.Name
				}
				break
			}
		}
	}
	return nil, ""
}

// ClientByName 按语言名获取客户端
func (m *Manager) ClientByName(name string) *Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.clients[name]
}

// IsAvailable 检查是否有可用 LSP server
func (m *Manager) IsAvailable() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.clients) > 0
}

// detect 根据项目文件结构检测语言（不检查命令是否可执行）
func (m *Manager) detect() []LanguageConfig {
	var detected []LanguageConfig

	for _, lang := range m.languages {
		if m.isLanguagePresent(lang) {
			detected = append(detected, lang)
		}
	}
	return detected
}

// isLanguagePresent 检查项目是否包含指定语言的文件
func (m *Manager) isLanguagePresent(lang LanguageConfig) bool {
	// 检查特征文件
	for _, f := range lang.Files {
		if _, err := os.Stat(filepath.Join(m.rootPath, f)); err == nil {
			return true
		}
	}

	// 检查特征扩展名（浅扫描，最多 200 个文件）
	found := false
	_ = filepath.WalkDir(m.rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found {
			return filepath.SkipDir
		}
		// 跳过隐藏目录和 vendor
		name := d.Name()
		if d.IsDir() && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == ".git") {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(name))
		for _, lext := range lang.Extensions {
			if ext == lext {
				found = true
				return filepath.SkipAll
			}
		}
		return nil
	})

	return found
}

// isCommandAvailable 检查命令是否在 PATH 中
func (m *Manager) isCommandAvailable(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// ---------- 方法代理 ----------

// Definition 对文件路径自动选择正确的 LSP server
func (m *Manager) Definition(file string, line, character int) ([]Location, error) {
	client, lang := m.ClientForFile(file)
	if client == nil {
		return nil, fmt.Errorf("no LSP server available for %s", filepath.Ext(file))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = lang
	return client.Definition(ctx, file, line, character)
}

// References 对文件路径自动选择正确的 LSP server
func (m *Manager) References(file string, line, character int) ([]Location, error) {
	client, _ := m.ClientForFile(file)
	if client == nil {
		return nil, fmt.Errorf("no LSP server available for %s", filepath.Ext(file))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return client.References(ctx, file, line, character)
}

// Hover 对文件路径自动选择正确的 LSP server
func (m *Manager) Hover(file string, line, character int) (*Hover, error) {
	client, _ := m.ClientForFile(file)
	if client == nil {
		return nil, fmt.Errorf("no LSP server available for %s", filepath.Ext(file))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return client.Hover(ctx, file, line, character)
}

// DocumentSymbols 对文件路径自动选择正确的 LSP server
func (m *Manager) DocumentSymbols(file string) ([]DocumentSymbol, error) {
	client, _ := m.ClientForFile(file)
	if client == nil {
		return nil, fmt.Errorf("no LSP server available for %s", filepath.Ext(file))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return client.DocumentSymbols(ctx, file)
}

// WorkspaceSymbols 使用第一个可用的 LSP server
func (m *Manager) WorkspaceSymbols(query string) ([]SymbolInformation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, client := range m.clients {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		syms, err := client.WorkspaceSymbols(ctx, query)
		if err == nil {
			return syms, nil
		}
	}
	return nil, fmt.Errorf("no LSP server available for workspace symbols")
}

// GetDiagnostics 获取文件的诊断信息
func (m *Manager) GetDiagnostics(file string) ([]Diagnostic, error) {
	client, _ := m.ClientForFile(file)
	if client == nil {
		return nil, fmt.Errorf("no LSP server available for %s", filepath.Ext(file))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return client.ForceDiagnostics(ctx, file)
}
