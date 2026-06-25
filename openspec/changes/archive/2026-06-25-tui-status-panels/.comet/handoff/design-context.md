# Comet Design Handoff

- Change: tui-status-panels
- Phase: design
- Mode: compact
- Context hash: 6127494da1dd6f76106aeabd978db4c61636615e22fe3a3f7f32792b79a2977f

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/tui-status-panels/proposal.md

- Source: openspec/changes/tui-status-panels/proposal.md
- Lines: 1-38
- SHA256: 2450a63b11e9f83675f7d5bdf97a8f42789913b599cb1e91e8e50fdf51dfce02

```md
## Why

当前 TUI 状态栏仅显示 spinner + 耗时（运行中）或模型名 + "idle"（空闲），缺少上下文窗口使用率、缓存命中率、Git 分支状态、余额、后台任务数等关键运行时信息。同时缺少 Todo 任务面板来可视化 agent 的任务进度。对标 Reasonix 的三行状态栏布局和 Todo 面板，需要重构状态栏为信息密度更高的多行布局。

## What Changes

- **重构状态栏为三行布局**：工作行（spinner+elapsed+token↓）、模式行（Plan/YOLO/Shell 标签 + effort + git）、数据行（模型名 + 上下文仪表 + 缓存率 + 任务数 + 余额）
- **新增上下文窗口仪表**：显示已用/总量 token 数，按压缩阈值着色（绿色安全 → 黄色接近 → 红色触发）
- **新增缓存命中率显示**：显示 prompt cache 命中率百分比
- **新增 Git 分支/状态显示**：异步读取当前 worktree 的 Git 分支名和状态（clean/dirty/N commits ahead）
- **新增余额/费用显示**：异步刷新 provider 余额，显示在数据行
- **新增 Todo 任务面板**：解析 agent 的 todo_write 工具调用，以结构化面板展示在输入区上方
- **新增自定义状态行命令支持**：允许用户配置外部命令生成自定义状态行内容
- **修改 tui-chat-interface 的"状态栏信息展示"需求**：从仅显示模型名升级为多行信息布局

## Capabilities

### New Capabilities

- `context-gauge`: 上下文窗口使用率仪表，按压缩阈值着色
- `cache-hit-display`: prompt cache 命中率实时显示
- `git-status-display`: Git 分支名和状态实时显示
- `balance-display`: provider 余额异步刷新与显示
- `todo-panel`: agent todo_write 任务列表的结构化面板渲染
- `custom-statusline`: 用户自定义状态行命令支持

### Modified Capabilities

- `tui-chat-interface`: 修改"状态栏信息展示"需求，从单行模型名升级为三行信息布局

## Impact

- `internal/tui/statusbar.go`: 完全重写，从 ~40 行扩展到三行布局渲染
- `internal/tui/model.go`: 新增 gitStatus、balance、contextUsed/contextWindow、cacheHitRate、todoArgs 等字段
- `internal/tui/view.go`: 修改 View() 布局计算（bottomHeight 增加面板行）
- `internal/tui/components.go`: 可能新增 Todo 面板组件
- `internal/tui/toolcard.go`: 可能新增 todo_write 参数解析
- `cmd/cli/tui.go` 或 `cmd/cli/tui_runner.go`: 可能需要传递 config 到 TUI model
```

## openspec/changes/tui-status-panels/design.md

- Source: openspec/changes/tui-status-panels/design.md
- Lines: 1-60
- SHA256: 8ea0eddcd15a1239e144062955143f1e6a177af317844308f3a4a2dc1339ed1f

