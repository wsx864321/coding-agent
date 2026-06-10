package tools

import (
	"bytes"
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// =====================================================================
// 元信息
// =====================================================================

func TestBashTool_Name(t *testing.T) {
	b := NewBashTool("")
	if got := b.Name(); got != "bash" {
		t.Errorf("Name() = %q, want %q", got, "bash")
	}
}

func TestBashTool_Description(t *testing.T) {
	b := NewBashTool("")
	if d := b.Description(); d == "" {
		t.Error("Description() returned empty string")
	}
}

func TestBashTool_Schema(t *testing.T) {
	b := NewBashTool("")
	raw := b.Schema()
	if len(raw) == 0 {
		t.Fatal("Schema() returned empty")
	}
	s := string(raw)
	// 必须包含必填字段
	for _, kw := range []string{`"command"`, `"workdir"`, `"timeout"`, `"required"`} {
		if !strings.Contains(s, kw) {
			t.Errorf("Schema() missing %q: %s", kw, s)
		}
	}
}

func TestBashTool_NewBashToolDefaults(t *testing.T) {
	b := NewBashTool("")
	if b.DefaultTimeout != 60*time.Second {
		t.Errorf("DefaultTimeout = %v, want 60s", b.DefaultTimeout)
	}
	if b.MaxOutputBytes != 1024*1024 {
		t.Errorf("MaxOutputBytes = %d, want 1MB", b.MaxOutputBytes)
	}
	if b.AllowedDirs != nil {
		t.Errorf("AllowedDirs = %v, want nil", b.AllowedDirs)
	}
}

// =====================================================================
// 正常执行
// =====================================================================

// shellEchoCmd 构造一个能在 Windows + Unix 都能跑的"输出 hello"命令
func shellEchoCmd() string {
	if runtime.GOOS == "windows" {
		return "echo hello"
	}
	return `echo hello`
}

func TestBashTool_Execute_Success(t *testing.T) {
	b := NewBashTool("")
	out, err := b.Execute(context.Background(), map[string]any{
		"command": shellEchoCmd(),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("output %q does not contain %q", out, "hello")
	}
}

// shellPwdCmd 输出当前工作目录
func shellPwdCmd() string {
	if runtime.GOOS == "windows" {
		return "cd"
	}
	return "pwd"
}

func TestBashTool_Execute_Workdir(t *testing.T) {
	dir := t.TempDir()
	abs, _ := filepath.Abs(dir)
	b := NewBashTool("")

	out, err := b.Execute(context.Background(), map[string]any{
		"command": shellPwdCmd(),
		"workdir": abs,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// t.TempDir() 末尾的子目录名是 ASCII (TestBashTool_xxx<rand>\NNN)，
	// 即使系统用户目录是中文也不影响这段比较。
	// 用 Base 校验 + ToSlash 统一路径分隔符（Windows 既有 / 也有 \）
	trimmed := strings.TrimSpace(out)
	if !strings.Contains(filepath.ToSlash(trimmed), filepath.Base(filepath.ToSlash(abs))) {
		t.Errorf("output %q does not contain workdir basename %q", trimmed, filepath.Base(abs))
	}
}

// =====================================================================
// 输出捕获与合并
// =====================================================================

// shellStdoutAndStderrCmd 触发 stdout 和 stderr 都有内容
func shellStdoutAndStderrCmd() string {
	if runtime.GOOS == "windows" {
		return "echo out & echo err 1>&2"
	}
	return `echo out; echo err 1>&2`
}

func TestBashTool_Execute_StderrMerged(t *testing.T) {
	b := NewBashTool("")
	out, err := b.Execute(context.Background(), map[string]any{
		"command": shellStdoutAndStderrCmd(),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "out") {
		t.Errorf("output missing stdout: %q", out)
	}
	if !strings.Contains(out, "err") {
		t.Errorf("output missing stderr: %q", out)
	}
}

func TestMergeOutput(t *testing.T) {
	cases := []struct {
		stdout, stderr, want string
	}{
		{"a", "", "a"},
		{"", "b", "b"},
		{"a", "b", "a\nb"},
		{"", "", ""},
	}
	for _, c := range cases {
		got := mergeOutput(c.stdout, c.stderr)
		if got != c.want {
			t.Errorf("mergeOutput(%q,%q) = %q, want %q", c.stdout, c.stderr, got, c.want)
		}
	}
}

// =====================================================================
// 退出码
// =====================================================================

// shellExit1Cmd 退出码 1
func shellExit1Cmd() string {
	if runtime.GOOS == "windows" {
		return "exit 1"
	}
	return "exit 1"
}

func TestBashTool_Execute_NonZeroExit(t *testing.T) {
	b := NewBashTool("")
	_, err := b.Execute(context.Background(), map[string]any{
		"command": shellExit1Cmd(),
	})
	if err == nil {
		t.Fatal("expected error for non-zero exit, got nil")
	}
	if !strings.Contains(err.Error(), "退出码 1") {
		t.Errorf("error %q does not mention exit code 1", err.Error())
	}
}

// =====================================================================
// 超时
// =====================================================================

// shellSleepCmd 睡眠 N 秒（用于超时测试）
func shellSleepCmd(sec int) string {
	if runtime.GOOS == "windows" {
		// ping -n N localhost 1>nul 等待约 N 秒
		return "ping -n " + itoa(sec) + " 127.0.0.1 >nul"
	}
	return "sleep " + itoa(sec)
}

// itoa 避免在测试代码 import strconv
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func TestBashTool_Execute_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip on windows: cmd.exe does not propagate signals to child processes")
	}
	b := NewBashTool("")
	b.DefaultTimeout = 100 * time.Millisecond

	start := time.Now()
	_, err := b.Execute(context.Background(), map[string]any{
		"command": shellSleepCmd(5),
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "超时") {
		t.Errorf("error %q does not mention 超时", err.Error())
	}
	// 不应等到 5s 完成；容忍 4s 上限（避免 CI 抖动）
	if elapsed > 4*time.Second {
		t.Errorf("execution took %v, expected < 4s", elapsed)
	}
}

func TestBashTool_Execute_TimeoutParamOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows 上 cmd.exe 不透传信号给子进程（ping/timeout），
		// 测试容易受子进程自然退出时间影响不稳定；跳过以保证 CI 稳定。
		t.Skip("skip on windows: cmd.exe does not propagate signals to child processes")
	}
	b := NewBashTool("")
	b.DefaultTimeout = 30 * time.Second // 默认很长

	start := time.Now()
	_, err := b.Execute(context.Background(), map[string]any{
		"command": shellSleepCmd(5),
		"timeout": 1, // 1s 超时
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "超时") {
		t.Errorf("error %q does not mention 超时", err.Error())
	}
	if elapsed > 4*time.Second {
		t.Errorf("execution took %v, expected < 4s", elapsed)
	}
}

// =====================================================================
// 取消
// =====================================================================

func TestBashTool_Execute_Canceled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip on windows: cmd.exe does not propagate signals to child processes")
	}
	b := NewBashTool("")
	ctx, cancel := context.WithCancel(context.Background())
	// 50ms 后取消
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := b.Execute(ctx, map[string]any{
		"command": shellSleepCmd(5),
	})
	if err == nil {
		t.Fatal("expected cancel error, got nil")
	}
	if !strings.Contains(err.Error(), "取消") && !strings.Contains(err.Error(), "超时") {
		// cancel 会让 ctx 处于 Canceled 状态，命令被 Kill；
		// 这里允许"取消"或"超时"任一（取决于 cmd.Run 返回顺序）
		t.Errorf("error %q does not mention 取消/超时", err.Error())
	}
}

// =====================================================================
// 参数校验
// =====================================================================

func TestBashTool_Execute_EmptyCommand(t *testing.T) {
	b := NewBashTool("")
	_, err := b.Execute(context.Background(), map[string]any{
		"command": "",
	})
	if err == nil {
		t.Fatal("expected error for empty command, got nil")
	}
	if !strings.Contains(err.Error(), "command 不能为空") {
		t.Errorf("error %q does not mention empty command", err.Error())
	}
}

func TestBashTool_Execute_WhitespaceCommand(t *testing.T) {
	b := NewBashTool("")
	_, err := b.Execute(context.Background(), map[string]any{
		"command": "   \t  \n",
	})
	if err == nil {
		t.Fatal("expected error for whitespace-only command, got nil")
	}
	if !strings.Contains(err.Error(), "command 不能为空") {
		t.Errorf("error %q does not mention empty command", err.Error())
	}
}

func TestBashTool_Execute_InvalidArgs(t *testing.T) {
	b := NewBashTool("")
	// 传一个无法 unmarshal 成 bashArgs 的值（command 期望 string，传 int）
	_, err := b.Execute(context.Background(), map[string]any{
		"command": 123,
	})
	if err == nil {
		t.Fatal("expected error for invalid args, got nil")
	}
}

func TestBashTool_Execute_NilArgs(t *testing.T) {
	b := NewBashTool("")
	_, err := b.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil args, got nil")
	}
}

