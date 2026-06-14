package memory

// Type 表示记忆的类型
type Type string

const (
	// TypeUser 用户身份、偏好、专长（跨项目全局共享）
	TypeUser Type = "user"
	// TypeFeedback 工作方式指导、经验教训（跨项目全局共享）
	TypeFeedback Type = "feedback"
	// TypeProject 进行中的工作、目标、约束、项目事实（项目专属）
	TypeProject Type = "project"
	// TypeReference 外部资源指针：URL、工单号、文档链接（项目专属）
	TypeReference Type = "reference"
)

// IsGlobal 判断该类型是否应存储到全局目录（跨项目共享）
func (t Type) IsGlobal() bool {
	return t == TypeUser || t == TypeFeedback
}

// IsProject 判断该类型是否应存储到项目专属目录
func (t Type) IsProject() bool {
	return t == TypeProject || t == TypeReference
}

// Scope 表示层级文档的来源层级，优先级从低到高
type Scope string

const (
	ScopeUser    Scope = "user"    // ~/.coding-agent/AGENTS.md
	ScopeAncestor Scope = "ancestor" // Git 根目录的 AGENTS.md
	ScopeProject Scope = "project"  // 项目根目录的 AGENTS.md（共享）
	ScopeLocal   Scope = "local"   // 项目根目录的 AGENTS.local.md（个人，git-ignored）
)

// Source 一个已加载的层级文档
type Source struct {
	Path  string
	Scope Scope
	Body  string // @import 展开后的内容
}

// Memory 一个已保存的记忆事实
type Memory struct {
	Name        string // kebab-case slug，也是文件名（不含 .md）
	Title       string // 人类可读标签（用于索引显示）
	Description string // 单行摘要（用于索引和搜索）
	Type        Type
	Body        string // Markdown 正文
}

// Set 一次会话加载的全部内存
type Set struct {
	Docs    []Source // 层级文档（按优先级升序）
	Store   *Store   // 自动记忆存储
	Index   string   // 加载时的 MEMORY.md 内容
	CWD     string   // 项目工作目录
	UserDir string   // 用户配置根目录
}
