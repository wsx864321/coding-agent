# Task 1 Report — refine-tui-experience

Status: done

## RED

- command: `go test ./internal/tui/... -run 'TestViewLayout|TestViewBusy|TestViewStatusAndError|TestViewRendersMessageInputAndHelpAreas' -count=1 -v`
- summary: `TestViewBusyInputPaneShowsProcessingHint` FAIL — busy 时仍渲染 `> should-not-show` 草稿，未显示处理中提示；其余 3 个测试 PASS（布局顺序与消息区隔离已满足）。

## Implementation

- files:
  - `internal/tui/view_test.go`（新建：布局顺序、busy 输入栏、消息区隔离测试）
  - `internal/tui/view.go`（新增 `renderInputPane` / `renderHelp`，`View()` 使用分区渲染）
- notes: busy 时输入栏显示 `> (处理中，Esc 可中断)`；帮助行按 busy/错误/中断状态切换；布局顺序固定为标题 → 消息 → 输入 → 错误 → 状态 → 帮助。

## GREEN

- command: `go test ./internal/tui/... -run 'TestViewLayout|TestViewBusy|TestViewStatusAndError|TestViewRendersMessageInputAndHelpAreas' -count=1 -v`
- summary: 4 个测试全部 PASS；`go test ./internal/tui/... -count=1` 全包通过。

## Commits

- 2e88c19 feat(tui): refine message/input/status/error layout hierarchy

## Concerns

- none
