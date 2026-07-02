package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// GrepTool 使用正则表达式在文件中搜索文本内容。
//
// 当 path 为文件时直接搜索该文件；为目录时递归搜索（跳过 .git、node_modules 及隐藏文件）。
// 输出格式 path:line:text，最多 200 条匹配。
type GrepTool struct {
	AllowedDirs []string
}

// NewGrepTool 创建默认配置的 GrepTool
//
// 参数 workdir 是 AllowedDirs 白名单的基准目录，语义同 NewReadFileTool。
func NewGrepTool(workdir string) *GrepTool {
	return &GrepTool{
		AllowedDirs: allowedDirsFromWorkdir(workdir),
	}
}

// ReadOnly 纯读文件内容，可并行
func (t *GrepTool) ReadOnly() bool { return true }

// Name 返回工具名称
func (t *GrepTool) Name() string { return "grep" }

// Description 返回工具功能描述
func (t *GrepTool) Description() string {
	return "在文件或目录中使用正则表达式（RE2 语法）搜索文本内容。" +
		"当 path 为目录时递归搜索（跳过 .git、node_modules 及以 . 开头的隐藏文件/目录）。" +
		"返回 path:line:text 格式的匹配行，最多 200 条。"
}

// grepArgs grep 的入参
type grepArgs struct {
	Pattern        string `json:"pattern"`
	Path           string `json:"path,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

// Schema 返回 JSON Schema
func (t *GrepTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "要搜索的正则表达式（RE2 语法）",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "要搜索的文件或目录路径（默认 \".\"）",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"minimum":     1,
				"description": "搜索超时秒数（默认 30，最大 300）",
			},
		},
		"required": []string{"pattern"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

// Execute 执行 grep 搜索
func (t *GrepTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	var p grepArgs
	if err := decodeArgs(args, &p); err != nil {
		return "", err
	}
	if strings.TrimSpace(p.Pattern) == "" {
		return "", fmt.Errorf("pattern 不能为空")
	}

	// 编译正则
	re, err := regexp.Compile(p.Pattern)
	if err != nil {
		return "", fmt.Errorf("正则表达式编译失败: %w", err)
	}

	searchPath := NormalizeMingwPath(p.Path)
	if searchPath == "" {
		searchPath = "."
	}

	if err := t.checkPath(searchPath); err != nil {
		return "", err
	}

	// 懒加载超时保护
	timeout := 30
	if p.TimeoutSeconds > 0 {
		timeout = p.TimeoutSeconds
		if timeout > 300 {
			timeout = 300
		}
	}
	dctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	info, err := os.Stat(searchPath)
	if err != nil {
		return "", fmt.Errorf("无法访问路径 %s: %w", searchPath, err)
	}

	var results []string
	if info.IsDir() {
		results, err = t.grepDir(dctx, re, searchPath)
	} else {
		results, err = t.grepFile(dctx, re, searchPath)
	}
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

func (t *GrepTool) checkPath(path string) error {
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

// maxGrepResults 单次搜索最多返回的匹配行数
const maxGrepResults = 200

// grepFile 在单个文件中搜索匹配行
func (t *GrepTool) grepFile(ctx context.Context, re *regexp.Regexp, path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取文件 %s 失败: %w", path, err)
	}

	// 跳过二进制文件（含 NUL 字节）
	if bytes.IndexByte(data, 0) >= 0 {
		return nil, nil
	}

	var results []string
	lines := strings.SplitAfter(string(data), "\n")
	for i, line := range lines {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		line = strings.TrimSuffix(line, "\n")
		if re.MatchString(line) {
			results = append(results, fmt.Sprintf("%s:%d:%s", path, i+1, line))
			if len(results) >= maxGrepResults {
				break
			}
		}
	}
	return results, nil
}

// grepDir 在目录中递归搜索匹配行
func (t *GrepTool) grepDir(ctx context.Context, re *regexp.Regexp, dir string) ([]string, error) {
	var results []string
	truncated := false

	err := filepath.Walk(dir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil // 跳过不可访问的路径
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// 跳过目录
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}
			// 跳过隐藏目录（. 开头）
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// 跳过隐藏文件
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// 跳过超大文件（>1MB）
		if info.Size() > 1*1024*1024 {
			return nil
		}

		// 已截断则停止遍历
		if len(results) >= maxGrepResults {
			truncated = true
			return filepath.SkipAll
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil // 跳过不可读文件
		}

		// 跳过二进制文件
		if bytes.IndexByte(data, 0) >= 0 {
			return nil
		}

		lines := strings.SplitAfter(string(data), "\n")
		for i, line := range lines {
			line = strings.TrimSuffix(line, "\n")
			if re.MatchString(line) {
				results = append(results, fmt.Sprintf("%s:%d:%s", path, i+1, line))
				if len(results) >= maxGrepResults {
					truncated = true
					return filepath.SkipAll
				}
			}
		}
		return nil
	})

	if err != nil && !errors.Is(err, filepath.SkipAll) {
		return nil, err
	}

	_ = truncated
	return results, nil
}
