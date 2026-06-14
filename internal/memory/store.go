package memory

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// StoreOptions 配置 Store
type StoreOptions struct {
	CWD     string // 当前工作目录
	Workdir string // 项目根目录
	UserDir string // 用户配置根目录
}

// Store 管理自动记忆的文件存储
type Store struct {
	Dir       string // 项目专属记忆目录
	GlobalDir string // 跨项目全局记忆目录
}

// NewStore 创建 Store
func NewStore(opts StoreOptions) *Store {
	userDir := ResolveUserDir(opts.UserDir)
	bucket := projectBucket(opts.Workdir)

	return &Store{
		Dir:       filepath.Join(userDir, "projects", bucket, "memory"),
		GlobalDir: filepath.Join(userDir, "memory", "global"),
	}
}

// DirFor 根据记忆类型返回写入目录
func (s *Store) DirFor(t Type) string {
	if t.IsGlobal() {
		return s.GlobalDir
	}
	return s.Dir
}

// Index 读取并返回 MEMORY.md 内容
//
// 合并 GlobalDir 和 Dir 下的 MEMORY.md（Global 在前，Project 在后）。
func (s *Store) Index() string {
	var parts []string

	// 全局索引
	if idx, err := os.ReadFile(filepath.Join(s.GlobalDir, "MEMORY.md")); err == nil {
		parts = append(parts, strings.TrimSpace(string(idx)))
	}

	// 项目索引
	if idx, err := os.ReadFile(filepath.Join(s.Dir, "MEMORY.md")); err == nil {
		parts = append(parts, strings.TrimSpace(string(idx)))
	}

	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n")
}

// Save 持久化一条记忆
//
// 行为：
//   - 渲染 YAML frontmatter + body → 写入 <name>.md
//   - 更新对应目录的 MEMORY.md 索引
//   - 若类型变更导致目录不同，自动归档旧位置文件
func (s *Store) Save(m Memory) error {
	if m.Name == "" {
		return fmt.Errorf("memory name 不能为空")
	}

	dir := s.DirFor(m.Type)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建记忆目录失败: %w", err)
	}

	// 检查是否需要跨目录迁移（类型变更）
	for _, checkDir := range []string{s.Dir, s.GlobalDir} {
		if checkDir == dir {
			continue
		}
		oldPath := filepath.Join(checkDir, m.Name+".md")
		if _, err := os.Stat(oldPath); err == nil {
			_ = s.archiveFile(oldPath, checkDir)
			_ = s.rebuildIndex(checkDir)
		}
	}

	// 渲染并写入
	content := renderMemoryFile(m)
	path := filepath.Join(dir, m.Name+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("写入记忆文件失败: %w", err)
	}

	// 更新索引
	return s.rebuildIndex(dir)
}

// Load 按名称读取一条记忆的完整内容
func (s *Store) Load(name string) (*Memory, error) {
	for _, dir := range []string{s.Dir, s.GlobalDir} {
		path := filepath.Join(dir, name+".md")
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		return parseMemoryFile(name, string(data)), nil
	}
	return nil, fmt.Errorf("memory %q 不存在", name)
}

// ListActive 返回所有活跃记忆（不含已归档）
func (s *Store) ListActive(filterType Type) []Memory {
	var all []Memory
	for _, dir := range []string{s.GlobalDir, s.Dir} {
		memories, _ := s.listDir(dir)
		for _, m := range memories {
			if filterType == "" || m.Type == filterType {
				all = append(all, m)
			}
		}
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].Name < all[j].Name
	})
	return all
}

// Delete 归档一条记忆（软删除）
//
// 将文件移动到 .archive/<timestamp>-<name>.md，并更新 MEMORY.md 索引。
func (s *Store) Delete(name string) error {
	for _, dir := range []string{s.Dir, s.GlobalDir} {
		path := filepath.Join(dir, name+".md")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		if err := s.archiveFile(path, dir); err != nil {
			return err
		}
		return s.rebuildIndex(dir)
	}
	return fmt.Errorf("memory %q 不存在", name)
}

// SearchText 构建搜索文本（name + title + description + type + body）
func SearchText(m Memory) string {
	return strings.Join([]string{m.Name, m.Title, m.Description, string(m.Type), m.Body}, "\n")
}

// archiveFile 将文件移动到 .archive/ 目录
func (s *Store) archiveFile(path, parentDir string) error {
	archiveDir := filepath.Join(parentDir, ".archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return err
	}
	base := filepath.Base(path)
	archivePath := filepath.Join(archiveDir, fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102-150405.000"), base))
	return os.Rename(path, archivePath)
}

// listDir 列出目录下所有活跃记忆文件
func (s *Store) listDir(dir string) ([]Memory, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var memories []Memory
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if e.Name() == "MEMORY.md" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		mem := parseMemoryFile(name, string(data))
		if mem != nil {
			memories = append(memories, *mem)
		}
	}
	return memories, nil
}

// rebuildIndex 重建指定目录的 MEMORY.md 索引
func (s *Store) rebuildIndex(dir string) error {
	memories, err := s.listDir(dir)
	if err != nil {
		return err
	}

	if len(memories) == 0 {
		// 删除空索引文件
		_ = os.Remove(filepath.Join(dir, "MEMORY.md"))
		return nil
	}

	var b strings.Builder
	b.WriteString("# 记忆索引\n\n")
	for _, m := range memories {
		fmt.Fprintf(&b, "- [%s](%s.md) — %s\n", m.Title, m.Name, m.Description)
	}

	return os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte(b.String()), 0o644)
}

// renderMemoryFile 渲染记忆文件的完整内容（YAML frontmatter + body）
func renderMemoryFile(m Memory) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", m.Name)
	fmt.Fprintf(&b, "title: %s\n", m.Title)
	fmt.Fprintf(&b, "description: %s\n", m.Description)
	fmt.Fprintf(&b, "type: %s\n", m.Type)
	b.WriteString("---\n\n")
	b.WriteString(m.Body)
	return b.String()
}

// parseMemoryFile 从文件内容解析 Memory
func parseMemoryFile(name string, content string) *Memory {
	meta, body := parseSimpleFrontmatter(content)
	if meta == nil {
		return nil
	}

	title := meta["title"]
	if title == "" {
		title = name
	}

	return &Memory{
		Name:        name,
		Title:       title,
		Description: meta["description"],
		Type:        Type(meta["type"]),
		Body:        strings.TrimSpace(body),
	}
}

// parseSimpleFrontmatter 解析 YAML frontmatter（简单实现，无外部依赖）
//
// 格式：
//
//	---
//	key: value
//	---
//	body
func parseSimpleFrontmatter(content string) (map[string]string, string) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return nil, content
	}

	// 查找结束的 ---
	rest := content[3:]
	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return nil, content
	}

	fm := rest[:endIdx]
	body := strings.TrimLeft(rest[endIdx+4:], "\n") // 跳过 \n--- 及后续换行

	meta := make(map[string]string)
	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			// 去除首尾引号
			val = strings.Trim(val, "\"'")
			meta[key] = val
		}
	}
	return meta, body
}

// projectBucket 对项目路径做哈希分桶
func projectBucket(workdir string) string {
	wd := strings.TrimSpace(workdir)
	if wd == "" {
		wd, _ = os.Getwd()
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