```md
## Context

当前状态栏（`internal/tui/statusbar.go`）仅约 40 行，渲染单行：运行中显示 `spinner label (elapsed)`，空闲显示 `modelName │ idle/statusMsg`。Reasonix 的状态栏为三行布局：工作行（spinner+elapsed+token↓）、模式行（Plan/YOLO 标签 + effort + git）、数据行（模型 + 上下文仪表 + 缓存率 + 任务数 + 余额），并在输入区上方渲染 Todo 面板。

本次重构将状态栏从单行扩展为多行，并新增 Todo 面板渲染。

## Goals / Non-Goals

**Goals:**
- 三行状态栏布局：工作行、模式行、数据行
- 上下文窗口仪表（已用/总量 + 压缩阈值着色）
- 缓存命中率显示
- Git 分支/状态异步读取与显示
- 余额异步刷新与显示
- Todo 面板解析与渲染
- 自定义状态行命令支持

**Non-Goals:**
- 不改变输入系统（Change C）
- 不实现覆盖层（Change D）
- 不实现系统通知
- 不改变 agent 核心逻辑（仅读取状态）

## Decisions

### D1: 状态栏使用三行固定布局

**选择**: 底部固定三行：工作行（仅运行中显示）、模式行（始终显示）、数据行（始终显示）。每行内容可换行（宽度不足时）。

**理由**: Reasonix 使用相同布局。三行分离关注点：工作行进度、模式行当前模式、数据行运行时指标。固定高度简化 viewport 高度计算。

### D2: 上下文仪表使用百分比 + 阈值着色

**选择**: 显示格式为 `ctx N/M (P%)`，颜色按压缩阈值变化：<50% 绿色、50-80% 黄色、>80% 红色。

**理由**: 直观反映上下文窗口压力，帮助用户判断是否需要 `/compact`。

### D3: Git 状态异步读取

**选择**: 使用 `tea.Cmd` 异步执行 `git` 命令读取分支名和状态，结果通过 `gitStatusMsg` 返回。在 turn 完成后和启动时刷新。

**理由**: 避免阻塞 UI 事件循环。Git 命令可能耗时（大型仓库），异步读取保证 UI 响应。

### D4: Todo 面板解析 todo_write 工具调用

**选择**: 在 ToolDispatch/ToolResult 处理中识别 `todo_write` 工具，解析其 JSON 参数，提取任务列表。面板渲染在输入区上方，显示每个任务的状态（pending/in_progress/completed）和名称。

**理由**: 用户需要在不滚动 transcript 的情况下看到当前任务进度。面板固定在底部区域，始终可见。

### D5: 余额异步刷新

**选择**: 在启动时和每个 turn 完成后异步调用 provider 的余额接口，结果缓存到 `balance` 字段。

**理由**: 余额查询可能涉及网络请求，异步执行不阻塞 UI。

## Risks / Trade-offs

- **[性能] Git 状态刷新频率**: 每个 turn 完成后刷新 Git 状态，频繁操作可能产生 I/O 压力。→ 使用轻量级 `git rev-parse --abbrev-ref HEAD` + `git status --porcelain` 命令，限制超时。
- **[兼容性] Git 不可用时**: 非 Git 仓库或无 git 命令时降级为空显示。→ 在 `fetchGitStatus` 中捕获错误，返回空 `gitStatus`。
- **[布局] 小终端适配**: 终端宽度 < 80 列时三行可能拥挤。→ 每行内容使用空格分隔，超宽时自然换行（lipgloss MaxWidth + wrap）。
```

## openspec/changes/tui-status-panels/tasks.md

- Source: openspec/changes/tui-status-panels/tasks.md
- Lines: 1-54
- SHA256: 84db2c3aa326db9789e47e766ca506e79cfab4cb61ef5ec6660df2bfe658a8d8

```md
## 1. 状态栏布局重构

- [ ] 1.1 重写 `internal/tui/statusbar.go`：实现三行布局渲染函数（renderWorkingLine、renderModeLine、renderDataLine）
- [ ] 1.2 修改 `internal/tui/view.go` 的 `bottomHeight()` 计算，纳入三行状态栏 + Todo 面板高度
- [ ] 1.3 修改 `internal/tui/view.go` 的 `View()` 方法，按新布局组装底部区域

## 2. 上下文窗口仪表

- [ ] 2.1 在 `internal/tui/model.go` 中新增 `contextUsed`、`contextWindow` 字段
- [ ] 2.2 在 Agent 或 Runner 中暴露上下文快照接口（`ContextSnapshot() (used, window int)`）
- [ ] 2.3 在 `internal/tui/statusbar.go` 中实现上下文仪表渲染（百分比 + 阈值着色）
- [ ] 2.4 在 turn 完成后刷新上下文快照

## 3. 缓存命中率显示

- [ ] 3.1 在 `internal/tui/model.go` 中新增 `cacheHitRate` 字段
- [ ] 3.2 在 Agent 或 Provider 中暴露缓存命中率接口
- [ ] 3.3 在 `internal/tui/statusbar.go` 中渲染缓存命中率（如 "cache 85%"）

## 4. Git 状态显示

- [ ] 4.1 在 `internal/tui/model.go` 中新增 `gitStatus` 结构体（branch、ahead、behind、dirty）
- [ ] 4.2 实现 `fetchGitStatus()` 异步命令：执行 `git rev-parse --abbrev-ref HEAD` + `git status --porcelain`
- [ ] 4.3 在 `internal/tui/statusbar.go` 中渲染 Git 状态（如 "main ↑3 ↓1" 或 "main *dirty"）
- [ ] 4.4 在启动时和每个 turn 完成后触发 Git 状态刷新

## 5. 余额显示

- [ ] 5.1 在 `internal/tui/model.go` 中新增 `balance` 字段
- [ ] 5.2 实现 `fetchBalance()` 异步命令：调用 provider 余额接口
- [ ] 5.3 在 `internal/tui/statusbar.go` 中渲染余额（如 "¥110.00"）
- [ ] 5.4 在启动时和每个 turn 完成后触发余额刷新

## 6. Todo 任务面板

- [ ] 6.1 在 `internal/tui/model.go` 中新增 `todoArgs` 字段（存储最近一次 todo_write 的参数）
- [ ] 6.2 在 ToolDispatch/ToolResult 处理中识别 `todo_write` 工具，解析 JSON 提取任务列表
- [ ] 6.3 在 `internal/tui/statusbar.go` 或新建 `internal/tui/todopanel.go` 中实现 Todo 面板渲染
- [ ] 6.4 Todo 面板固定在输入区上方，显示任务状态图标（⏳/✓/○）和名称
- [ ] 6.5 空任务列表时不渲染 Todo 面板（不占空间）

## 7. 自定义状态行

- [ ] 7.1 在 `internal/tui/model.go` 中新增 `statuslineCmd`、`statuslineOut` 字段
- [ ] 7.2 实现 `runStatusline()` 异步命令：执行用户配置的命令，读取 stdout 第一行
- [ ] 7.3 在 `internal/tui/statusbar.go` 中：当 statuslineCmd 非空时，用 statuslineOut 替换内置数据行
- [ ] 7.4 在启动时和每个 turn 完成后触发自定义状态行刷新

## 8. 集成测试

- [ ] 8.1 为状态栏三行布局编写单元测试
- [ ] 8.2 为上下文仪表着色逻辑编写单元测试
- [ ] 8.3 为 Todo 面板解析与渲染编写单元测试
- [ ] 8.4 运行全量测试套件确认无回归
```

