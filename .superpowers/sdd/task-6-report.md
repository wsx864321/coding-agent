# Task 6 Report — refine-tui-experience

## Task

更新 TUI 文档（`docs/tui.md`），与 `internal/tui` 实现及测试一致。

## RED

**方式**：文档任务；以现有 `internal/tui` 测试套件作为行为契约（TDD skill 允许的文档主任务验证路径）。

**RED 证据（更新前）**：

- `docs/tui.md` 仅含启动、简略快捷键、能力/限制列表，**未**描述：
  - 布局顺序（消息 → 输入 → 错误 → 状态 → 帮助）
  - busy 输入锁与处理中输入区文案
  - `j`/`k` 与输入非空时的冲突规则
  - 错误/状态与消息区隔离
  - 长会话滚动、流式跟底、resize/状态变化时的 scroll 稳定
- 测试已覆盖上述行为（如 `TestViewLayoutOrderMessageBeforeInputBeforeErrorBeforeStatus`、`TestBusyBlocksInputEditing`、`TestJKTypeWhenInputNotEmpty`、`TestViewStatusAndErrorNotInMessagePane`、`TestStreamChunkKeepsBottomFollow` 等），文档与测试 **不一致**。

**基线测试**（文档修改前）：

```text
$ go test ./internal/tui/... -count=1
ok  	github.com/wsx864321/coding-agent/internal/tui	0.057s
```

## GREEN

**变更文件**：

- `docs/tui.md` — 增补「界面布局」「快捷键/冲突规则」「处理中与流式输出」「错误与恢复」「长会话滚动」等章节，逐项对应测试与 `view.go` / `model.go` 行为，不夸大未实现功能。

**GREEN 验证**（文档修改后）：

```text
$ go test ./internal/tui/... -count=1
ok  	github.com/wsx864321/coding-agent/internal/tui	0.057s
```

**文档 ↔ 测试映射**：

| 文档章节 | 代表测试 |
|----------|----------|
| 布局顺序 | `TestViewLayoutOrderMessageBeforeInputBeforeErrorBeforeStatus` |
| busy 输入锁 | `TestBusyBlocksInputEditing`, `TestViewBusyInputPaneShowsProcessingHint` |
| j/k 冲突 | `TestJKTypeWhenInputNotEmpty`, `TestScrollWithKAndJ` |
| 错误/状态分区 | `TestViewStatusAndErrorNotInMessagePane`, `TestViewShowsErrorBanner` |
| 错误恢复 | `TestRecoverableErrorHelpAffordance`, `TestSubmitAfterRecoverableErrorStartsNewTurn` |
| 中断 | `TestEscInterrupt`, `TestStreamErrorAfterInterruptDoesNotSetLastError` |
| 长会话滚动 | `TestViewScrollHidesOlderMessages`, `TestStreamChunkKeepsBottomFollow`, `TestWindowResizeClampsScrollOffset`, `TestStatusAppearanceStabilizesScroll` |

## Commits

- `docs(tui): document refined layout states and key behavior`

## Concerns

- 无代码变更；终端/lipgloss 在不同 OS 下的视觉细节未在文档中展开（与 design Non-Goals 一致）。
- 帮助行中文文案与测试断言字符串绑定；若未来改 copy 需同步测试或文档。
