---
comet_change: tui-status-panels
role: technical-design
canonical_spec: openspec
---

# TUI 状态栏与信息面板重构 — 技术设计

## 1. 三行状态栏布局

### 1.1 布局结构

```
工作行（仅 busy）: ⣾ thinking 12s · ↓1.2k
模式行（始终）:    Plan · deepseek-flash · high · main ↑3
数据行（始终）:    ctx 45K/128K (35%) · cache 85% · 2 jobs · ¥110.00
```

### 1.2 实现

重写 `internal/tui/statusbar.go`：

```go
func renderWorkingLine(m Model) string   // busy 时渲染，idle 时返回 ""
func renderModeLine(m Model) string      // 始终渲染
func renderDataLine(m Model) string      // 始终渲染
```

`bottomHeight()` 动态计算：
```go
func (m Model) bottomHeight() int {
    h := 0
    if m.busy { h++ }          // 工作行
    h += 2                      // 模式行 + 数据行
    if m.todoArgs != "" { h++ } // Todo 面板
    h += m.textarea.Height()    // 输入区
    h += 1                      // 帮助行
    return h
}
```

## 2. 上下文窗口仪表

### 2.1 数据来源

在 Agent 上暴露接口：
```go
type ContextSnapshot interface {
    ContextSnapshot() (used int, window int)
}
```

### 2.2 渲染

```go
func renderContextGauge(used, window int) string {
    if window == 0 { return "" }
    pct := used * 100 / window
    color := gaugeColor(pct) // <50 green, 50-80 yellow, >80 red
    return color(fmt.Sprintf("ctx %s/%s (%d%%)", shortTokens(used), shortTokens(window), pct))
}
```

### 2.3 刷新时机

每个 turn 完成后通过 `ContextSnapshot()` 获取最新值。

## 3. Git 状态显示

### 3.1 异步读取

```go
func fetchGitStatus() tea.Cmd {
    return func() tea.Msg {
        branch := runGitCmd("rev-parse", "--abbrev-ref", "HEAD")
        status := runGitCmd("status", "--porcelain")
        return gitStatusMsg{status: gitStatus{
            branch: branch,
            dirty:  status != "",
        }}
    }
}
```

### 3.2 渲染

- clean: `main`
- dirty: `main *`
- ahead: `main ↑3`

### 3.3 刷新时机

启动时 + 每个 turn 完成后。

## 4. Todo 任务面板

### 4.1 解析

在 ToolDispatch 处理中识别 `todo_write` 工具，解析 JSON：
```go
type todoItem struct {
    Content    string `json:"content"`
    Status     string `json:"status"`
    ActiveForm string `json:"activeForm"`
}
```

### 4.2 渲染

```
⏳ task1 · ⟳ task2 · ✓ task3
```

- pending → ⏳
- in_progress → ⟳（高亮）
- completed → ✓（dim）

## 5. 余额显示

### 5.1 异步刷新

```go
func fetchBalance(ctrl *Controller) tea.Cmd {
    return func() tea.Msg {
        b, err := ctrl.Balance(context.Background())
        if err != nil || b == nil {
            return balanceMsg{}
        }
        return balanceMsg{text: b.Display()}
    }
}
```

## 6. 自定义状态行

### 6.1 配置

```toml
[statusline]
command = "my-statusline.sh"
```

### 6.2 执行

```go
func runStatusline(cmd string, ctxJSON string) tea.Cmd {
    return func() tea.Msg {
        // 执行 cmd，stdin 传入 ctxJSON
        // 读取 stdout 第一行
        // 超时 2s
        return statuslineMsg{out: firstLine}
    }
}
```

## 7. 测试策略

| 类型 | 覆盖 |
|------|------|
| 单元测试 | 三行布局渲染、上下文仪表着色、Todo 解析 |
| 集成测试 | Git 状态刷新、余额刷新、自定义状态行 |
| 回归测试 | `go test ./internal/tui/...` |

## 8. 文件变更清单

| 文件 | 变更类型 |
|------|---------|
| `internal/tui/statusbar.go` | 重写：三行布局 |
| `internal/tui/model.go` | 修改：新增 gitStatus、balance、contextUsed/Window、todoArgs 等字段 |
| `internal/tui/view.go` | 修改：bottomHeight 计算 |
| `internal/tui/todopanel.go` | 新增：Todo 面板渲染 |
