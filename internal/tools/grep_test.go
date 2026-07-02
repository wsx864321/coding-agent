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
// grep
// =====================================================================

func TestGrepFile_Single(t *testing.T) {
	dir := mkTempDir(t)
	path := writeTemp(t, dir, "a.txt", "hello world\nfoo bar\nhello again\n")

	tool := NewGrepTool(dir)
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "hello",
		"path":    path,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 matches, got %d: %q", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], path+":1:") {
		t.Errorf("expected line 1, got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], path+":3:") {
		t.Errorf("expected line 3, got %q", lines[1])
	}
}

func TestGrepFile_NoMatch(t *testing.T) {
	dir := mkTempDir(t)
	path := writeTemp(t, dir, "a.txt", "hello world\nfoo bar\n")

	tool := NewGrepTool(dir)
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "xyzzy",
		"path":    path,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "(无匹配)" {
		t.Errorf("expected (无匹配), got %q", out)
	}
}

func TestGrepFile_Regex(t *testing.T) {
	dir := mkTempDir(t)
	path := writeTemp(t, dir, "a.txt", "alpha\nbeta\naleph\n")

	tool := NewGrepTool(dir)
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern": `al.*a`,
		"path":    path,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 match, got %d: %q", len(lines), out)
	}
}

func TestGrepFile_BadRegex(t *testing.T) {
	dir := mkTempDir(t)
	path := writeTemp(t, dir, "a.txt", "hello\n")

	tool := NewGrepTool(dir)
	_, err := tool.Execute(context.Background(), map[string]any{
		"pattern": `[unclosed`,
		"path":    path,
	})
	if err == nil {
		t.Error("expected error for invalid regex")
	}
	if !strings.Contains(err.Error(), "正则表达式编译失败") {
		t.Errorf("expected regex compile error, got: %v", err)
	}
}

func TestGrepDir_Recursive(t *testing.T) {
	dir := mkTempDir(t)
	writeTemp(t, dir, "a.go", "package main\nfunc main() {}")
	writeTemp(t, dir, "sub/b.go", "package sub\nfunc helper() {}")

	tool := NewGrepTool(dir)
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "func",
		"path":    dir,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 matches, got %d: %q", len(lines), out)
	}
}

func TestGrepDir_SkipsGit(t *testing.T) {
	dir := mkTempDir(t)
	writeTemp(t, dir, "a.go", "func hello() {}")
	writeTemp(t, dir, ".git/config", "func shouldBeSkipped() {}")
	writeTemp(t, dir, "node_modules/pkg/index.js", "func skippedToo() {}")

	tool := NewGrepTool(dir)
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "func",
		"path":    dir,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(out, ".git") {
		t.Errorf("expected .git to be skipped, got %q", out)
	}
	if strings.Contains(out, "node_modules") {
		t.Errorf("expected node_modules to be skipped, got %q", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 match from a.go, got %d: %q", len(lines), out)
	}
}

func TestGrepDir_SkipsHidden(t *testing.T) {
	dir := mkTempDir(t)
	writeTemp(t, dir, "a.txt", "visible match")
	writeTemp(t, dir, ".hidden/h.txt", "hidden match")
	writeTemp(t, dir, ".hiddendotfile", "dotfile match")

	tool := NewGrepTool(dir)
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "match",
		"path":    dir,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(out, ".hidden") {
		t.Errorf("expected hidden dir/file to be skipped, got %q", out)
	}
	if strings.Contains(out, "hidden match") {
		t.Errorf("expected hidden match to be skipped, got %q", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 match (visible), got %d: %q", len(lines), out)
	}
}

func TestGrepDir_SkipsBinary(t *testing.T) {
	dir := mkTempDir(t)
	writeTemp(t, dir, "a.txt", "hello world")
	binaryPath := filepath.Join(dir, "bin.bin")
	if err := os.WriteFile(binaryPath, []byte{0, 1, 2, 3, 'h', 'e', 'l', 'l', 'o'}, 0o644); err != nil {
		t.Fatalf("WriteFile binary: %v", err)
	}

	tool := NewGrepTool(dir)
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "hello",
		"path":    dir,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(out, "bin.bin") {
		t.Errorf("expected binary file to be skipped, got %q", out)
	}
}

func TestGrep_DefaultPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip on windows: path separator semantics in test")
	}
	dir := mkTempDir(t)
	writeTemp(t, dir, "test.go", "package p")

	// change into dir, execute with no path → defaults to "."
	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(orig)

	tool := NewGrepTool(dir)
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "package",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "test.go:1:package p") {
		t.Errorf("expected default path match, got %q", out)
	}
}

func TestGrep_PathWhitelist(t *testing.T) {
	dir := mkTempDir(t)
	writeTemp(t, dir, "a.txt", "hello")

	tool := NewGrepTool(filepath.Join(dir, "subdir")) // different whitelist
	_, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "hello",
		"path":    dir,
	})
	if err == nil {
		t.Error("expected path whitelist error")
	}
	if !strings.Contains(err.Error(), "不在允许的目录白名单中") {
		t.Errorf("expected whitelist error, got: %v", err)
	}
}

func TestGrep_Timeout(t *testing.T) {
	dir := mkTempDir(t)
	writeTemp(t, dir, "a.txt", "hello")

	tool := NewGrepTool(dir)
	out, err := tool.Execute(context.Background(), map[string]any{
		"pattern":         "hello",
		"path":            dir,
		"timeout_seconds": 1,
	})
	if err != nil {
		t.Fatalf("Execute with timeout: %v", err)
	}
	if !strings.Contains(out, "a.txt:1:hello") {
		t.Errorf("expected match, got %q", out)
	}
}

func TestGrep_EmptyPattern(t *testing.T) {
	dir := mkTempDir(t)
	tool := NewGrepTool(dir)
	_, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "",
		"path":    dir,
	})
	if err == nil {
		t.Error("expected error for empty pattern")
	}
}

func TestGrep_NonexistentPath(t *testing.T) {
	tool := NewGrepTool("")
	_, err := tool.Execute(context.Background(), map[string]any{
		"pattern": "hello",
		"path":    "/nonexistent/path/12345",
	})
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}