// =====================================================================
// AllowedDirs
// =====================================================================

func TestBashTool_Execute_AllowedDirsBlocks(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()
	outsideAbs, _ := filepath.Abs(outside)

	b := NewBashTool("")
	b.AllowedDirs = []string{allowed}

	_, err := b.Execute(context.Background(), map[string]any{
		"command": shellEchoCmd(),
		"workdir": outsideAbs,
	})
	if err == nil {
		t.Fatal("expected error when workdir outside AllowedDirs, got nil")
	}
	if !strings.Contains(err.Error(), "不在允许的目录白名单中") {
		t.Errorf("error %q does not mention allowed dirs", err.Error())
	}
}

func TestBashTool_Execute_AllowedDirsAllows(t *testing.T) {
	allowed := t.TempDir()
	allowedAbs, _ := filepath.Abs(allowed)

	b := NewBashTool("")
	b.AllowedDirs = []string{allowed}

	_, err := b.Execute(context.Background(), map[string]any{
		"command": shellEchoCmd(),
		"workdir": allowedAbs,
	})
	if err != nil {
		t.Fatalf("expected success for allowed workdir, got: %v", err)
	}
}

func TestBashTool_Execute_AllowedDirsNotCheckedWhenEmpty(t *testing.T) {
	// workdir 为空时不走白名单
	outside := t.TempDir()
	_ = outside

	b := NewBashTool("")
	b.AllowedDirs = []string{"/some/totally/unrelated/dir"}

	// 不传 workdir
	_, err := b.Execute(context.Background(), map[string]any{
		"command": shellEchoCmd(),
	})
	if err != nil {
		t.Errorf("expected success when workdir is empty, got: %v", err)
	}
}

