## Why

TUI 存在三个体验问题需修复：
1. 斜杠命令补全菜单被底部状态栏遮挡，用户无法看到补全项
2. 首次进入 TUI 时界面空白，无欢迎引导
3. 助手回复前缀 "assistant:" 不够美观

## What Changes

- 修复 `bottomHeight()` 未计算补全菜单高度导致菜单被遮挡
- 空消息时渲染欢迎 banner（模型名、工作目录、快捷键）
- 将 "assistant:" 前缀替换为 "> "

## Impact

- `internal/tui/statusbar.go`: bottomHeight 增加补全菜单行数
- `internal/tui/transcript.go`: 欢迎界面 + 前缀替换
