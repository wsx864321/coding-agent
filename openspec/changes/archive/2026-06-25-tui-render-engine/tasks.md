## 1. 事件系统扩展

- [x] 1.1 在 `internal/event/event.go` 中新增 `ReasoningText` 事件类型（Kind 枚举 + Text 字段复用）
- [x] 1.2 在 `internal/event/event.go` 中新增 `ToolProgress` 事件类型（Kind 枚举 + ToolCallID + Chunk 字段）
- [x] 1.3 在 `internal/tui/sink.go` 中确保新事件类型正确转发到 TUI channel

## 2. 推理文本渲染

- [x] 2.1 在 `internal/tui/model.go` 中新增 reasoning 相关状态字段（reasoning builder、reasoningLineIdx、showReasoning、thinkStart）
- [x] 2.2 在 `internal/tui/model.go` 的 Update 中处理 `ReasoningText` 事件：累积推理文本、更新摘要行
- [x] 2.3 在 `internal/tui/transcript.go` 中实现推理块渲染（折叠摘要行 + dim 样式展开内容）
- [x] 2.4 在 `internal/tui/model.go` 中实现 Ctrl+O 切换 `showReasoning` 状态
- [x] 2.5 在 `internal/tui/model.go` 中处理推理完成（Text 事件到达时提交推理摘要行）

## 3. 工具流式输出

- [x] 3.1 在 `internal/tui/model.go` 中新增流式输出状态字段（toolStreamIdx、toolStreamID、toolTail、toolPartial、toolLineCount）
- [x] 3.2 在 `internal/tui/model.go` 的 Update 中处理 `ToolProgress` 事件：追加 chunk、尾部截断、更新行数计数
- [x] 3.3 在 `internal/tui/toolcard.go` 中实现流式输出块渲染（"⎿ working · Ns" + 尾部行 + 行数摘要）
- [x] 3.4 在 `internal/tui/model.go` 中处理 ToolResult 事件时将流式输出转为折叠摘要
- [x] 3.5 实现事件合并（drain loop）：高频 ToolProgress 事件批量处理，每帧最多重渲染一次

## 4. Shell 输出折叠/展开

- [x] 4.1 在 `internal/tui/model.go` 中新增 `shellOutputs map[string]string` 和 `shellExpanded map[string]bool`
- [x] 4.2 在 `internal/tui/model.go` 的 ToolResult 处理中识别 Shell 工具（bash），存储完整输出到 shellOutputs
- [x] 4.3 在 `internal/tui/toolcard.go` 中实现 Shell 输出折叠渲染（前 8 行 + "⎿ N lines, collapsed"）
- [x] 4.4 在 `internal/tui/model.go` 中实现 Ctrl+B 切换最近 Shell 输出块的展开/折叠
- [x] 4.5 在 `internal/tui/transcript.go` 中实现展开/折叠时原地重写 transcript 块

## 5. Markdown 渲染升级

- [x] 5.1 在 `internal/tui/markdown.go` 中配置 glamour 启用 chroma 语法高亮（`glamour.WithChromaStyle()`）
- [x] 5.2 验证代码块语法高亮在常见语言（Go、Python、JavaScript、Bash、Diff）上正确工作
- [x] 5.3 在 `internal/tui/markdown.go` 中新增 Diff 视图渲染：检测 diff 格式代码块，对 +/- 行叠加 lipgloss 着色
- [x] 5.4 在 `internal/tui/transcript.go` 的 `renderEntry` 中集成 diff 视图检测与渲染

## 6. 文本选择与复制

- [x] 6.1 在 `internal/tui/model.go` 中新增 `selection` 结构体（startLine、startCol、endLine、endCol、active）
- [x] 6.2 在 `internal/tui/model.go` 的 Update 中处理 MouseMsg：左键按下开始选择、拖动扩展、释放结束
- [x] 6.3 在 `internal/tui/view.go` 中实现选择区域的反色高亮渲染
- [x] 6.4 在 `internal/tui/model.go` 中实现 Ctrl+C 在选中时复制到剪贴板（使用 `github.com/atotto/clipboard`）
- [x] 6.5 处理选择与滚动的交互（选择时滚轮/PgUp/PgDn 扩展选择范围）
- [x] 6.6 添加 `github.com/atotto/clipboard` 依赖到 go.mod

## 7. Diff 视图渲染

- [x] 7.1 在 `internal/tui/model.go` 中新增 `diffMaxLines` 字段（默认 0 = 不限制）
- [x] 7.2 在 `internal/tui/toolcard.go` 或新建 `internal/tui/diffview.go` 中实现 diff 格式检测与 +/- 着色
- [x] 7.3 实现 `/diff-fold` 斜杠命令处理（在 TUI 输入中识别并更新 diffMaxLines）
- [x] 7.4 实现 diff 块超过阈值时的折叠渲染（"N more lines" 提示）

## 8. 集成测试与验证

- [x] 8.1 为推理文本渲染编写单元测试（ReasoningText 事件处理、折叠/展开切换）
- [x] 8.2 为工具流式输出编写单元测试（ToolProgress 事件处理、尾部截断、行数计数）
- [x] 8.3 为 Shell 输出折叠编写单元测试（存储、展开/折叠、Ctrl+B 切换）
- [x] 8.4 为文本选择编写单元测试（选择状态机、剪贴板复制）
- [x] 8.5 为 Diff 视图编写单元测试（格式检测、着色、折叠）
- [x] 8.6 运行全量测试套件确认无回归