## openspec/changes/tui-status-panels/specs/balance-display/spec.md

- Source: openspec/changes/tui-status-panels/specs/balance-display/spec.md
- Lines: 1-16
- SHA256: d34eb81d9aec6c8343ee099881776743fc72b56a37ba5f30d2e8a27044638ae2

```md
## ADDED Requirements

### Requirement: 余额显示
系统 SHALL 在状态栏数据行显示 provider 账户余额（如 "¥110.00"），异步刷新，不阻塞 UI。

#### Scenario: 余额可用
- **WHEN** provider 支持余额查询且查询成功
- **THEN** 状态栏显示余额（如 "¥110.00"）

#### Scenario: 余额不可用
- **WHEN** provider 不支持余额查询或查询失败
- **THEN** 余额不显示（静默降级）

#### Scenario: 余额刷新
- **WHEN** TUI 启动时和每个 turn 完成后
- **THEN** 余额异步刷新
```

## openspec/changes/tui-status-panels/specs/cache-hit-display/spec.md

- Source: openspec/changes/tui-status-panels/specs/cache-hit-display/spec.md
- Lines: 1-16
- SHA256: 975370d199afa703f284173fb445fcbeaad7b5f0d954ed6df549b2be41881507

```md
## ADDED Requirements

### Requirement: 缓存命中率显示
系统 SHALL 在状态栏数据行显示 prompt cache 命中率百分比（如 "cache 85%"），帮助用户了解 token 成本优化效果。

#### Scenario: 缓存命中率可用
- **WHEN** provider 支持缓存且返回缓存命中统计
- **THEN** 状态栏显示 "cache N%"（N 为命中率整数）

#### Scenario: 缓存命中率不可用
- **WHEN** provider 不支持缓存或无缓存统计数据
- **THEN** 缓存命中率不显示

#### Scenario: 缓存命中率更新
- **WHEN** 每个 turn 完成后
- **THEN** 缓存命中率刷新为最新值
```

## openspec/changes/tui-status-panels/specs/context-gauge/spec.md

- Source: openspec/changes/tui-status-panels/specs/context-gauge/spec.md
- Lines: 1-20
- SHA256: a67896dc109eb5fb42a494f162c81b407811b2d4cfa1a6be0934063a65c03c3b

```md
## ADDED Requirements

### Requirement: 上下文窗口使用率仪表
系统 SHALL 在状态栏数据行显示上下文窗口使用率，格式为 `ctx N/M (P%)`，其中 N 为已用 token 数，M 为窗口总 token 数，P 为百分比。颜色按压缩阈值变化：<50% 绿色、50-80% 黄色、>80% 红色。

#### Scenario: 上下文使用率正常
- **WHEN** 上下文使用率 < 50%
- **THEN** 仪表以绿色显示

#### Scenario: 上下文使用率接近阈值
- **WHEN** 上下文使用率在 50%-80% 之间
- **THEN** 仪表以黄色显示

#### Scenario: 上下文使用率触发压缩
- **WHEN** 上下文使用率 > 80%
- **THEN** 仪表以红色显示

#### Scenario: 上下文窗口信息不可用
- **WHEN** 无法获取上下文窗口信息（如 once 模式）
- **THEN** 仪表不显示
```

