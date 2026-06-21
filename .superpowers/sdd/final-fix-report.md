# Final Fix Report — refine-tui-experience

Status: complete

## Findings Addressed

1. **`messageViewportHeight()` overhead 低估** — 基线 overhead 从 5 改为 6（title + 双分隔 + input + help）；error/status 各 +2（blank + 行），与 `View()` 实际布局一致。
2. **`StreamDoneMsg` / `streamClosedMsg` 滚动收尾不一致** — `StreamDoneMsg` 由 `clampScrollToBottom()` 改为 `stabilizeScroll()`，与 `streamClosedMsg` 及实现计划一致。
3. **`docs/tui.md` “提交即跟底” 超前** — 文档改为：流式 chunk 自动滚底；提交新轮仅钳制滚动、不强制跳底。

## Changes

- `internal/tui/model.go`: 修正 `messageViewportHeight()` overhead；`StreamDoneMsg` 使用 `stabilizeScroll()`。
- `internal/tui/model_test.go`: 新增 5 个回归测试（viewport overhead、view 行数、StreamDone/streamClosed/submit 滚动策略）。
- `docs/tui.md`: 长会话滚动段落与真实行为对齐。

## Validation

```text
go test ./internal/tui/... -count=1 -v
PASS — 47 tests, ok github.com/wsx864321/coding-agent/internal/tui
```

TDD: 先写失败测试（RED），确认 3 项失败原因符合预期后实现（GREEN），全包 PASS。

## Commits

- `694cb37` — fix(tui): correct viewport overhead and unify stream-end scroll

## Residual Notes

- 极矮终端（`height <= overhead`）仍回退 viewport=1；未单独测 lipgloss 样式换行对行高的影响。
- 提交后不强制跟底是刻意行为；若产品希望“提交即看最新”，需另开变更改为 `clampScrollToBottom()` 并更新文档/测试。
