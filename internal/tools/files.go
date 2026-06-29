package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// =====================================================================
// ReadFileTool —— 读取单个文件
// =====================================================================

// ReadFileTool 读取一个文本文件并返回其内容
type ReadFileTool struct {
	// AllowedDirs 允许读取的目录白名单；为空表示不限制
	AllowedDirs []string
	// MaxBytes 单次读取的最大字节数；0 表示不限制
	MaxBytes int
}

// NewReadFileTool 创建具有默认配置的 ReadFileTool
//
// 参数 workdir 是 AllowedDirs 白名单的基准目录：
//   - 传非空路径：AllowedDirs = []string{workdir}，LLM 只能读该目录下文件
//   - 传 ""：AllowedDirs = nil，不限制
//
// 推荐用法（CLI 入口）：
//
//	wd, _ := os.Getwd()
//	tool := tools.NewReadFileTool(wd)
//
// 如需自定义白名单，可继续覆盖：
//
//	tool.AllowedDirs = []string{"/path/a", "/path/b"}
//	tool.AllowedDirs = nil
func NewReadFileTool(workdir string) *ReadFileTool {
	return &ReadFileTool{
		AllowedDirs: allowedDirsFromWorkdir(workdir),
		MaxBytes:    10 * 1024 * 1024, // 默认 10MB
	}
}

// ReadOnly 纯读文件，可并行
func (t *ReadFileTool) ReadOnly() bool { return true }

// Name 返回工具名称
func (t *ReadFileTool) Name() string { return "read_file" }

// Description 返回工具功能描述
func (t *ReadFileTool) Description() string {
	return "读取单个文本文件的内容。返回完整文件内容，编码假设为 UTF-8。" +
		"受 AllowedDirs 白名单限制。"
}

// readFileArgs read_file 的入参
type readFileArgs struct {
	Path  string `json:"path"`
	Start int    `json:"start,omitempty"` // 起始行号（1-based），0 表示从头开始
	End   int    `json:"end,omitempty"`   // 结束行号（1-based，闭区间），0 表示到文件末尾
}

// Schema 返回工具 JSON Schema
func (t *ReadFileTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "待读取文件的相对或绝对路径",
			},
			"start": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"description": "起始行号（1-based，包含），缺省为 1",
			},
			"end": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"description": "结束行号（1-based，包含），缺省为文件末尾",
			},
		},
		"required": []string{"path"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

// Execute 读取文件
func (t *ReadFileTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	var p readFileArgs
	if err := decodeArgs(args, &p); err != nil {
		return "", err
	}
	if strings.TrimSpace(p.Path) == "" {
		return "", errors.New("path 不能为空")
	}
	if err := t.checkPath(p.Path); err != nil {
		return "", err
	}

	data, err := os.ReadFile(p.Path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}
	if t.MaxBytes > 0 && len(data) > t.MaxBytes {
		return "", fmt.Errorf("文件大小 %d 超过限制 %d", len(data), t.MaxBytes)
	}

	// 行号范围过滤
	if p.Start > 0 || p.End > 0 {
		lines := bytes.Split(data, []byte("\n"))
		// 去掉因文件末尾换行产生的空行
		if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
			lines = lines[:len(lines)-1]
		}
		start := p.Start
		if start < 1 {
			start = 1
		}
		end := p.End
		if end < 1 || end > len(lines) {
			end = len(lines)
		}
		if start > end {
			return "", fmt.Errorf("start (%d) > end (%d)", start, end)
		}
		if start > len(lines) {
			return "", fmt.Errorf("start (%d) 超过文件总行数 %d", start, len(lines))
		}
		var buf bytes.Buffer
		for i := start - 1; i < end; i++ {
			buf.Write(lines[i])
			if i < end-1 {
				buf.WriteByte('\n')
			}
		}
		return buf.String(), nil
	}

	return string(data), nil
}

