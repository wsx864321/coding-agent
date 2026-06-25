---
comet_change: tui-input-system
role: technical-design
canonical_spec: openspec
archived-with: 2026-06-25-tui-input-system
status: final
---

# TUI 输入系统与交互增强 — 技术设计

## 1. 斜杠命令自动补全

### 1.1 Completion 结构体

```go
type completion struct {
    items    []string
    selected int
    active   bool
}
```

### 1.2 触发与过滤

- 输入 `/` 时 `active = true`
- 根据 `/` 后文本前缀过滤可用命令列表
- 命令列表从 slash commands 注册表获取（/help、/skills、/model 等 + 自定义命令）

### 1.3 键盘交互

- ↑↓：移动选中项
- Tab/Enter：接受补全，替换输入区文本
- Esc：关闭菜单

### 1.4 渲染

在输入区上方渲染补全菜单（lipgloss 列表样式），高亮当前选中项。

## 2. 输入历史回溯

### 2.1 数据结构

```go
submittedInputs      []string // 已发送消息历史
submittedInputCursor int      // 当前回溯位置，-1 表示未回溯
```

### 2.2 交互

- 空闲时 ↑：回溯到上一条消息
- 空闲时 ↓：回溯到下一条（或回到空白）
- 编辑后 Enter：作为新消息发送，不修改历史
- 输入新字符：重置回溯光标

## 3. 运行中排队输入

### 3.1 数据结构

```go
pendingInterject []string // 排队消息队列
```

### 3.2 交互

- 运行中 Enter：追加到队列，显示 "feedback queued"
- TurnDone：自动发送队首消息
- 多条排队：显示 "N queued"

## 4. 剪贴板粘贴

### 4.1 Ctrl+V 处理

```go
case "ctrl+v":
    text := clipboard.ReadAll()
    if len(text) > 500 {
        m.insertFoldedPaste(text) // 折叠显示
    } else {
        m.input.InsertString(text)
    }
```

### 4.2 图片粘贴

- 检测剪贴板图片数据
- 保存为临时文件
- 插入 `@/tmp/image-xxx.png` 引用

## 5. @文件引用解析

### 5.1 触发

- 输入 `@` 后跟部分文件名
- 基于工作目录 glob/walk 搜索匹配文件
- 限制搜索深度和结果数量

### 5.2 接受

- 选择文件后插入 `@path/to/file` 引用
- 发送时解析为文件内容

## 6. #快速记忆

### 6.1 格式检测

```go
if strings.HasPrefix(line, "# ") {
    note := strings.TrimPrefix(line, "# ")
    path, err := ctrl.QuickAdd(memory.ScopeProject, note)
    // 显示确认提示
}
```

## 7. !shell 直接执行

### 7.1 格式检测

```go
if strings.HasPrefix(line, "!") {
    cmd := strings.TrimPrefix(line, "!")
    ctrl.RunShell(cmd)
    // 输入区边框变色
}
```

## 8. Plan/YOLO 模式切换

### 8.1 状态字段

```go
planMode bool
yoloMode bool
```

### 8.2 快捷键

- Shift+Tab：切换 Plan 模式
- Ctrl+Y：切换 YOLO 模式

### 8.3 视觉指示

- Plan：状态栏蓝色 "Plan" 标签
- YOLO：状态栏红色 "YOLO" 标签
- Plan 模式下用户消息前添加标记

## 9. 测试策略

| 类型 | 覆盖 |
|------|------|
| 单元测试 | 补全过滤、历史回溯、排队队列、模式切换 |
| 集成测试 | 斜杠命令流程、@引用解析、#记忆写入、!shell 执行 |
| 回归测试 | `go test ./internal/tui/...` |

## 10. 文件变更清单

| 文件 | 变更类型 |
|------|---------|
| `internal/tui/model.go` | 修改：新增 completion、history、queue、mode 字段 + 按键处理 |
| `internal/tui/completion.go` | 新增：补全菜单渲染与逻辑 |
| `internal/tui/components.go` | 修改：textarea 自适应高度配置 |
| `cmd/cli/tui.go` | 修改：传递 slash commands 列表 |
