---
comet_change: tui-overlays
role: technical-design
canonical_spec: openspec
archived-with: 2026-06-25-tui-overlays
status: final
---

# TUI 覆盖层与模态系统 — 技术设计

## 1. 覆盖层框架

### 1.1 接口设计

```go
type Overlay interface {
    Update(msg tea.KeyPressMsg) (tea.Cmd, bool) // bool = consumed
    View(width int) string
    Active() bool
}
```

### 1.2 路由优先级

在 Model.Update 中按优先级检查：
1. chooser（Ask 卡片）
2. rewindPicker
3. mcpImport
4. resumePicker
5. mcpManager
6. clearConfirm
7. skillPicker
8. pendingApproval（审批横幅）
9. completion（补全菜单）

### 1.3 渲染位置

覆盖层渲染在 viewport 和输入区之间，占用 `transcriptHeight` 的一部分。

## 2. Skill 选择器

### 2.1 结构体

```go
type skillPicker struct {
    skills   []skill.Skill
    selected int
    detail   bool // 是否显示详情
}
```

### 2.2 交互

- ↑↓：导航
- Enter：查看详情 / 返回列表
- Esc：关闭

## 3. MCP 管理器

### 3.1 结构体

```go
type mcpManager struct {
    servers  []mcpServerInfo
    selected int
}
```

### 3.2 交互

- ↑↓：导航
- Enter：连接/断开
- R：重连
- Esc：关闭

## 4. 会话恢复选择器

### 4.1 结构体

```go
type resumePicker struct {
    sessions []sessionInfo
    selected int
}
```

### 4.2 交互

- ↑↓：导航
- Enter：恢复会话
- Esc：关闭

## 5. Rewind 检查点选择器

### 5.1 触发

- 输入区为空 + 600ms 内双击 Esc

### 5.2 结构体

```go
type rewindPicker struct {
    checkpoints []checkpointInfo
    selected    int
}
```

### 5.3 交互

- ↑↓：导航
- Enter：回退到检查点
- Esc：关闭

## 6. 模型切换器

### 6.1 结构体

```go
type modelPicker struct {
    models   []modelInfo
    selected int
    current  string
}
```

### 6.2 交互

- ↑↓：导航
- Enter：切换模型
- Esc：关闭

## 7. /clear 确认对话框

### 7.1 结构体

```go
type clearConfirm struct{}
```

### 7.2 渲染

```
Clear session? This cannot be undone. [y]es [n]o
```

### 7.3 交互

- y：清除会话
- n/Esc：取消

## 8. Ask 多选题卡片

### 8.1 结构体

```go
type chooser struct {
    question    string
    options     []askOption
    multiSelect bool
    selected    map[int]bool
    custom      map[int]string
    typing      bool
    tab         int
}
```

### 8.2 交互

- 单选：数字键选择
- 多选：空格切换 + Enter 确认
- 自由输入：键入文本 + Enter 提交
- Esc：关闭，返回空答案

## 9. 审批横幅增强

### 9.1 渲染

```
Allow Bash("go test ./...")? [y]es [n]o [a]lways
```

### 9.2 敏感参数脱敏

```go
var sensitiveKeys = []string{"password", "token", "secret", "api_key"}
// 脱敏后显示 "***"
```

## 10. 测试策略

| 类型 | 覆盖 |
|------|------|
| 单元测试 | 覆盖层路由、各覆盖层键盘交互 |
| 集成测试 | Skill 选择器、Ask 卡片、审批横幅 |
| 回归测试 | `go test ./internal/tui/...` |

## 11. 文件变更清单

| 文件 | 变更类型 |
|------|---------|
| `internal/tui/model.go` | 修改：新增覆盖层字段 + 路由逻辑 |
| `internal/tui/overlays/skillpicker.go` | 新增 |
| `internal/tui/overlays/mcpmanager.go` | 新增 |
| `internal/tui/overlays/resumepicker.go` | 新增 |
| `internal/tui/overlays/rewindpicker.go` | 新增 |
| `internal/tui/overlays/modelpicker.go` | 新增 |
| `internal/tui/overlays/clearconfirm.go` | 新增 |
| `internal/tui/overlays/chooser.go` | 新增 |
| `internal/tui/approval.go` | 修改：增强审批横幅 |
| `internal/tui/view.go` | 修改：集成覆盖层渲染 |