## openspec/changes/tui-status-panels/specs/custom-statusline/spec.md

- Source: openspec/changes/tui-status-panels/specs/custom-statusline/spec.md
- Lines: 1-16
- SHA256: 383a0dfe12be66446e930dac3e2f390ddd5e2d1c160c26f631d0281cffd97586

```md
## ADDED Requirements

### Requirement: 自定义状态行命令
系统 SHALL 支持用户通过配置指定一个外部命令，其 stdout 第一行替换内置数据行显示。

#### Scenario: 配置了自定义状态行命令
- **WHEN** 用户配置了 `statusline.command`
- **THEN** 状态栏数据行显示该命令的 stdout 第一行（而非内置数据行）

#### Scenario: 自定义命令执行失败
- **WHEN** 自定义状态行命令执行失败或超时
- **THEN** 数据行回退为内置显示

#### Scenario: 自定义命令刷新
- **WHEN** TUI 启动时和每个 turn 完成后
- **THEN** 自定义状态行命令被异步执行，结果更新到数据行
```

## openspec/changes/tui-status-panels/specs/git-status-display/spec.md

- Source: openspec/changes/tui-status-panels/specs/git-status-display/spec.md
- Lines: 1-24
- SHA256: 41515b0a540fa38c90b9ef838835595e2ed01fcd4354410645e484c7ebd34787

```md
## ADDED Requirements

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
```

## openspec/changes/tui-status-panels/specs/todo-panel/spec.md

- Source: openspec/changes/tui-status-panels/specs/todo-panel/spec.md
- Lines: 1-20
- SHA256: 091e0602aa1a3c61c4f24256462d149c00681bd6c91781d0ae559537954bae8d

```md
## ADDED Requirements

### Requirement: Todo 任务面板
系统 SHALL 在输入区上方渲染 Todo 任务面板，解析 agent 的 todo_write 工具调用，以结构化列表展示任务状态和名称。

#### Scenario: agent 调用 todo_write
- **WHEN** agent 发起 todo_write 工具调用
- **THEN** 系统解析 JSON 参数中的任务列表，在输入区上方渲染 Todo 面板

#### Scenario: Todo 面板渲染格式
- **WHEN** Todo 面板有任务
- **THEN** 每个任务显示状态图标（⏳ pending / ⟳ in_progress / ✓ completed）和任务名称，当前进行中的任务高亮

#### Scenario: 任务列表为空
- **WHEN** todo_write 参数中任务列表为空
- **THEN** Todo 面板不渲染（不占空间）

#### Scenario: 跨 turn 保持
- **WHEN** 一个 turn 完成且新的 turn 开始
- **THEN** Todo 面板保持上一次 todo_write 的状态，直到新的 todo_write 更新
```

## openspec/changes/tui-status-panels/specs/tui-chat-interface/spec.md

- Source: openspec/changes/tui-status-panels/specs/tui-chat-interface/spec.md
- Lines: 1-16
- SHA256: 1a5b52ca859a38222510576cde1f28ef64a54044908b4405d53bdf2967bffcf3

```md
## MODIFIED Requirements

### Requirement: 状态栏信息展示
系统 MUST 在底部状态栏显示三行信息布局：工作行（spinner + elapsed + token↓，仅运行中显示）、模式行（Plan/YOLO/Shell 标签 + effort + git 分支状态）、数据行（模型名 + 上下文仪表 + 缓存率 + 任务数 + 余额）。

#### Scenario: 运行中状态栏
- **WHEN** agent 正在处理 turn
- **THEN** 工作行显示 spinner 动画 + 已耗时间 + 输出 token 数（如 "⣾ thinking 12s · ↓1.2k"）；模式行显示当前模式标签；数据行显示模型名 + 上下文仪表 + 缓存率

#### Scenario: 空闲状态栏
- **WHEN** agent 空闲等待输入
- **THEN** 工作行隐藏；模式行显示模式标签 + effort + git 分支；数据行显示模型名 + 上下文仪表 + 缓存率 + 余额

#### Scenario: 小终端适配
- **WHEN** 终端宽度 < 80 列
- **THEN** 状态栏各行内容自然换行，不截断
```

