## 1. 修复补全菜单遮挡

- [x] 1.1 在 `internal/tui/statusbar.go` 的 `bottomHeight()` 中增加补全菜单高度

## 2. 添加欢迎界面

- [x] 2.1 在 `internal/tui/transcript.go` 的 `renderTranscriptContent()` 中空消息时渲染 welcome banner

## 3. 替换 "assistant:" 前缀

- [x] 3.1 将 `internal/tui/transcript.go` 中所有 "assistant: " 替换为 "> "
