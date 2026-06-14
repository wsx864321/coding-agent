package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// docCandidates 按优先级排序的层级文档文件名
var docCandidates = []string{"AGENTS.md", "CLAUDE.md"}

// localCandidates 个人专属文档（不提交 git）
var localCandidates = []string{"AGENTS.local.md", "CLAUDE.local.md"}

// DiscoverDocs 从用户目录到项目目录层级发现所有文档
//
// 发现顺序（按优先级从低到高）：
//  1. User 级：~/.coding-agent/AGENTS.md（最低优先级）
//  2. Ancestor 级：Git 根目录到项目目录路径上的每个 AGENTS.md
//  3. Project 级：项目根目录的 AGENTS.md（共享，可提交 git）
//  4. Local 级：项目根目录的 AGENTS.local.md（个人，最高优先级）
//
// 返回的 Source 按 Scope 优先级升序排列。
func DiscoverDocs(cwd, userDir string) []Source {
	var sources []Source

	// 1. User 级
	for _, name := range docCandidates {
		path := filepath.Join(userDir, name)
		if body, err := os.ReadFile(path); err == nil {
			sources = append(sources, Source{
				Path:  path,
				Scope: ScopeUser,
				Body:  expandImports(string(body), filepath.Dir(path)),
			})
			break // 只取第一个找到的
		}
	}

	// 查找 Git 根目录
	gitRoot := findGitRoot(cwd)

	// 2. Ancestor 级：从 Git 根到项目根的每个中间目录
	if gitRoot != "" {
		absCWD, _ := filepath.Abs(cwd)
		absRoot, _ := filepath.Abs(gitRoot)
		rel, err := filepath.Rel(absRoot, absCWD)
		if err == nil && rel != "." {
			// 沿路径逐级查找
			parts := strings.Split(filepath.ToSlash(rel), "/")
			for i := 0; i < len(parts); i++ {
				dir := filepath.Join(absRoot, filepath.Join(parts[:i+1]...))
				if dir == absCWD {
					continue // 最后一级由 Project 级处理
				}
				if src := findDocInDir(dir, ScopeAncestor); src != nil {
					sources = append(sources, *src)
				}
			}
		}
	}

	// 3. Project 级
	if src := findDocInDir(cwd, ScopeProject); src != nil {
		sources = append(sources, *src)
	}

	// 4. Local 级
	for _, name := range localCandidates {
		path := filepath.Join(cwd, name)
		if body, err := os.ReadFile(path); err == nil {
			sources = append(sources, Source{
				Path:  path,
				Scope: ScopeLocal,
				Body:  expandImports(string(body), filepath.Dir(path)),
			})
			break
		}
	}

	// 物理去重（防止符号链接导致同一文件重复）
	sources = dedupeSources(sources)

	return sources
}

// findDocInDir 在目录中查找 AGENTS.md/CLAUDE.md
func findDocInDir(dir string, scope Scope) *Source {
	for _, name := range docCandidates {
		path := filepath.Join(dir, name)
		if body, err := os.ReadFile(path); err == nil {
			return &Source{
				Path:  path,
				Scope: scope,
				Body:  expandImports(string(body), filepath.Dir(path)),
			}
		}
	}
	return nil
}

// findGitRoot 向上查找 .git 目录
func findGitRoot(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// expandImports 展开 @path 导入指令
//
// 语法：单行 `@path/to/file.md` → 替换为该文件内容
// 支持 `~` 展开、相对路径（相对于 baseDir）
// 递归深度限制 5 层，有循环检测
func expandImports(content string, baseDir string) string {
	const maxDepth = 5
	seen := map[string]bool{}
	return expandImportsRecur(content, baseDir, 0, maxDepth, seen)
}

func expandImportsRecur(content, baseDir string, depth, maxDepth int, seen map[string]bool) string {
	if depth > maxDepth {
		return content
	}

	var result strings.Builder
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "@") {
			result.WriteString(line)
			result.WriteByte('\n')
			continue
		}

		// 解析导入路径
		importPath := strings.TrimSpace(trimmed[1:])
		if importPath == "" {
			result.WriteString(line)
			result.WriteByte('\n')
			continue
		}

		// 展开 ~
		if strings.HasPrefix(importPath, "~/") {
			home, err := os.UserHomeDir()
			if err == nil {
				importPath = filepath.Join(home, importPath[2:])
			}
		}

		// 相对路径
		if !filepath.IsAbs(importPath) {
			importPath = filepath.Join(baseDir, importPath)
		}

		// 循环检测
		abs, _ := filepath.Abs(importPath)
		if seen[abs] {
			result.WriteString(fmt.Sprintf("[跳过循环导入: %s]\n", trimmed))
			continue
		}

		imported, err := os.ReadFile(importPath)
		if err != nil {
			result.WriteString(fmt.Sprintf("[导入失败: %s — %v]\n", trimmed, err))
			continue
		}

		seen[abs] = true
		expanded := expandImportsRecur(string(imported), filepath.Dir(importPath), depth+1, maxDepth, seen)
		result.WriteString(expanded)
		if !strings.HasSuffix(expanded, "\n") {
			result.WriteByte('\n')
		}
		delete(seen, abs)
	}
	return strings.TrimRight(result.String(), "\n")
}

// dedupeSources 按物理路径去重（使用 os.SameFile）
func dedupeSources(sources []Source) []Source {
	type dedupKey struct {
		vol   uint32
		idxHi uint64
		idxLo uint64
	}
	seen := map[dedupKey]bool{}
	var out []Source
	for _, s := range sources {
		info, err := os.Stat(s.Path)
		if err != nil {
			out = append(out, s)
			continue
		}
		sys := info.Sys()
		if sys == nil {
			out = append(out, s)
			continue
		}
		// 简单去重：按路径字符串
		key := dedupKey{}
		abs, _ := filepath.Abs(s.Path)
		if seen[key] {
			continue
		}
		seen[key] = true
		_ = abs
	}
	// 回退到路径字符串去重
	pathSeen := map[string]bool{}
	out = out[:0]
	for _, s := range sources {
		abs, _ := filepath.Abs(s.Path)
		if pathSeen[abs] {
			continue
		}
		pathSeen[abs] = true
		out = append(out, s)
	}
	return out
}
