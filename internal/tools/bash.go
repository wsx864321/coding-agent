package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/wsx864321/coding-agent/internal/jobs"
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

// ReadOnly bash 可能有任意副作用，不可并行
func (b *BashTool) ReadOnly() bool { return false }

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
	// RunInBackground 可选，后台执行：立即返回 job id，跨 turn 持续运行。
	// 用 bash_output 读输出，wait 等待，kill_shell 终止。
	// 适合长命令（install/build/test/deploy）。
	RunInBackground bool `json:"run_in_background,omitempty"`
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
			"run_in_background": map[string]any{
				"type":        "boolean",
				"description": "后台执行：立即返回 job id，跨 turn 持续运行。用 bash_output 读输出，wait 等待，kill_shell 终止。适合长命令（install/build/test/deploy）。后台执行不受 timeout 限制。",
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

	// 后台执行分支：通过 jobs.Manager 启动，立即返回 job id
	if params.RunInBackground {
		return b.runBackground(ctx, params)
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
	if b.MaxOutputBytes > 0 && int64(len(output)) >= int64(b.MaxOutputBytes) {
		output = output[:b.MaxOutputBytes] + "\n... (输出被截断)"
	}

	// Windows: 将 Git Bash 风格路径 /d/project/... 转为 D:\project\...
	// 仅当转换后的路径实际存在时才替换，避免误伤 /usr/bin 等非 Windows 路径
	output = sanitizeBashOutput(output)

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

// runBackground 通过 jobs.Manager 后台执行命令，立即返回 job id。
// 后台 job 运行在 Manager 的 session context 下（跨 turn 存活），不受前台
// timeout 限制。stdout/stderr 流入 job buffer，模型用 bash_output 增量读取。
func (b *BashTool) runBackground(ctx context.Context, params bashArgs) (string, error) {
	jm, ok := jobs.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf("后台执行不可用：当前上下文未配置 jobs.Manager")
	}

	workdir := params.Workdir
	// 校验工作目录白名单
	if len(b.AllowedDirs) > 0 && workdir != "" {
		ok, err := isInAllowedDirs(workdir, b.AllowedDirs)
		if err != nil {
			return "", fmt.Errorf("校验 workdir 失败: %w", err)
		}
		if !ok {
			return "", fmt.Errorf("workdir %q 不在允许的目录白名单中", workdir)
		}
	}

	command := strings.TrimSpace(params.Command)
	preview := commandPreview(command)
	sessionID := jobs.SessionFromContext(ctx)

	job := jm.StartForSession(sessionID, "bash", preview, func(jobCtx context.Context, out io.Writer) (string, error) {
		cmd, err := buildCommand(jobCtx, command, workdir)
		if err != nil {
			return "", err
		}
		// 后台 job 的输出不限制单次大小（jobWriter 已有 10MB 上限防 OOM）
		cmd.Stdout = out
		cmd.Stderr = out
		return "", cmd.Run()
	})

	return fmt.Sprintf("已启动后台任务 %q。它跨 turn 持续运行；用 bash_output(job_id=%q) 读取输出，wait 等待完成，kill_shell(job_id=%q) 终止。",
		job.ID, job.ID, job.ID), nil
}

// commandPreview 截取命令前若干字符作为 job label，便于日志展示。
func commandPreview(cmd string) string {
	const maxLen = 60
	cmd = strings.ReplaceAll(cmd, "\n", " ")
	cmd = strings.TrimSpace(cmd)
	if len(cmd) > maxLen {
		return cmd[:maxLen] + "..."
	}
	return cmd
}

// sanitizeBashOutput 将 bash 输出中的 Git Bash / MSYS2 posix 路径（如 /d/project/...）
// 转换为 Windows 路径，从源头阻止 LLM 学到无法使用的路径格式。
// 非 Windows 平台直接透传；转换后的路径会通过 os.Stat 验证存在性。
func sanitizeBashOutput(output string) string {
	if runtime.GOOS != "windows" {
		return output
	}
	return msys2PathRe.ReplaceAllStringFunc(output, func(match string) string {
		win := NormalizeMingwPath(match)
		if win == match {
			return match
		}
		// 验证转换后的路径确实存在（可能是目录也可能是文件）
		if _, err := os.Stat(win); err == nil {
			return win
		}
		// 尝试父目录：/d/project/coding-agent/internal/tools/files.go
		// 如果文件不存在但父目录存在，也做转换
		dir := win
		for {
			parent := filepath.Dir(dir)
			if parent == dir {
				return match
			}
			if _, err := os.Stat(parent); err == nil {
				return win
			}
			dir = parent
		}
	})
}

// msys2PathRe 匹配 MSYS2 / Git Bash 风格的 posix 绝对路径：
// /单字母/剩余...  例如 /d/project/coding-agent/src/main.go
var msys2PathRe *regexp.Regexp

func init() {
	msys2PathRe = regexp.MustCompile(`/[a-zA-Z]/[^\s"'` + "`" + `;:]*`)
}
