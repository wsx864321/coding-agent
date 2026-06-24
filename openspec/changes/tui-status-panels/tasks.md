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
