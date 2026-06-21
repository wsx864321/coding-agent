# Task 2 Report — refine-tui-experience

Status: done

## RED

- command: `go test ./internal/tui/... -run 'TestStatusAppearance' -count=1 -v`
- summary: 增强 `TestStatusAppearanceStabilizesScroll`（中断前注入 `scrollOffset = maxBefore+3`）后失败：`scrollOffset=26 out of range [0,24] after interrupt status`，证明 status 行出现后需重新钳制。

## Implementation

- files: `internal/tui/model.go`, `internal/tui/model_test.go`
- notes: 新增 `stabilizeScroll()`；在 `interruptTurn`、`StreamErrorMsg`、`streamClosedMsg`、`submit` 调用；`WindowSizeMsg` 保留 `clampScroll`；`StreamChunkMsg` 保留 `clampScrollToBottom` 以跟随底部。

## GREEN

- command: `go test ./internal/tui/... -run 'TestWindowResizeClamps|TestStatusAppearance|TestStreamChunkKeeps|TestScroll' -count=1 -v`
- summary: 全部 PASS（含 5 个 TestScroll* 与 3 个 Task 2 验收测试）；完整包 `go test ./internal/tui/...` 亦 PASS。

## Commits

- 0fbc221346e9e235633e71dc557e73342837cf77

## Concerns

- none