// checkPath 校验路径是否在白名单内
func (t *ReadFileTool) checkPath(path string) error {
	if len(t.AllowedDirs) == 0 {
		return nil
	}
	ok, err := isInAllowedDirs(path, t.AllowedDirs)
	if err != nil {
		return fmt.Errorf("校验路径失败: %w", err)
	}
	if !ok {
		return fmt.Errorf("path %q 不在允许的目录白名单中", path)
	}
	return nil
}

// =====================================================================
// WriteFileTool —— 写入或追加文件
// =====================================================================

// WriteFileTool 写入文本到文件，支持覆盖与追加两种模式
type WriteFileTool struct {
	AllowedDirs []string
}

// NewWriteFileTool 创建具有默认配置的 WriteFileTool
//
// 参数 workdir 是 AllowedDirs 白名单的基准目录，语义同 NewReadFileTool。
func NewWriteFileTool(workdir string) *WriteFileTool {
	return &WriteFileTool{
		AllowedDirs: allowedDirsFromWorkdir(workdir),
	}
}

// ReadOnly 写文件有副作用，不可并行
func (t *WriteFileTool) ReadOnly() bool { return false }

// Name 返回工具名称
func (t *WriteFileTool) Name() string { return "write_file" }

// Description 返回工具功能描述
func (t *WriteFileTool) Description() string {
	return "将文本写入文件。默认覆盖已有内容；append=true 时追加到末尾。" +
		"会自动创建不存在的父目录。受 AllowedDirs 白名单限制。"
}

// writeFileArgs write_file 的入参
type writeFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Append  bool   `json:"append,omitempty"`
}

// Schema 返回 JSON Schema
func (t *WriteFileTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "目标文件路径",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "待写入的文本内容",
			},
			"append": map[string]any{
				"type":        "boolean",
				"description": "是否追加到文件末尾，默认 false（覆盖）",
			},
		},
		"required": []string{"path", "content"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

// Execute 写入文件
func (t *WriteFileTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	var p writeFileArgs
	if err := decodeArgs(args, &p); err != nil {
		return "", err
	}
	if strings.TrimSpace(p.Path) == "" {
		return "", errors.New("path 不能为空")
	}
	if err := t.checkPath(p.Path); err != nil {
		return "", err
	}

	// 父目录自动创建
	dir := filepath.Dir(p.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建父目录失败: %w", err)
	}

	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if p.Append {
		flag = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	}

	f, err := os.OpenFile(p.Path, flag, 0o644)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer f.Close()

	n, err := f.WriteString(p.Content)
	if err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}
	mode := "覆盖"
	if p.Append {
		mode = "追加"
	}
	return fmt.Sprintf("OK: %s写入 %d 字节到 %s", mode, n, p.Path), nil
}

func (t *WriteFileTool) checkPath(path string) error {
	if len(t.AllowedDirs) == 0 {
		return nil
	}
	ok, err := isInAllowedDirs(path, t.AllowedDirs)
	if err != nil {
		return fmt.Errorf("校验路径失败: %w", err)
	}
	if !ok {
		return fmt.Errorf("path %q 不在允许的目录白名单中", path)
	}
	return nil
}

// =====================================================================
// EditFileTool —— find-and-replace 编辑
// =====================================================================

// EditFileTool 在文件中查找文本并替换
type EditFileTool struct {
	AllowedDirs []string
}

// NewEditFileTool 创建默认配置
//
// 参数 workdir 是 AllowedDirs 白名单的基准目录，语义同 NewReadFileTool。
func NewEditFileTool(workdir string) *EditFileTool {
	return &EditFileTool{
		AllowedDirs: allowedDirsFromWorkdir(workdir),
	}
}

// ReadOnly 编辑文件有副作用，不可并行
func (t *EditFileTool) ReadOnly() bool { return false }

// Name 返回工具名称
func (t *EditFileTool) Name() string { return "edit_file" }

// Description 返回工具功能描述
func (t *EditFileTool) Description() string {
	return "在文件中查找 old_text 并替换为 new_text。" +
		"默认 old_text 必须唯一匹配；all=true 时替换所有匹配项。" +
		"受 AllowedDirs 白名单限制。"
}

