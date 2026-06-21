# Task 4 Report — next-action affordance after error/interrupt

## RED

Command:

```bash
go test ./internal/tui/... -run 'TestSubmitAfterRecoverable|TestBusyBlocksInput|TestRecoverableErrorHelp|TestEscInterruptAllowsInputAfter' -count=1 -v
```

Result (after adding planned tests, before fix):

```
=== RUN   TestBusyBlocksInputEditing
--- PASS: TestBusyBlocksInputEditing (0.00s)
=== RUN   TestRecoverableErrorHelpAffordance
--- PASS: TestRecoverableErrorHelpAffordance (0.00s)
=== RUN   TestEscInterruptAllowsInputAfter
--- PASS: TestEscInterruptAllowsInputAfter (0.00s)
=== RUN   TestSubmitAfterRecoverableErrorStartsNewTurn
    runner_test.go:263: runner prompt="", want try again
--- FAIL: TestSubmitAfterRecoverableErrorStartsNewTurn (0.00s)
FAIL
```

Summary: three keymap/affordance tests already green; `TestSubmitAfterRecoverableErrorStartsNewTurn` failed because the test checked `runner.prompt` before invoking the returned `tea.Cmd` (Bubble Tea only starts the async runner after `cmd()` is called, same pattern as `TestEnterSubmitEndToEndWithStubRunner`).

## GREEN

Command:

```bash
go test ./internal/tui/... -run 'TestSubmitAfterRecoverable|TestBusyBlocksInput|TestRecoverableErrorHelp|TestEscInterruptAllowsInputAfter' -count=1 -v
```

Result:

```
=== RUN   TestBusyBlocksInputEditing
--- PASS: TestBusyBlocksInputEditing (0.00s)
=== RUN   TestRecoverableErrorHelpAffordance
--- PASS: TestRecoverableErrorHelpAffordance (0.00s)
=== RUN   TestEscInterruptAllowsInputAfter
--- PASS: TestEscInterruptAllowsInputAfter (0.00s)
=== RUN   TestSubmitAfterRecoverableErrorStartsNewTurn
--- PASS: TestSubmitAfterRecoverableErrorStartsNewTurn (0.00s)
PASS
ok  	github.com/wsx864321/coding-agent/internal/tui	0.691s
```

Fix: invoke `cmd()` after Enter in `TestSubmitAfterRecoverableErrorStartsNewTurn` before asserting `runner.prompt`. No `model.go` changes required — existing `submit()` clears `lastError`, `Update` guards `!m.busy` for input editing, and `renderHelp()` (view layer) already shows “可继续输入并 Enter 发送” for error/interrupted states.

## Files

| File | Change |
|------|--------|
| `internal/tui/keymap_test.go` | Added `TestBusyBlocksInputEditing`, `TestRecoverableErrorHelpAffordance` |
| `internal/tui/runner_test.go` | Added `TestSubmitAfterRecoverableErrorStartsNewTurn` (with `cmd()` dispatch) |
| `internal/tui/model.go` | No change — behavior already met spec |

## Commit

```
feat(tui): ensure next-action affordance after error or interrupt
```

## Concerns

- Plan snippet for `TestSubmitAfterRecoverableErrorStartsNewTurn` omits `cmd()` dispatch; without it the test is flaky/fails under Go’s scheduler because `stubRunner.RunTurn` runs in a goroutine started by `submit()` but only reliably observed after the stream command executes (mirrors real Bubble Tea runtime).
- Interrupted-state help affordance is covered by existing `renderHelp()` logic (Task 1); this task only adds `TestRecoverableErrorHelpAffordance` for error state, not a separate interrupted-help test.
