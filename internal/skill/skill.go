package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Scope 表示 skill 的来源层级，高优先级覆盖低优先级
type Scope string

const (
	ScopeProject Scope = "project"
	ScopeGlobal  Scope = "global"
	ScopeBuiltin Scope = "builtin"
)

// RunAs 表示 skill 的执行模式
type RunAs string

const (
	RunInline   RunAs = "inline"
	RunSubagent RunAs = "subagent"
)

// Skill 是一个可调用的技能定义
type Skill struct {
	Name        string
	Description string
	Body        string // markdown body（不含 frontmatter）
	Scope       Scope
	Path        string // 文件绝对路径；builtin 为 "(builtin)"
	RunAs       RunAs
	Model       string   // 可选的模型覆盖（预留，v1 不使用）
	AllowedTools []string // 可选的工具白名单（预留，v1 不使用）
}

// ConventionDirs 是跨工具兼容的约定目录名
var ConventionDirs = []string{".coding-agent", ".agents", ".claude"}

// reservedNames 不允许作为 skill 名称（与 REPL 内置命令冲突）
var reservedNames = map[string]bool{
	"help": true, "reset": true, "exit": true, "quit": true,
	"history": true, "tools": true, "hooks": true, "skills": true,
}

var validNameRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// IsValidSkillName 检查 skill 名称是否合法
func IsValidSkillName(name string) bool {
	return validNameRe.MatchString(name) && !reservedNames[strings.ToLower(name)]
}

// Store 管理 skill 的发现、注册和检索
type Store struct {
	mu      sync.RWMutex
	skills  map[string]Skill
	workdir string
	homeDir string
}

// StoreOptions 配置 Store 的发现行为
type StoreOptions struct {
	Workdir string // 项目根目录
	HomeDir string // 用户主目录；空字符串自动检测
}

// NewStore 创建并初始化 Store，执行一次文件系统扫描
func NewStore(opts StoreOptions) *Store {
	home := opts.HomeDir
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	s := &Store{
		skills:  make(map[string]Skill),
		workdir: opts.Workdir,
		homeDir: home,
	}
	s.scan()
	return s
}

// List 返回已注册的 skill 列表（按 name 排序，prompt-cache 友好）
func (s *Store) List() []Skill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Skill, 0, len(s.skills))
	for _, sk := range s.skills {
		out = append(out, sk)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get 按名称查找 skill；不存在返回 nil
func (s *Store) Get(name string) *Skill {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sk, ok := s.skills[name]
	if !ok {
		return nil
	}
	return &sk
}

// Install 创建/覆盖一个 skill 到 project 级目录，并更新内存注册表
//
// 保存路径：<workdir>/.coding-agent/skills/<name>/SKILL.md
func (s *Store) Install(name, content string) (string, error) {
	if !IsValidSkillName(name) {
		return "", fmt.Errorf("非法的 skill 名称: %q", name)
	}

	dir := filepath.Join(s.workdir, ".coding-agent", "skills", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}

	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}

	sk, err := loadSkillFile(path, ScopeProject)
	if err != nil {
		return path, fmt.Errorf("解析新 skill 失败: %w", err)
	}

	s.mu.Lock()
	s.skills[sk.Name] = sk
	s.mu.Unlock()

	return path, nil
}

// Refresh 重新扫描所有发现路径，更新内存注册表
func (s *Store) Refresh() {
	s.mu.Lock()
	s.skills = make(map[string]Skill)
	s.mu.Unlock()
	s.scan()
}

// scan 执行实际的文件系统扫描
func (s *Store) scan() {
	roots := s.roots()
	for _, root := range roots {
		skills := scanDir(root.Path, root.Scope)
		s.mu.Lock()
		for _, sk := range skills {
			if _, exists := s.skills[sk.Name]; !exists {
				s.skills[sk.Name] = sk
			}
		}
		s.mu.Unlock()
	}

	// 最后注册内置 skill（优先级最低）
	for _, sk := range builtinSkills() {
		s.mu.Lock()
		if _, exists := s.skills[sk.Name]; !exists {
			s.skills[sk.Name] = sk
		}
		s.mu.Unlock()
	}
}

// Root 表示一个扫描根目录
type Root struct {
	Path  string
	Scope Scope
}

// roots 构建扫描根目录列表（按优先级排序）
func (s *Store) roots() []Root {
	var roots []Root

	// 1. Project 级：<workdir>/<convention>/skills/
	if s.workdir != "" {
		for _, conv := range ConventionDirs {
			dir := filepath.Join(s.workdir, conv, "skills")
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				roots = append(roots, Root{Path: dir, Scope: ScopeProject})
			}
		}
	}

	// 2. Global 级：~/<convention>/skills/
	if s.homeDir != "" {
		for _, conv := range ConventionDirs {
			dir := filepath.Join(s.homeDir, conv, "skills")
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				roots = append(roots, Root{Path: dir, Scope: ScopeGlobal})
			}
		}
	}

	return roots
}

// scanDir 扫描一个 skill 根目录，返回发现的 skill 列表
//
// 支持两种布局：
//   - 目录布局：<root>/<name>/SKILL.md
//   - 扁平布局：<root>/<name>.md
func scanDir(root string, scope Scope) []Skill {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}

	var skills []Skill
	for _, entry := range entries {
		name := entry.Name()

		if entry.IsDir() {
			// 目录布局
			skillFile := filepath.Join(root, name, "SKILL.md")
			if sk, err := loadSkillFile(skillFile, scope); err == nil {
				skills = append(skills, sk)
			}
			continue
		}

		// 扁平布局：<name>.md
		if strings.HasSuffix(strings.ToLower(name), ".md") && !strings.EqualFold(name, "README.md") {
			skillFile := filepath.Join(root, name)
			if sk, err := loadSkillFile(skillFile, scope); err == nil {
				skills = append(skills, sk)
			}
		}
	}
	return skills
}

// loadSkillFile 读取并解析一个 SKILL.md 文件
func loadSkillFile(path string, scope Scope) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}

	meta, body := ParseFrontmatter(string(data))

	// 推断 name：frontmatter > 目录名 > 文件名（去 .md）
	name := meta["name"]
	if name == "" {
		dir := filepath.Dir(path)
		base := filepath.Base(dir)
		if base == "." || strings.EqualFold(filepath.Base(path), "SKILL.md") {
			name = base
		} else {
			name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
	}

	// 在 Windows 上统一路径分隔符
	absPath := path
	if abs, err := filepath.Abs(path); err == nil {
		absPath = abs
	}
	if runtime.GOOS == "windows" {
		absPath = filepath.FromSlash(absPath)
	}

	runAs := RunInline
	if strings.EqualFold(meta["runAs"], "subagent") || strings.EqualFold(meta["context"], "fork") {
		runAs = RunSubagent
	}

	return Skill{
		Name:        name,
		Description: meta["description"],
		Body:        body,
		Scope:       scope,
		Path:        absPath,
		RunAs:       runAs,
		Model:       meta["model"],
	}, nil
}
