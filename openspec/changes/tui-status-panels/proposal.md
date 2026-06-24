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
