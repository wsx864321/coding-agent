# Git Worktree 支持

coding-agent 原生支持 git worktree 隔离工作流，与 `using-git-worktrees` Skill 集成。

## 功能

### 1. 启动时自动检测

Agent 启动时自动检测当前目录的 git worktree 状态：

- **正常仓库**（`GIT_DIR == GIT_COMMON`）：不注入额外上下文
- **Linked worktree**（`GIT_DIR != GIT_COMMON` 且非 submodule）：在 system prompt 中注入 worktree 上下文
- **Submodule**：跳过 worktree 检测，视为正常仓库

检测逻辑对应 `using-git-worktrees` Skill 的 Step 0。

### 2. System Prompt 上下文注入

当检测到 worktree 时，system prompt 末尾会追加：

```
## Git Worktree 上下文

你当前在隔离的 git worktree 中工作：
- 路径: `/path/to/.worktrees/feature-x`
- 分支: `feature/x`

你的文件操作应在此 worktree 内进行，不要修改主仓库或其他 worktree 的文件。
```

Detached HEAD 时会有额外警告。

### 3. `worktree` 工具

LLM 可通过 `worktree` 工具管理 worktree，三个操作：

#### create

```json
{
  "op": "create",
  "branch": "feature/my-change"
}
```

- 自动选择目录：`.worktrees/<branch>/`
- 自动验证 `.worktrees/` 在 `.gitignore` 中（不存在则追加）
- 执行 `git worktree add .worktrees/<branch> -b <branch>`
- 返回创建结果和提示（建议在新 worktree 中启动 agent）

#### list

```json
{
  "op": "list"
}
```

- 执行 `git worktree list --porcelain`
- 解析并返回结构化列表（路径、分支、HEAD 状态、bare 标记）

#### remove

```json
{
  "op": "remove",
  "path": "/path/to/.worktrees/old-feature"
}
```

- 执行 `git worktree remove <path>`
- 自动执行 `git worktree prune` 清理

### 4. `.worktrees/` 安全守护

Agent 启动时自动检查项目 `.gitignore` 是否包含 `.worktrees/`。若未包含则追加，防止 worktree 内容被意外提交。

## 与 `using-git-worktrees` Skill 的对应

| Skill Step | coding-agent 实现 |
|-----------|------------------|
| Step 0: 检测已有 worktree | `DetectWorktree()` — agent 启动时自动执行，结果注入 system prompt |
| Step 1a: 原生 worktree 工具 | `worktree` Tool — LLM 可直接调用 create/list/remove |
| Step 1b: git worktree fallback | 保留 — LLM 仍可用 bash 工具手动操作 |
| `.gitignore` 验证 | `EnsureWorktreeGitignore()` — 启动时 + create 时双重守护 |

## 架构

```
internal/agent/
├── worktree.go        # DetectWorktree, SetWorktreeContext, SystemPromptContext
├── worktree_guard.go  # EnsureWorktreeGitignore (启动时安全守护)
└── worktree_test.go   # 8 个测试用例

internal/tools/
├── worktree.go        # WorktreeTool (create/list/remove + ensureGitignore)
└── worktree_test.go   # 11 个测试用例
```

## 安全性

- `.worktrees/` 自动加入 `.gitignore`，防止意外提交
- worktree 路径基于 workdir 计算，不从用户输入直接拼接
- 非 git 仓库时所有 worktree 操作安全降级（报错或跳过）
