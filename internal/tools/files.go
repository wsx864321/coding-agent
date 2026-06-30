package tools

import (
	"bytes"
	"context"
	"encoding/binary"
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
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// =====================================================================
// ReadFileTool —— 读取单个文件
// =====================================================================

// ReadFileTool 读取一个文本文件并返回其内容，带行号前缀。
type ReadFileTool struct {
	// AllowedDirs 允许读取的目录白名单；为空表示不限制
	AllowedDirs []string
	// MaxBytes 单次读取的最大字节数；0 表示不限制
	MaxBytes int
}

const (
	readFileDefaultLimit  = 2000 // 默认返回行数
	readFileBinaryPeek    = 8 * 1024
	readFileDetectSample  = 256 * 1024
)

// NewReadFileTool 创建具有默认配置的 ReadFileTool
//
// 参数 workdir 是 AllowedDirs 白名单的基准目录：
//   - 传非空路径：AllowedDirs = []string{workdir}，LLM 只能读该目录下文件
//   - 传 ""：AllowedDirs = nil，不限制
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
	return "Read a text file with optional line offset/limit. Output prefixes each line with its 1-based number (e.g. `   42→...`) so subsequent edit_file calls can target exact lines. Use `offset` and `limit` to page through large files; the tool reports total length and pagination hints in a trailer."
}

// readFileArgs read_file 的入参
type readFileArgs struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"` // 0-based 起始行，缺省 0
	Limit  int    `json:"limit,omitempty"`  // 返回行数，缺省 2000
}

// Schema 返回工具 JSON Schema
func (t *ReadFileTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "File path",
			},
			"offset": map[string]any{
				"type":        "integer",
				"minimum":     0,
				"description": "0-based line offset to start reading from (default 0)",
			},
			"limit": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"description": "Maximum lines to return (default 2000)",
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
	p.Path = NormalizeMingwPath(p.Path)
	// 兼容旧的 start/end 参数（支持 float64（JSON 反序列化）和 int（Go 字面量））
	if p.Offset == 0 {
		if start, ok := args["start"].(float64); ok {
			p.Offset = int(start) - 1 // 1-based → 0-based
		} else if start, ok := args["start"].(int); ok {
			p.Offset = start - 1
		}
	}
	if p.Limit == 0 {
		if end, ok := args["end"].(float64); ok {
			p.Limit = int(end) - p.Offset
		} else if end, ok := args["end"].(int); ok {
			p.Limit = end - p.Offset
		}
	}
	if err := t.checkPath(p.Path); err != nil {
		return "", err
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	if p.Limit <= 0 {
		p.Limit = readFileDefaultLimit
	}

	// 目录检查
	if info, err := os.Stat(p.Path); err == nil && info.IsDir() {
		return "", fmt.Errorf("%s 是目录，不是文件 — 用 ls 工具列出内容", p.Path)
	}

	// 大小检查
	if t.MaxBytes > 0 {
		if info, err := os.Stat(p.Path); err == nil && info.Size() > int64(t.MaxBytes) {
			return "", fmt.Errorf("文件大小 %d 超过限制 %d", info.Size(), t.MaxBytes)
		}
	}

	f, err := os.Open(p.Path)
	if err != nil {
		return "", fmt.Errorf("读取 %s: %w", p.Path, err)
	}
	defer f.Close()

	// Peek 前 8KB 做二进制检测
	peek := make([]byte, readFileBinaryPeek)
	pn, perr := io.ReadFull(f, peek)
	peek = peek[:pn]
	peekEOF := perr != nil

	// BOM 检测（UTF-16 含 NUL 是正常的，不能用 NUL 检测误判）
	if enc := detectEncoding(peek); enc != encUTF8 {
		rest, rerr := io.ReadAll(f)
		if rerr != nil {
			return "", fmt.Errorf("读取 %s: %w", p.Path, rerr)
		}
		all := append(peek, rest...)
		return t.scanLines(string(decodeBytes(all, enc)), p.Offset, p.Limit)
	}

	// NUL 字节 → 二进制文件
	if bytes.IndexByte(peek, 0) >= 0 {
		return "", fmt.Errorf("%s 可能是二进制文件（检测到 NUL 字节）", p.Path)
	}

	// 读取样本做编码检测
	head := peek
	if !peekEOF {
		more := make([]byte, readFileDetectSample-len(peek))
		mn, _ := io.ReadFull(f, more)
		head = append(peek, more[:mn]...)
	}
	enc, _ := detectFullEncoding(head)

	src := io.MultiReader(bytes.NewReader(head), f)
	if dec := encodingDecoder(enc); dec != nil {
		return t.scanLines(transformReader(src, dec), p.Offset, p.Limit)
	}
	return t.scanLines(scanReader(src), p.Offset, p.Limit)
}

