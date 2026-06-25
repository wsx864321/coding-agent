# git-status-display Specification

## Purpose
TBD - created by archiving change tui-status-panels. Update Purpose after archive.
## Requirements
### Requirement: Git 分支与状态显示
系统 SHALL 在状态栏模式行显示当前 Git 仓库的分支名和工作区状态。显示格式为 "branchName"（clean）、"branchName *"（dirty）、"branchName ↑N"（N commits ahead）。

#### Scenario: 在 Git 仓库中启动 TUI
- **WHEN** 当前工作目录是 Git 仓库且 TUI 启动
- **THEN** 状态栏显示当前分支名（如 "main"）

#### Scenario: 工作区有未提交更改
- **WHEN** `git status --porcelain` 返回非空结果
- **THEN** 分支名后显示 "*" 标记（如 "main *"）

#### Scenario: 本地分支领先远程
- **WHEN** 本地分支有未推送的 commits
- **THEN** 分支名后显示 "↑N"（如 "main ↑3"）

#### Scenario: 非 Git 仓库
- **WHEN** 当前工作目录不是 Git 仓库
- **THEN** Git 状态不显示

#### Scenario: Git 状态刷新
- **WHEN** 每个 turn 完成后
- **THEN** Git 状态异步刷新