// editFileArgs edit_file 的入参
type editFileArgs struct {
	Path    string `json:"path"`
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
	All     bool   `json:"all,omitempty"`
}

// Schema 返回 JSON Schema
func (t *EditFileTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "目标文件路径",
			},
			"old_text": map[string]any{
				"type":        "string",
				"description": "待替换的文本片段（必须与文件内容完全一致，包括空白）",
			},
			"new_text": map[string]any{
				"type":        "string",
				"description": "替换后的新文本",
			},
			"all": map[string]any{
				"type":        "boolean",
				"description": "是否替换所有匹配项；默认 false（要求唯一匹配）",
			},
		},
		"required": []string{"path", "old_text", "new_text"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

// Execute 执行编辑
func (t *EditFileTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	var p editFileArgs
	if err := decodeArgs(args, &p); err != nil {
		return "", err
	}
	if strings.TrimSpace(p.Path) == "" {
		return "", errors.New("path 不能为空")
	}
	if p.OldText == "" {
		return "", errors.New("old_text 不能为空")
	}
	if err := t.checkPath(p.Path); err != nil {
		return "", err
	}

	data, err := os.ReadFile(p.Path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}
	content := string(data)
	count := strings.Count(content, p.OldText)
	if count == 0 {
		return "", errors.New("old_text 在文件中未找到")
	}
	if !p.All && count > 1 {
		return "", fmt.Errorf("old_text 在文件中匹配 %d 次，要求唯一匹配；如需全部替换请设置 all=true", count)
	}

	var newContent string
	if p.All {
		newContent = strings.ReplaceAll(content, p.OldText, p.NewText)
	} else {
		// count == 1 已由上面 if 保证
		idx := strings.Index(content, p.OldText)
		newContent = content[:idx] + p.NewText + content[idx+len(p.OldText):]
	}

	if err := os.WriteFile(p.Path, []byte(newContent), 0o644); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}
	return fmt.Sprintf("OK: 替换 %d 处", count), nil
}

func (t *EditFileTool) checkPath(path string) error {
	if len(t.AllowedDirs) == 0 {
		return nil
	}
	ok, err := isInAllowedDirs(path, t.AllowedDirs)
	if err != nil {
		return fmt.Errorf("校验路径失败: %w", err)
	}
	if !ok {
		return fmt.Errorf("path %q 不在允许的目录白名单中", path)
	}
	return nil
}

// =====================================================================
// GlobFileTool —— glob 模式匹配
// =====================================================================

// GlobFileTool 根据 glob 模式查找文件路径
type GlobFileTool struct {
	AllowedDirs []string
}

// NewGlobFileTool 创建默认配置
//
// 参数 workdir 是 AllowedDirs 白名单的基准目录，语义同 NewReadFileTool。
func NewGlobFileTool(workdir string) *GlobFileTool {
	return &GlobFileTool{
		AllowedDirs: allowedDirsFromWorkdir(workdir),
	}
}

// ReadOnly 纯读文件系统元数据，可并行
func (t *GlobFileTool) ReadOnly() bool { return true }

// Name 返回工具名称
func (t *GlobFileTool) Name() string { return "glob_file" }

// Description 返回工具功能描述
func (t *GlobFileTool) Description() string {
	return "在指定目录（默认当前工作目录）下，按 glob 模式查找文件路径。" +
		"支持 `*`/`?`/`[abc]` 简单通配与 `**` 递归匹配。" +
		"返回按路径排序的匹配列表，每行一个。"
}

// globFileArgs glob_file 的入参
type globFileArgs struct {
	Pattern    string `json:"pattern"`
	BaseDir    string `json:"base_dir,omitempty"`
	MaxResults int    `json:"max_results,omitempty"`
}