// scanLines reads lines and formats them with right-aligned 1-based line numbers.
func (t *ReadFileTool) scanLines(content string, offset, limit int) (string, error) {
	lines := strings.SplitAfter(content, "\n")
	// 处理末尾无换行的最后一行
	if len(lines) > 0 && !strings.HasSuffix(lines[len(lines)-1], "\n") {
		// 最后一行不是以 \n 结尾，保持原样
	}
	total := len(lines)
	// 去掉末尾空行（由 strings.SplitAfter 产生）
	if total > 0 && lines[total-1] == "" {
		lines = lines[:total-1]
		total = len(lines)
	}
	if total == 0 {
		return "(空文件)", nil
	}
	if offset >= total {
		return fmt.Sprintf("(offset %d 超过文件总行数 %d)", offset, total), nil
	}

	end := offset + limit
	if end > total {
		end = total
	}
	hasMore := end < total

	shown := lines[offset:end]
	maxLineNo := offset + len(shown)
	w := len(fmt.Sprint(maxLineNo))

	var b strings.Builder
	for i, line := range shown {
		lineNo := offset + i + 1
		// 去掉末尾换行符用于显示
		display := strings.TrimSuffix(line, "\n")
		fmt.Fprintf(&b, "%*d→%s\n", w, lineNo, display)
	}
	if hasMore {
		fmt.Fprintf(&b, "\n[more lines below; pass offset=%d to continue]\n", end)
	}
	return b.String(), nil
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
// 轻量编码处理（内联版，避免独立 package）
// =====================================================================

type fileEncoding int

const (
	encUTF8 fileEncoding = iota
	encUTF8BOM
	encUTF16LE
	encUTF16BE
	encGB18030
	encLossy
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

func detectEncoding(peek []byte) fileEncoding {
	if len(peek) >= 3 && peek[0] == 0xEF && peek[1] == 0xBB && peek[2] == 0xBF {
		return encUTF8BOM
	}
	if len(peek) >= 2 && peek[0] == 0xFF && peek[1] == 0xFE {
		return encUTF16LE
	}
	if len(peek) >= 2 && peek[0] == 0xFE && peek[1] == 0xFF {
		return encUTF16BE
	}
	return encUTF8
}

func detectFullEncoding(data []byte) (fileEncoding, []byte) {
	switch {
	case len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF:
		return encUTF8BOM, data
	case len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE:
		return encUTF16LE, data
	case len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF:
		return encUTF16BE, data
	}
	if utf8.Valid(data) {
		return encUTF8, data
	}
	dec := simplifiedchinese.GB18030.NewDecoder()
	if _, _, err := transform.Bytes(dec, data); err == nil {
		return encGB18030, data
	}
	return encLossy, data
}

func decodeBytes(data []byte, enc fileEncoding) []byte {
	switch enc {
	case encUTF8BOM:
		return data[3:]
	case encUTF16LE:
		return decodeUTF16(data[2:], binary.LittleEndian)
	case encUTF16BE:
		return decodeUTF16(data[2:], binary.BigEndian)
	case encGB18030:
		out, _, err := transform.Bytes(simplifiedchinese.GB18030.NewDecoder(), data)
		if err != nil {
			return data
		}
		return out
	}
	return data
}

// encodeBytes 将 UTF-8 文本编码回原格式写入磁盘。
func encodeBytes(text string, enc fileEncoding) []byte {
	switch enc {
	case encUTF8BOM:
		return append(utf8BOM, []byte(text)...)
	case encUTF16LE:
		return encodeUTF16(text, binary.LittleEndian, true)
	case encUTF16BE:
		return encodeUTF16(text, binary.BigEndian, true)
	case encGB18030:
		out, _, err := transform.Bytes(simplifiedchinese.GB18030.NewEncoder(), []byte(text))
		if err != nil {
			return []byte(text)
		}
		return out
	}
	return []byte(text)
}

func encodingDecoder(enc fileEncoding) transform.Transformer {
	switch enc {
	case encGB18030:
		return simplifiedchinese.GB18030.NewDecoder()
	}
	return nil
}

// readFileEncoded 与 writeFileEncoded 供 edit_file / write_file 使用。
func readFileEncoded(path string) (content string, enc fileEncoding, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", encUTF8, err
	}
	enc, _ = detectFullEncoding(b)
	return string(decodeBytes(b, enc)), enc, nil
}

func writeFileEncoded(path string, content string, enc fileEncoding) error {
	return os.WriteFile(path, encodeBytes(content, enc), 0o644)
}

func decodeUTF16(b []byte, order binary.ByteOrder) []byte {
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = order.Uint16(b[i*2:])
	}
	runes := utf16Decode(u)
	return []byte(string(runes))
}

