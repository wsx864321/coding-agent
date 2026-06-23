## 1. Bubble Tea v2 迁移 + 基础组件引入

- [x] 1.1 升级 go.mod 依赖：bubbletea v1→v2、引入 bubbles/v2、lipgloss/v2、go-runewidth
- [x] 1.2 迁移 `internal/tui/model.go` 到 v2 API：Model.View() 返回 tea.View、tea.KeyMsg 适配、tea.WindowSizeMsg 适配
- [x] 1.3 引入 bubbles/v2 textarea 替代 string 输入：配置 Shift+Enter 换行、Enter 提交、动态高度（1-5 行）、CharLimit、IME 支持
- [x] 1.4 引入 bubbles/v2 viewport 替代手动滚动：鼠标滚轮、PgUp/PgDn/Home/End、滚动条、tail-follow（流式时自动跟底）
- [x] 1.5 引入 bubbles/v2 spinner：配置 spinner 样式、在 busy 状态显示动画 + 耗时计数
- [x] 1.6 迁移 `cmd/cli/tui.go`：适配 v2 的 tea.NewProgram 选项（WithAltScreen → tea.View.AltScreen）
- [x] 1.7 修复所有现有测试适配 v2 API（model_test.go、keymap_test.go、runner_test.go）

## 2. CJK 显示宽度修复

- [x] 2.1 引入 `go-runewidth` 依赖，重写 `wrapText` 函数使用 `runewidth.RuneWidth()` 计算显示宽度
- [x] 2.2 更新 `renderMessageLines` 中 prefix 宽度计算为显示宽度
- [x] 2.3 添加 CJK 换行的单元测试（中文、日文、emoji 混合场景）

## 3. 事件系统扩展（工具调用 + 审批）

- [x] 3.1 扩展 `StreamEmitter` 接口：增加 OnToolStart(name, args)、OnToolEnd(name, result, err)、OnApprovalRequest(name, args, respond)
- [x] 3.2 定义新的 tea.Msg 类型：ToolStartMsg、ToolEndMsg、ApprovalRequestMsg、ApprovalResponseMsg
- [x] 3.3 修改 `internal/agent/loop.go` 的 loopStepWithText：在 invokeTool 前后调用 emitter 的 OnToolStart/OnToolEnd
- [ ] 3.4 修改 `internal/agent/loop.go` 的 invokeTool：在 permission.Check 判定需要确认时调用 OnApprovalRequest 并等待 respond 回调
- [x] 3.5 更新 `cmd/cli/tui_runner.go`：适配扩展后的 StreamEmitter 接口，传递所有事件到 channel
- [x] 3.6 更新 chanEmitter：增加 OnToolStart、OnToolEnd、OnApprovalRequest 的 channel 实现

## 4. 工具调用可视化（TUI 侧）

- [x] 4.1 在 Model 中增加工具调用状态跟踪（activeTools、toolResults）
- [x] 4.2 在 Update 中处理 ToolStartMsg 和 ToolEndMsg：更新 transcript、切换 spinner 文案
- [x] 4.3 实现 renderToolCard：工具卡片渲染（`● ToolName("args summary")`），包含颜色标记和参数截断
- [x] 4.4 实现工具输出折叠展示：结果超过 8 行时折叠，显示行数摘要

## 5. 审批交互（TUI 侧）

- [ ] 5.1 在 Model 中增加 pendingApproval 模态状态（工具名、参数、respond 回调）
- [ ] 5.2 在 Update 中处理 ApprovalRequestMsg：进入审批模态，拦截按键路由
- [ ] 5.3 实现 renderApprovalBanner：显示工具名、参数摘要、`[y]es [n]o` 选项
- [ ] 5.4 处理审批按键（y/n）：调用 respond 回调（sync.Once 保护），退出审批模态
- [ ] 5.5 修改 `cmd/cli/chat_setup.go`：TUI 模式不再使用全自动拒绝 AskerFunc，改为由 TUI 审批

## 6. Markdown ANSI 渲染

- [ ] 6.1 引入 goldmark + chroma/v2 依赖
- [ ] 6.2 实现 mdRenderer 结构体：goldmark AST walker → lipgloss-styled ANSI 字符串
- [ ] 6.3 实现基础元素渲染：标题（粗体+颜色）、段落、粗体/斜体、内联代码（背景色）
- [ ] 6.4 实现列表渲染：有序列表（数字前缀）、无序列表（bullet 前缀）、嵌套缩进
- [ ] 6.5 实现代码块渲染：fenced code block + chroma 语法高亮 + 缩进 gutter
- [ ] 6.6 实现引用块渲染：`│` 前缀 + dim 样式
- [ ] 6.7 实现 GFM 表格渲染：列对齐、表头加粗、边框字符
- [ ] 6.8 添加 Markdown 渲染单元测试（各元素类型 + 嵌套组合）

## 7. 流式 Markdown 渲染优化

- [ ] 7.1 实现 flushableMarkdownPrefix：检测段落边界（空行 + 不在 fenced block 内），返回可安全渲染的前缀
- [ ] 7.2 重构流式更新逻辑：StreamChunkMsg 追加到 pending buffer → flushablePrefix → mdRenderer.Render → 写入 transcript
- [ ] 7.3 在 StreamDoneMsg 时刷新剩余 pending 内容
- [ ] 7.4 添加流式渲染的单元测试（半代码块、跨段落、空输出等边界场景）

## 8. 布局 + 状态栏 + View 整合

- [ ] 8.1 重构 View 函数：三区布局（viewport + 状态/审批 + textarea + help）
- [ ] 8.2 实现 bottomHeight 动态计算：textarea 高度 + 状态栏 + help + 可选审批横幅
- [ ] 8.3 实现状态栏渲染：模型名称 + busy/idle 状态 + 可选耗时
- [ ] 8.4 处理 WindowSizeMsg：调整 viewport/textarea 尺寸
- [ ] 8.5 更新帮助文本：反映新的快捷键映射

## 9. 集成测试 + 回归验证

- [ ] 9.1 更新所有现有测试适配新的 Model 结构（textarea/viewport/spinner 嵌入）
- [ ] 9.2 添加工具调用事件流测试：ToolStartMsg → spinner 文案变化 → ToolEndMsg → 工具卡片渲染
- [ ] 9.3 添加审批流程测试：ApprovalRequestMsg → 审批模态 → y/n 响应 → 恢复正常状态
- [ ] 9.4 添加 CJK + Markdown 渲染组合测试
- [ ] 9.5 确认 `go build ./cmd` 编译通过、`go test ./...` 全部通过