// Schema 返回 JSON Schema
func (t *GlobFileTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "glob 模式，如 `**/*.go`",
			},
			"base_dir": map[string]any{
				"type":        "string",
				"description": "搜索的根目录，缺省为当前工作目录",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"description": "最多返回的结果数；0 表示不限制",
			},
		},
		"required": []string{"pattern"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

// Execute 执行 glob
func (t *GlobFileTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	var p globFileArgs
	if err := decodeArgs(args, &p); err != nil {
		return "", err
	}
	if strings.TrimSpace(p.Pattern) == "" {
		return "", errors.New("pattern 不能为空")
	}
	base := p.BaseDir
	if base == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("获取工作目录失败: %w", err)
		}
		base = wd
	}
	if err := t.checkPath(base); err != nil {
		return "", err
	}

	// 懒加载超时保护
	dctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	results, err := globMatch(dctx, base, p.Pattern, p.MaxResults)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "(无匹配)", nil
	}
	var buf bytes.Buffer
	for _, r := range results {
		buf.WriteString(r)
		buf.WriteByte('\n')
	}
	return strings.TrimRight(buf.String(), "\n"), nil
}

func (t *GlobFileTool) checkPath(path string) error {
	if len(t.AllowedDirs) == 0 {
		return nil
	}
	ok, err := isInAllowedDirs(path, t.AllowedDirs)
	if err != nil {
		return fmt.Errorf("校验路径失败: %w", err)
	}
	if !ok {
		return fmt.Errorf("base_dir %q 不在允许的目录白名单中", path)
	}
	return nil
}

// globMatch 在 base 下按 pattern 查找匹配路径
//
// 支持的 glob 语法：
//   - `*`         单层匹配除路径分隔符外的任意字符
//   - `**`        跨层匹配（递归）
//   - `?`         单字符
//   - `[abc]`     字符集
//
// 实现策略：先把 pattern 编译为正则，再用 filepath.Walk 遍历。
func globMatch(ctx context.Context, base, pattern string, maxResults int) ([]string, error) {
	re, err := globToRegexp(pattern)
	if err != nil {
		return nil, fmt.Errorf("pattern 解析失败: %w", err)
	}

	var (
		results   []string
		truncated bool
	)
	err = filepath.Walk(base, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // 跳过不可访问的目录
		}
		// 跳过常见的元数据目录
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return nil
		}
		// 统一使用正斜杠进行匹配（Windows 友好）
		rel = filepath.ToSlash(rel)
		if re.MatchString(rel) {
			results = append(results, path)
			if maxResults > 0 && len(results) >= maxResults {
				truncated = true
				return io.EOF // 截断遍历
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}

	sort.Strings(results)
	if truncated {
		results = append(results, fmt.Sprintf("...(已截断，仅显示前 %d 条)", maxResults))
	}
	return results, nil
}