func encodeUTF16(text string, order binary.ByteOrder, withBOM bool) []byte {
	runes := []rune(text)
	encoded := utf16Encode(runes)

	var buf bytes.Buffer
	if withBOM {
		var bom [2]byte
		if order == binary.LittleEndian {
			bom[0], bom[1] = 0xFF, 0xFE
		} else {
			bom[0], bom[1] = 0xFE, 0xFF
		}
		buf.Write(bom[:])
	}
	for _, u := range encoded {
		var b [2]byte
		order.PutUint16(b[:], u)
		buf.Write(b[:])
	}
	return buf.Bytes()
}

func utf16Decode(u []uint16) []rune {
	var out []rune
	for i := 0; i < len(u); i++ {
		c := u[i]
		if c >= 0xD800 && c <= 0xDBFF && i+1 < len(u) {
			c2 := u[i+1]
			if c2 >= 0xDC00 && c2 <= 0xDFFF {
				out = append(out, rune(c-0xD800)<<10|rune(c2-0xDC00)+0x10000)
				i++
				continue
			}
		}
		out = append(out, rune(c))
	}
	return out
}

func utf16Encode(runes []rune) []uint16 {
	var out []uint16
	for _, r := range runes {
		if r >= 0x10000 && r <= 0x10FFFF {
			r -= 0x10000
			out = append(out, uint16(0xD800+(r>>10)), uint16(0xDC00+(r&0x3FF)))
		} else {
			out = append(out, uint16(r))
		}
	}
	return out
}

// scanReader 把 io.Reader 的内容读到 string。
func scanReader(r io.Reader) string {
	data, _ := io.ReadAll(r)
	return string(data)
}