// =====================================================================
// MaxOutputBytes
// =====================================================================

// shellBigOutputCmd 输出 N 字节
func shellBigOutputCmd(bytes int) string {
	if runtime.GOOS == "windows" {
		// 用 for 循环 echo 累加
		return "" // windows 下用通用方式
	}
	// yes + head 限定行数
	// 简化：直接输出大量字符
	return "head -c " + itoa(bytes) + " /dev/zero | tr '\\0' 'a'"
}

func TestBashTool_Execute_MaxOutputBytesTruncate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip on windows: posix-only test command")
	}
	b := NewBashTool("")
	b.MaxOutputBytes = 100 // 限制 100 字节

	out, err := b.Execute(context.Background(), map[string]any{
		"command": shellBigOutputCmd(1000),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "输出被截断") {
		t.Errorf("output %q does not contain truncation marker", out)
	}
	// 截断后总长度应 <= 100 (限) + 一些截断提示字符串
	// 我们的截断策略: 取前 100 字节 + "\n... (输出被截断)"
	const marker = "\n... (输出被截断)"
	if !strings.HasSuffix(out, marker) {
		t.Errorf("output does not end with truncation marker: %q", out)
	}
}

func TestBashTool_Execute_MaxOutputBytesZeroNoLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skip on windows: posix-only test command")
	}
	b := NewBashTool("")
	b.MaxOutputBytes = 0 // 不限制

	out, err := b.Execute(context.Background(), map[string]any{
		"command": shellBigOutputCmd(500),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(out, "输出被截断") {
		t.Errorf("output should not contain truncation marker: %q", out)
	}
	// 应当接近 500 字符
	if len(out) < 400 {
		t.Errorf("output too short: %d bytes (expected ~500)", len(out))
	}
}

// =====================================================================
// limitedWriter 单元测试
// =====================================================================

func TestLimitedWriter(t *testing.T) {
	buf := &bytes.Buffer{}
	l := &limitedWriter{w: buf, n: 10}

	// 写 5 字节，未超限
	if n, err := l.Write([]byte("hello")); err != nil || n != 5 {
		t.Errorf("Write hello: n=%d err=%v", n, err)
	}
	// 再写 5 字节，正好达限
	if n, err := l.Write([]byte("world")); err != nil || n != 5 {
		t.Errorf("Write world: n=%d err=%v", n, err)
	}
	// 再写 1 字节，超限：仍然"假装"写入成功，但缓冲区只截到 10
	if n, err := l.Write([]byte("!")); err != nil || n != 1 {
		t.Errorf("Write !: n=%d err=%v", n, err)
	}
	if buf.Len() != 10 {
		t.Errorf("buffer len = %d, want 10", buf.Len())
	}
	if buf.String() != "helloworld" {
		t.Errorf("buffer = %q, want %q", buf.String(), "helloworld")
	}
}

// =====================================================================
// buildCommand 单元测试
// =====================================================================

func TestBuildCommand_AppliesWorkdir(t *testing.T) {
	cmd, err := buildCommand(context.Background(), "echo hi", t.TempDir())
	if err != nil {
		t.Fatalf("buildCommand: %v", err)
	}
	if cmd.Dir == "" {
		t.Error("cmd.Dir is empty, expected workdir to be set")
	}
}

func TestBuildCommand_NoWorkdir(t *testing.T) {
	cmd, err := buildCommand(context.Background(), "echo hi", "")
	if err != nil {
		t.Fatalf("buildCommand: %v", err)
	}
	if cmd.Dir != "" {
		t.Errorf("cmd.Dir = %q, want empty", cmd.Dir)
	}
}