// globToRegexp 将 glob 模式编译为正则表达式
//
// 语法：
//   - `**`        段：匹配任意层目录（含零层）
//   - `*`         匹配除 `/` 外的任意字符序列
//   - `?`         匹配除 `/` 外的单个字符
//   - `[abc]`     字符集
//   - `{a,b,c}`   花括号扩展，匹配其中任一选项（支持嵌套），如 `*.{go,md}`
//   - 其它字符按字面量匹配（自动转义）
func globToRegexp(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	segments := strings.Split(pattern, "/")
	for i, seg := range segments {
		// 相邻的普通段（非 **）之间补回 / 分隔符
		// ** 段无需补，因为 (?:.*/)? 已自带结尾 /
		if i > 0 && seg != "**" && segments[i-1] != "**" {
			b.WriteString("/")
		}
		if seg == "**" {
			// ** 的正则随位置而变：
			//   唯一段 "**"        -> .*           （匹配一切）
			//   开头非最后 "**/foo" -> (?:.*/)?     （可选前缀路径，无前导 /）
			//   结尾非开头 "foo/**" -> (?:/.*)?     （匹配 foo 或 foo/...）
			//   中间 "a/**/b"      -> /(?:.*/)?    （要求 a/ 前导，ab 不应匹配）
			switch {
			case i == 0 && i == len(segments)-1:
				b.WriteString(".*")
			case i == 0:
				b.WriteString("(?:.*/)?")
			case i == len(segments)-1:
				b.WriteString("(?:/.*)?")
			default:
				b.WriteString("/(?:.*/)?")
			}
		} else {
			part, err := segmentToRegexp(seg)
			if err != nil {
				return nil, err
			}
			b.WriteString(part)
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// segmentToRegexp 把单段 glob（不含 `/`）转为正则片段。
// 支持 `*`/`?`/`[abc]`/`{a,b}`，其中花括号可嵌套。
func segmentToRegexp(seg string) (string, error) {
	var b strings.Builder
	j := 0
	for j < len(seg) {
		ch := seg[j]
		switch ch {
		case '*':
			b.WriteString("[^/]*")
			j++
		case '?':
			b.WriteString("[^/]")
			j++
		case '[':
			// 查找匹配的 ]
			k := strings.IndexByte(seg[j:], ']')
			if k < 0 {
				b.WriteString(regexp.QuoteMeta(string(ch)))
				j++
				continue
			}
			inner := seg[j+1 : j+k]
			if inner == "" {
				return "", fmt.Errorf("glob: empty char class at position %d", j)
			}
			cc, err := charClassToRegexp(inner)
			if err != nil {
				return "", err
			}
			b.WriteString(cc)
			j += k + 1
		case '{':
			// 花括号扩展：{a,b,c} -> (?:a|b|c)，支持嵌套
			end := findMatchingBrace(seg, j)
			if end < 0 {
				// 无匹配的 }，按字面量处理
				b.WriteString(regexp.QuoteMeta(string(ch)))
				j++
				continue
			}
			inner := seg[j+1 : end]
			b.WriteString("(?:")
			for k, opt := range splitBraceOptions(inner) {
				if k > 0 {
					b.WriteString("|")
				}
				part, err := segmentToRegexp(opt)
				if err != nil {
					return "", err
				}
				b.WriteString(part)
			}
			b.WriteString(")")
			j = end + 1
		default:
			b.WriteString(regexp.QuoteMeta(string(ch)))
			j++
		}
	}
	return b.String(), nil
}

// charClassToRegexp 将 glob 字符集内部文本转为正则字符集。
// glob 取反用 !（如 [!abc]），转为正则的 ^（[^abc]）。
// 空字符集已在调用方处理。
func charClassToRegexp(inner string) (string, error) {
	var b strings.Builder
	b.WriteString("[")
	var j int
	chars := []rune(inner)
	// 取反：glob 的 [!...] 等价于正则的 [^...]
	if len(chars) > 0 && chars[0] == '!' {
		b.WriteString("^")
		j = 1
	}
	for j < len(chars) {
		ch := chars[j]
		if ch == ']' {
			b.WriteString("\\]")
		} else {
			b.WriteString(string(ch))
		}
		j++
	}
	b.WriteString("]")
	return b.String(), nil
}

// findMatchingBrace 返回 seg[start] 处 '{' 对应的 '}' 的索引，无匹配返回 -1。
// 用深度计数支持嵌套花括号。
func findMatchingBrace(seg string, start int) int {
	depth := 0
	for i := start; i < len(seg); i++ {
		switch seg[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitBraceOptions 按逗号分割花括号内的选项，忽略嵌套花括号里的逗号。
func splitBraceOptions(s string) []string {
	var opts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				opts = append(opts, s[start:i])
				start = i + 1
			}
		}
	}
	opts = append(opts, s[start:])
	return opts
}

// allowedDirsFromWorkdir 把 NewXxxTool 的 workdir 形参翻译成 AllowedDirs 字段
//
// 规则：
//   - workdir == ""  → nil（不限制）
//   - workdir != ""  → []string{workdir}
//
// 不在内部调 os.Getwd()，让 NewXxxTool 保持"无 I/O"语义。
func allowedDirsFromWorkdir(workdir string) []string {
	if workdir == "" {
		return nil
	}
	return []string{workdir}
}
