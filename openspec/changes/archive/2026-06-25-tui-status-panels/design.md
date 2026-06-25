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
