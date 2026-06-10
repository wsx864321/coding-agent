package tools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// =====================================================================
// 通用辅助
// =====================================================================

// mkTempDir 在 t.TempDir() 基础上返回绝对路径
func mkTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	return abs
}

// writeTemp 在 dir 下写一个文件，返回完整路径
func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return p
}

// =====================================================================
// read_file
// =====================================================================

func TestReadFile_Whole(t *testing.T) {
	dir := mkTempDir(t)
	path := writeTemp(t, dir, "a.txt", "hello\nworld\n")

	tool := NewReadFileTool(dir)
	out, err := tool.Execute(context.Background(), map[string]any{
		"path": path,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "hello\nworld\n" {
		t.Errorf("got %q, want %q", out, "hello\nworld\n")
	}
}

func TestReadFile_LineRange(t *testing.T) {
	dir := mkTempDir(t)
	path := writeTemp(t, dir, "a.txt", "L1\nL2\nL3\nL4\nL5\n")

	tool := NewReadFileTool(dir)
	out, err := tool.Execute(context.Background(), map[string]any{
		"path":  path,
		"start": 2,
		"end":   4,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "L2\nL3\nL4" {
		t.Errorf("got %q, want %q", out, "L2\nL3\nL4")
	}
}

func TestReadFile_AllowedDirsBlocks(t *testing.T) {
	allowed := mkTempDir(t)
	outside := mkTempDir(t)
	path := writeTemp(t, outside, "secret.txt", "nope")

	tool := NewReadFileTool("")
	tool.AllowedDirs = []string{allowed}
	_, err := tool.Execute(context.Background(), map[string]any{
		"path": path,
	})
	if err == nil {
		t.Fatal("expected error when path outside AllowedDirs, got nil")
	}
}

func TestReadFile_NotFound(t *testing.T) {
	tool := NewReadFileTool("")
	_, err := tool.Execute(context.Background(), map[string]any{
		"path": filepath.Join(t.TempDir(), "no-such-file"),
	})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// =====================================================================
// write_file
// =====================================================================

func TestWriteFile_Overwrite(t *testing.T) {
	dir := mkTempDir(t)
	path := filepath.Join(dir, "nested", "out.txt")

	tool := NewWriteFileTool(dir)
	out, err := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"content": "new content",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "覆盖") {
		t.Errorf("expected 覆盖 in output, got %q", out)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "new content" {
		t.Errorf("file content = %q, want %q", got, "new content")
	}
}

func TestWriteFile_Append(t *testing.T) {
	dir := mkTempDir(t)
	path := writeTemp(t, dir, "log.txt", "first\n")

	tool := NewWriteFileTool(dir)
	_, err := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"content": "second\n",
		"append":  true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "first\nsecond\n" {
		t.Errorf("file content = %q, want %q", got, "first\nsecond\n")
	}
}

func TestWriteFile_AllowedDirsBlocks(t *testing.T) {
	allowed := mkTempDir(t)
	outside := mkTempDir(t)
	path := filepath.Join(outside, "x.txt")

	tool := NewWriteFileTool("")
	tool.AllowedDirs = []string{allowed}
	_, err := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"content": "x",
	})
	if err == nil {
		t.Fatal("expected error when path outside AllowedDirs")
	}
}

// =====================================================================
// edit_file
// =====================================================================

func TestEditFile_UniqueReplace(t *testing.T) {
	dir := mkTempDir(t)
	path := writeTemp(t, dir, "a.go", "foo bar foo\n")

	tool := NewEditFileTool(dir)
	out, err := tool.Execute(context.Background(), map[string]any{
		"path":     path,
		"old_text": "bar",
		"new_text": "baz",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "替换 1 处") {
		t.Errorf("got %q, want contains 替换 1 处", out)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "foo baz foo\n" {
		t.Errorf("file = %q, want %q", got, "foo baz foo\n")
	}
}

func TestEditFile_MultipleRequiresAll(t *testing.T) {
	dir := mkTempDir(t)
	path := writeTemp(t, dir, "a.go", "foo foo foo\n")

	tool := NewEditFileTool(dir)
	_, err := tool.Execute(context.Background(), map[string]any{
		"path":     path,
		"old_text": "foo",
		"new_text": "bar",
	})
	if err == nil {
		t.Fatal("expected error for non-unique match")
	}
}

func TestEditFile_AllFlag(t *testing.T) {
	dir := mkTempDir(t)
	path := writeTemp(t, dir, "a.go", "foo foo foo\n")

	tool := NewEditFileTool(dir)
	_, err := tool.Execute(context.Background(), map[string]any{
		"path":     path,
		"old_text": "foo",
		"new_text": "bar",
		"all":      true,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "bar bar bar\n" {
		t.Errorf("file = %q, want %q", got, "bar bar bar\n")
	}
}

func TestEditFile_NotFound(t *testing.T) {
	dir := mkTempDir(t)
	path := writeTemp(t, dir, "a.go", "hello\n")

	tool := NewEditFileTool(dir)
	_, err := tool.Execute(context.Background(), map[string]any{
		"path":     path,
		"old_text": "world",
		"new_text": "x",
	})
	if err == nil {
		t.Fatal("expected error when old_text not found")
	}
}

// =====================================================================
// glob_file
// =====================================================================

func TestGlobFile_Basic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip on windows: posix path semantics")
	}
	dir := mkTempDir(t)
	writeTemp(t, dir, "a.go", "x")
	writeTemp(t, dir, "b.go", "x")
	writeTemp(t, dir, "c.txt", "x")
	writeTemp(t, dir, "sub/d.go", "x")

	tool := NewGlobFileTool(dir)
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "*.go",
		"base_dir": dir,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 matches, got %d: %q", len(lines), out)
	}
}

func TestGlobFile_Recursive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip on windows: posix path semantics")
	}
	dir := mkTempDir(t)
	writeTemp(t, dir, "a.go", "x")
	writeTemp(t, dir, "sub/b.go", "x")
	writeTemp(t, dir, "sub/deep/c.go", "x")

	tool := NewGlobFileTool(dir)
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "**/*.go",
		"base_dir": dir,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 matches, got %d: %q", len(lines), out)
	}
}

func TestGlobFile_MaxResults(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip on windows: posix path semantics")
	}
	dir := mkTempDir(t)
	writeTemp(t, dir, "a.go", "x")
	writeTemp(t, dir, "b.go", "x")
	writeTemp(t, dir, "c.go", "x")

	tool := NewGlobFileTool(dir)
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern":     "*.go",
		"base_dir":    dir,
		"max_results": 2,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// 期望前 2 条匹配 + 1 条截断提示
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 2 results + 1 truncation marker, got %d lines: %q", len(lines), out)
	}
	if !strings.Contains(lines[len(lines)-1], "截断") {
		t.Errorf("expected last line to be truncation marker, got %q", lines[len(lines)-1])
	}
}

