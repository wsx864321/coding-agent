package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// WorktreeTool 是 worktree 工具，实现 git worktree 的创建/列表/删除操作。
//
// 对应 using-git-worktrees Skill 的 Step 1a 原生工具语义。
type WorktreeTool struct {
	Workdir string // 项目根目录
}

// NewWorktreeTool 创建 WorktreeTool
func NewWorktreeTool(workdir string) *WorktreeTool {
	return &WorktreeTool{Workdir: workdir}
}

// ReadOnly 创建和删除 worktree 有副作用，不可并行
func (t *WorktreeTool) ReadOnly() bool { return false }

func (t *WorktreeTool) Name() string { return "worktree" }

func (t *WorktreeTool) Description() string {
	return "管理 git worktree：创建隔离的工作目录（create）、列出所有 worktree（list）、删除 worktree（remove）。" +
		" create 会自动在 .worktrees/<branch-name>/ 下创建工作目录，并在需要时将 .worktrees/ 加入 .gitignore。"
}

type worktreeArgs struct {
	Op     string `json:"op"`
	Branch string `json:"branch,omitempty"`
	Path   string `json:"path,omitempty"`
}

func (t *WorktreeTool) Schema() json.RawMessage {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"op": map[string]any{
				"type":        "string",
				"description": "操作类型：create（创建 worktree）、list（列出所有 worktree）、remove（删除 worktree）",
				"enum":        []string{"create", "list", "remove"},
			},
			"branch": map[string]any{
				"type":        "string",
				"description": "create 时：分支名（如 feature/xxx）；不传则自动生成",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "remove 时：要删除的 worktree 路径（对应 list 输出中的路径）",
			},
		},
		"required": []string{"op"},
	}
	raw, _ := json.Marshal(schema)
	return raw
}

func (t *WorktreeTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	raw, _ := json.Marshal(args)
	var p worktreeArgs
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", fmt.Errorf("解析参数失败: %w", err)
	}

	switch p.Op {
	case "create":
		return t.create(ctx, p)
	case "list":
		return t.list(ctx)
	case "remove":
		return t.remove(ctx, p)
	default:
		return "", fmt.Errorf("不支持的操作: %q，请使用 create、list 或 remove", p.Op)
	}
}

func (t *WorktreeTool) create(ctx context.Context, p worktreeArgs) (string, error) {
	// 验证当前目录是 git 仓库
	if !isGitRepo(t.Workdir) {
		return "", fmt.Errorf("当前目录不是 git 仓库，无法创建 worktree")
	}

	branch := strings.TrimSpace(p.Branch)
	if branch == "" {
		branch = "worktree-" + shortID()
	}

	// 确定 worktree 存放目录
	wtParent := filepath.Join(t.Workdir, ".worktrees")
	wtPath := filepath.Join(wtParent, branch)

	// 安全检查：确保 .worktrees/ 被 gitignore
	if err := t.ensureGitignore(); err != nil {
		return "", fmt.Errorf("无法确认 .worktrees/ 在 .gitignore 中: %w", err)
	}

	// 创建目录
	if err := os.MkdirAll(wtParent, 0o755); err != nil {
		return "", fmt.Errorf("创建 .worktrees/ 目录失败: %w", err)
	}

	// 执行 git worktree add
	cmd := exec.CommandContext(ctx, "git", "worktree", "add", wtPath, "-b", branch)
	cmd.Dir = t.Workdir
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git worktree add 失败: %s", string(ee.Stderr))
		}
		return "", fmt.Errorf("git worktree add 失败: %w", err)
	}

	result := strings.TrimSpace(string(out))
	return fmt.Sprintf("Worktree 创建成功！\n\n%s\n\n路径: %s\n分支: %s\n\n提示: 在新的 worktree 中启动 agent 以使用隔离的工作空间：\n  cd %s && coding-agent chat",
		result, wtPath, branch, wtPath), nil
}

func (t *WorktreeTool) list(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "worktree", "list", "--porcelain")
	cmd.Dir = t.Workdir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git worktree list 失败: %w", err)
	}

	worktrees := parsePorcelain(string(out))
	if len(worktrees) == 0 {
		return "当前仓库没有 worktree。", nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("当前仓库共 %d 个 worktree：\n\n", len(worktrees)))
	for i, wt := range worktrees {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, wt.Path))
		if wt.Branch != "" {
			b.WriteString(fmt.Sprintf("   分支: %s\n", wt.Branch))
		}
		if wt.HEAD {
			b.WriteString("   (detached HEAD)\n")
		}
		if wt.Bare {
			b.WriteString("   (bare — 主仓库)\n")
		}
	}
	return b.String(), nil
}

func (t *WorktreeTool) remove(ctx context.Context, p worktreeArgs) (string, error) {
	path := strings.TrimSpace(p.Path)
	if path == "" {
		return "", fmt.Errorf("remove 操作需要指定 path 参数（worktree 路径）")
	}

	cmd := exec.CommandContext(ctx, "git", "worktree", "remove", path)
	cmd.Dir = t.Workdir
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git worktree remove 失败: %s", string(ee.Stderr))
		}
		return "", fmt.Errorf("git worktree remove 失败: %w", err)
	}

	// 顺便 prune
	pruneCmd := exec.CommandContext(ctx, "git", "worktree", "prune")
	pruneCmd.Dir = t.Workdir
	pruneCmd.Output() // 忽略错误

	return fmt.Sprintf("Worktree 已删除: %s\n%s", path, strings.TrimSpace(string(out))), nil
}

// ensureGitignore 确保 .worktrees/ 在 .gitignore 中
func (t *WorktreeTool) ensureGitignore() error {
	giPath := filepath.Join(t.Workdir, ".gitignore")

	// 读取现有内容
	data, err := os.ReadFile(giPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("读取 .gitignore 失败: %w", err)
	}

	content := string(data)
	if strings.Contains(content, ".worktrees") {
		return nil // 已存在
	}

	// 追加
	f, err := os.OpenFile(giPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("打开 .gitignore 失败: %w", err)
	}
	defer f.Close()

	if len(data) > 0 && !strings.HasSuffix(content, "\n") {
		f.WriteString("\n")
	}
	f.WriteString("# git worktree 工作目录\n.worktrees/\n")
	return nil
}

// ---------- helpers ----------

func isGitRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = dir
	return cmd.Run() == nil
}

type worktreeInfo struct {
	Path   string
	Branch string
	HEAD   bool
	Bare   bool
}

// parsePorcelain 解析 git worktree list --porcelain 的输出
func parsePorcelain(out string) []worktreeInfo {
	var wts []worktreeInfo
	var current *worktreeInfo

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if current != nil {
				wts = append(wts, *current)
				current = nil
			}
			continue
		}

		if current == nil {
			current = &worktreeInfo{}
		}

		if strings.HasPrefix(line, "worktree ") {
			current.Path = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "HEAD ") {
			current.HEAD = true
		} else if strings.HasPrefix(line, "branch ") {
			current.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
		} else if line == "bare" {
			current.Bare = true
		}
	}

	if current != nil {
		wts = append(wts, *current)
	}

	return wts
}

func shortID() string {
	return strconv.FormatInt(time.Now().UnixNano()%1000000, 16)
}