// transformReader 用 transform.Transformer 包装 reader。
func transformReader(r io.Reader, tr transform.Transformer) string {
	// 简单方式：全部读到内存再转换
	data, _ := io.ReadAll(r)
	out, _, err := transform.Bytes(tr, data)
	if err != nil {
		return string(data)
	}
	return string(out)
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
	p.Path = NormalizeMingwPath(p.Path)
	if err := t.checkPath(p.Path); err != nil {
		return "", err
	}

	// 父目录自动创建
	dir := filepath.Dir(p.Path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("创建父目录失败: %w", err)
		}
	}

	if p.Append {
		f, err := os.OpenFile(p.Path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			return "", fmt.Errorf("打开文件失败: %w", err)
		}
		defer f.Close()
		n, err := f.WriteString(p.Content)
		if err != nil {
			return "", fmt.Errorf("写入文件失败: %w", err)
		}
		return fmt.Sprintf("OK: 追加写入 %d 字节到 %s", n, p.Path), nil
	}

	// 保持原文件编码（覆盖模式）
	existing, enc, rerr := readFileEncoded(p.Path)
	if rerr == nil && existing == p.Content {
		return fmt.Sprintf("%s 已包含完全相同的内容，无需写入", p.Path), nil
	}
	if rerr != nil {
		enc = encUTF8 // 文件不存在，默认 UTF-8
	}
	if err := writeFileEncoded(p.Path, p.Content, enc); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(p.Content), p.Path), nil
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
	p.Path = NormalizeMingwPath(p.Path)
	if p.OldText == "" {
		return "", errors.New("old_text 不能为空")
	}
	if err := t.checkPath(p.Path); err != nil {
		return "", err
	}

	content, enc, err := readFileEncoded(p.Path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}

	old, newStr := matchIndent(content, p.OldText, p.NewText)
	old, newStr = matchLineEndings(content, old, newStr)
	count := strings.Count(content, old)
	if count == 0 {
		return "", fmt.Errorf("old_text 在文件中未找到。尝试匹配: %q\n提示: 检查空白字符（tab vs 空格），用 read_file 重新确认目标文本的精确内容",
			p.OldText)
	}
	if !p.All && count > 1 {
		return "", fmt.Errorf("old_text 在文件中匹配 %d 次，要求唯一匹配；如需全部替换请设置 all=true", count)
	}

	var newContent string
	if p.All {
		newContent = strings.ReplaceAll(content, old, newStr)
	} else {
		// count == 1 已由上面 if 保证
		idx := strings.Index(content, old)
		newContent = content[:idx] + newStr + content[idx+len(old):]
	}

	if err := writeFileEncoded(p.Path, newContent, enc); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}
	return fmt.Sprintf("OK: 替换 %d 处", count), nil
}

// matchIndent 适配 old/new 的缩进风格以匹配文件。当 LLM 通过 TUI 渲染看到空格
// 但文件使用 tab（或反过来）时，将 old_text 转为文件实际使用的缩进风格后再匹配。
// 模式与 Reasonix 的 matchLineEndings 一致：精确匹配优先，失败后做一次轻量适配。
func matchIndent(content, old, newStr string) (string, string) {
	if strings.Contains(content, old) {
		return old, newStr
	}
	// 文件含 tab 但 LLM 用了空格 → 尝试把 4 空格组转 tab
	if strings.Contains(content, "\t") {
		tabbed := strings.ReplaceAll(old, "    ", "\t")
		if strings.Contains(content, tabbed) {
			return tabbed, strings.ReplaceAll(newStr, "    ", "\t")
		}
	}
	// 文件用空格但 LLM 用了 tab → 尝试把 tab 转 4 空格
	if strings.Contains(old, "\t") {
		spaced := strings.ReplaceAll(old, "\t", "    ")
		if strings.Contains(content, spaced) {
			return spaced, strings.ReplaceAll(newStr, "\t", "    ")
		}
	}
	return old, newStr
}

// matchLineEndings adapts an edit's old/new text to a CRLF file when the literal
// old_string isn't present but its CRLF form is. read_file strips '\r' so a
// model's multi-line old_string arrives LF-only while a Windows source stores
// '\r\n'; rewriting search and replacement to the file's ending fixes the match
// without rewriting the file's other line endings.
func matchLineEndings(content, old, newStr string) (string, string) {
	if strings.Contains(content, old) || !strings.Contains(content, "\r\n") {
		return old, newStr
	}
	toCRLF := func(s string) string {
		return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
	}
	if strings.Contains(content, toCRLF(old)) {
		return toCRLF(old), toCRLF(newStr)
	}
	return old, newStr
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
	base := NormalizeMingwPath(p.BaseDir)
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