func TestGlobFile_SkipsGitDir(t *testing.T) {
	dir := mkTempDir(t)
	writeTemp(t, dir, "a.go", "x")
	writeTemp(t, dir, ".git/x.go", "x") // 应被跳过

	tool := NewGlobFileTool(dir)
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern":  "**/*.go",
		"base_dir": dir,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(out, ".git") {
		t.Errorf("expected .git to be skipped, got %q", out)
	}
}

func TestGlobFile_NoMatch(t *testing.T) {
	dir := mkTempDir(t)
	tool := NewGlobFileTool("")
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern":  "*.zzz",
		"base_dir": dir,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "(无匹配)" {
		t.Errorf("got %q, want %q", out, "(无匹配)")
	}
}

// =====================================================================
// globToRegexp 单元测试
// =====================================================================

func TestGlobToRegexp(t *testing.T) {
	cases := []struct {
		pattern string
		input   string
		want    bool
	}{
		{"*.go", "main.go", true},
		{"*.go", "main.txt", false},
		{"*.go", "a/b.go", false},  // * 不跨 /
		{"**/*.go", "a/b/c.go", true},
		{"**/*.go", "c.go", true},
		{"file?.txt", "file1.txt", true},
		{"file?.txt", "file12.txt", false},
		{"[abc].go", "a.go", true},
		{"[abc].go", "d.go", false},
		{"a-b_c.d", "a-b_c.d", true}, // 字面量
	}
	for _, c := range cases {
		t.Run(c.pattern+"/"+c.input, func(t *testing.T) {
			re, err := globToRegexp(c.pattern)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			got := re.MatchString(c.input)
			if got != c.want {
				t.Errorf("pattern=%q input=%q got=%v want=%v", c.pattern, c.input, got, c.want)
			}
		})
	}
}
