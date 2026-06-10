package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// BashTool 实现了跨平台（Windows / 类 Unix）命令行执行的工具
//
// 设计要点：
//   - 在 Windows 上优先使用 cmd.exe /C 执行；
//   - 在类 Unix 系统上使用 sh -c 执行；
//   - 支持设置工作目录与超时时间；
//   - 自动捕获 stdout 与 stderr 并合并返回；
//   - 不在工具内部对用户命令做"语义解释"或拦截；安全策略由调用方按需扩展。
type BashTool struct {
	// DefaultTimeout 默认执行超时时间，0 表示不超时
	DefaultTimeout time.Duration
	// AllowedDirs 允许执行命令的工作目录白名单；为空表示不限制
	AllowedDirs []string
	// MaxOutputBytes 单次执行允许捕获的最大输出字节数；0 表示不限制
	MaxOutputBytes int
}

// NewBashTool 创建一个具有默认配置的 BashTool
//
// 参数 workdir：file 系列工具的白名单基准目录；bash 工具自身不消费此值（按设计不限工作目录）
//
// 安全策略：默认不限制 AllowedDirs。bash 工具的"安全"主要由调用方控制：
//   - 调用方可通过 agent.WithBashAllowedDirs(...) 收紧；
//   - 顶层更推荐走 permission.Checker / hooks.PreToolUse 实现 allow/ask/deny
//
// 之所以不默认收紧到 cwd：
//   - bash 的合法用途常常跨目录（cd /path/to/project && make test）
//   - AllowedDirs 留空意味着放行；调用方按需收紧
func NewBashTool(workdir string) *BashTool {
	return &BashTool{
		DefaultTimeout: 60 * time.Second,
		MaxOutputBytes: 1024 * 1024, // 默认 1MB
	}
}

// Name 返回工具名称
func (b *BashTool) Name() string {
	return "bash"
}

// Description 返回工具的功能描述
func (b *BashTool) Description() string {
	return "在本地终端执行一条 shell 命令，支持 Windows（cmd.exe）与类 Unix（sh）系统。" +
		"可指定工作目录与超时时间，stdout 与 stderr 会被合并返回。"
}

// bashArgs 是 BashTool 的执行参数
type bashArgs struct {
	// Command 待执行的命令字符串
	Command string `json:"command"`
	// Workdir 可选，指定执行命令的工作目录
	Workdir string `json:"workdir,omitempty"`
	// Timeout 可选，单次执行超时时间（秒），0 表示不超时
	Timeout int `json:"timeout,omitempty"`
}

// Schema 返回工具参数的 JSON Schema
func (b *BashTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "需要执行的 shell 命令字符串",
			},
			"workdir": map[string]any{
				"type":        "string",
				"description": "执行命令的工作目录，默认为当前进程工作目录",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"minimum":     0,
				"description": "超时时间（秒），0 表示不超时，默认 60s",
			},
		},
		"required": []string{"command"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

// Execute 执行工具
func (b *BashTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	var params bashArgs
	if err := decodeArgs(args, &params); err != nil {
		return "", err
	}

	command := strings.TrimSpace(params.Command)
	if command == "" {
		return "", errors.New("command 不能为空")
	}

	// 处理超时
	timeout := b.DefaultTimeout
	if params.Timeout > 0 {
		timeout = time.Duration(params.Timeout) * time.Second
	}

	// 处理工作目录白名单
	if len(b.AllowedDirs) > 0 && params.Workdir != "" {
		ok, err := isInAllowedDirs(params.Workdir, b.AllowedDirs)
		if err != nil {
			return "", fmt.Errorf("校验 workdir 失败: %w", err)
		}
		if !ok {
			return "", fmt.Errorf("workdir %q 不在允许的目录白名单中", params.Workdir)
		}
	}

	// 应用超时（必须先于 buildCommand，以便 CommandContext 绑定的 ctx 包含超时）
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// 构造跨平台命令
	cmd, err := buildCommand(ctx, command, params.Workdir)
	if err != nil {
		return "", err
	}

	// 限制最大输出
	var stdout, stderr bytes.Buffer
	if b.MaxOutputBytes > 0 {
		cmd.Stdout = &limitedWriter{w: &stdout, n: b.MaxOutputBytes}
		cmd.Stderr = &limitedWriter{w: &stderr, n: b.MaxOutputBytes}
	} else {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}

	// 执行
	err = cmd.Run()

	// 合并输出
	output := mergeOutput(stdout.String(), stderr.String())
	if b.MaxOutputBytes > 0 && int64(len(output)) > int64(b.MaxOutputBytes) {
		output = output[:b.MaxOutputBytes] + "\n... (输出被截断)"
	}

	// 处理超时错误
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return output, fmt.Errorf("命令执行超时 (>= %s)", timeout)
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return output, fmt.Errorf("命令执行被取消")
	}

	// 处理退出码
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return output, fmt.Errorf("命令执行失败，退出码 %d: %w", exitErr.ExitCode(), err)
		}
		return output, fmt.Errorf("执行命令失败: %w", err)
	}

	return output, nil
}

// buildCommand 根据操作系统构造对应的 exec.Cmd
//
// Windows: cmd.exe /C <command>
// Unix:    sh -c <command>
func buildCommand(ctx context.Context, command, workdir string) (*exec.Cmd, error) {
	if runtime.GOOS == "windows" {
		// Windows 下使用 cmd.exe /C 执行
		// 注意：使用 /S /C "..." 包裹以正确处理包含特殊字符的命令
		cmd := exec.CommandContext(ctx, "cmd.exe", "/S", "/C", command)
		if workdir != "" {
			cmd.Dir = workdir
		}
		return cmd, nil
	}

	// 类 Unix 系统使用 sh -c
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	if workdir != "" {
		cmd.Dir = workdir
	}
	return cmd, nil
}

// mergeOutput 合并 stdout 和 stderr
func mergeOutput(stdout, stderr string) string {
	if stderr == "" {
		return stdout
	}
	if stdout == "" {
		return stderr
	}
	return stdout + "\n" + stderr
}

// limitedWriter 用于限制单个流的最大写入字节数
type limitedWriter struct {
	w       *bytes.Buffer
	n       int
	written int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	remaining := l.n - l.written
	if remaining <= 0 {
		return len(p), nil // 假装写入，避免命令执行错误
	}
	if len(p) > remaining {
		_, _ = l.w.Write(p[:remaining])
		l.written = l.n
		return len(p), nil
	}
	_, _ = l.w.Write(p)
	l.written += len(p)
	return len(p), nil
}
