## Context

TUI 的三个体验瑕疵，修复方案均为局部代码调整。

## Fix 1: 补全菜单被遮挡

**根因**: `bottomHeight()` 计算底部固定区域高度时未包含补全菜单，viewport 占用全部剩余空间，菜单被推到屏幕外。

**修复**: 在 `bottomHeight()` 中，当 `m.completion.active` 且 `len(m.completion.items) > 0` 时，增加 `len(items) + 2`（border/padding）行。

## Fix 2: 欢迎界面

**修复**: 修改 `renderTranscriptContent()`，空消息时渲染 welcome banner 替代 "(暂无消息)"。

Banner 内容：模型名 + 工作目录 + 快捷键列表（居中或左对齐）。

## Fix 3: 前缀替换

**修复**: 全局替换 `"assistant: "` → `"> "`，更新 `assistantInnerWidth()` 的前缀宽度计算。
