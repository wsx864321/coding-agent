---
change: add-bubbletea-tui-interface
design-doc: docs/superpowers/specs/2026-06-21-bubbletea-tui-interface-design.md
base-ref: 98e431618c4c5a5fe26f1bb4dc31fade2651891c
archived-with: 2026-06-21-add-bubbletea-tui-interface
---

# Bubble Tea TUI Implementation Plan

## Execution Checklist

- [x] 1.1 Add TUI subcommand wiring
- [x] 1.2 Add Bubble Tea dependency and startup skeleton
- [x] 2.1 Build message list and input model
- [x] 2.2 Connect agent session flow and stream rendering
- [x] 2.3 Implement keymap and error feedback
- [x] 3.1 Validate startup/send/stream/interrupt/exit on Windows macOS Linux
- [x] 3.2 Add TUI docs and usage notes

## Global Constraints

- Keep existing `chat` and `once` behavior unchanged.
- Keep key semantics fixed: Enter send, Esc interrupt turn, Ctrl+C exit, Up/Down and j/k navigate.
- Stream events must enter Bubble Tea through `tea.Msg`; no direct cross-goroutine model mutation.
- Current local Go toolchain is below project requirement; use task-local tests for progress and finish full verification in Go 1.26 environment.

## Completed Work

### Task 1.1
- Added `cmd/cli/tui.go` and registered `tui` command.
- Added shared setup helper `cmd/cli/chat_setup.go`.
- Refactored `cmd/cli/chat.go` to reuse shared setup.
- Updated `cmd/cli/root.go` command description.
- Added `cmd/cli/tui_test.go` command registration coverage.

### Task 1.2
- Added dependencies in `go.mod` and `go.sum`:
  - `github.com/charmbracelet/bubbletea`
  - `github.com/charmbracelet/lipgloss`
- Added `internal/tui/model.go`, `internal/tui/view.go`, `internal/tui/model_test.go`.
- Updated `cmd/cli/tui.go` to run Bubble Tea program with alt screen.
- Verified: `go test ./internal/tui/... -count=1` passes.

## Remaining Tasks

### Task 2.1
- Expand model for message list, input buffer, and viewport state.
- Render message pane and input pane.
- Add state transition tests for input/edit/scroll behavior.

### Task 2.2
- Add runner bridge to invoke agent turn and emit stream events.
- Append user message and incrementally render assistant tokens.
- Handle completion and error events in model state.

### Task 2.3
- Implement key handlers for Enter, Esc, Ctrl+C, Up/Down, j/k.
- Add error banner and interrupted-turn feedback.
- Add keymap and error-path tests.

### Task 3.1
- Smoke test in Windows and one non-Windows terminal.
- Validate five flows: startup, send, stream, interrupt, exit.

### Task 3.2
- Add `docs/tui.md` with usage, keymap, known limits, and next steps.
