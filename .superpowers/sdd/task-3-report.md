# Task 3 Report — refine-tui-experience

Status: done

## RED

- command: `go test ./internal/tui/... -run 'TestSubmitSetsProcessing|TestStreamDoneClearsProcessing|TestStreamClosedResets|TestInterruptDoesNotAppend|TestStreamDoneClearsBusy|TestStreamErrorClearsBusy|TestEscInterrupt' -count=1 -v`
- summary: 新增 7 个验收测试后编译失败（`processingStatusMsg` 未定义）；补齐常量与状态机后，失败项为 submit 未设 processing `statusMsg`、done/closed/error 未清空 `statusMsg`、`streamClosedMsg` 未完整 reset。

## Implementation

- files: `internal/tui/model.go`, `internal/tui/runner_test.go`, `internal/tui/keymap_test.go`
- notes:
  - 新增 `processingStatusMsg`；`submit()` 进入 processing 状态（`busy` + `statusMsg`）。
  - `StreamDoneMsg` / `streamClosedMsg` / 非 interrupted 的 `StreamErrorMsg` 清空 `statusMsg` 并恢复可交互。
  - `streamClosedMsg` 同步清空 `turnCancel`、`interrupted`。
  - interrupted 路径保持 `statusMsg=已中断`，忽略后续 stream error，不写入 `lastError` 或消息正文。

## GREEN

- command: `go test ./internal/tui/... -run 'TestSubmitSetsProcessing|TestStreamDoneClearsProcessing|TestStreamClosedResets|TestInterruptDoesNotAppend|TestStreamDoneClearsBusy|TestStreamErrorClearsBusy|TestEscInterrupt' -count=1 -v`
- summary: 8 个匹配测试全部 PASS（含 `TestEscInterruptsBusyTurn` / `TestEscInterruptAllowsInputAfter`）；`go test ./internal/tui/... -count=1` 全包 PASS。

## Commits

- 36e1fd31580365383ae3ddec5a7bb55daafbfb8a

## Concerns

- none
