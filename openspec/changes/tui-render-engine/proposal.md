## Why

当前 TUI 的渲染能力仅覆盖基础的消息流、工具卡片和 Markdown 渲染，与 Reasonix 等成熟终端 AI 编码助手的渲染体验存在显著差距。具体表现为：无思考/推理文本展示、工具输出仅展示最终结果而无实时流式反馈、Shell 输出无法交互式折叠/展开、Markdown 代码块缺少语法高亮、无文本选择与复制能力、无 Diff 视图渲染。本次升级将 TUI 渲染引擎提升至生产级水平，对标 Reasonix 的核心渲染体验。

## What Changes

- **新增思考/推理块渲染**：将 LLM 的 reasoning/thinking 文本以折叠块展示（默认折叠），支持 Ctrl+O 切换展开/折叠，实时流式追加
- **新增工具输出实时流式渲染**：引入 ToolProgress 事件类型，工具执行过程中实时流式展示输出（尾部截断），替代当前仅展示最终结果的模式
- **新增 Shell 输出折叠/展开交互**：Shell 命令输出默认折叠（显示前 N 行 + 行数摘要），支持 Ctrl+B 切换完整输出
- **升级 Markdown 渲染**：代码块集成 chroma 语法高亮（当前已声明但未完整启用），新增 Diff 视图渲染（+/- 行着色），完善 GFM 表格渲染
- **新增文本选择与复制**：支持鼠标拖拽选中 viewport 中的文本，Ctrl+C / Super+C 在选中时复制到系统剪贴板
- **新增 Diff 视图渲染**：识别 diff 格式文本并以 +/- 着色渲染，支持 /diff-fold 控制折叠行数

## Capabilities

### New Capabilities

- `reasoning-display`: LLM 思考/推理文本的折叠展示与流式渲染，支持 Ctrl+O 切换展开/折叠
- `tool-streaming`: 工具执行过程中的实时流式输出展示，尾部截断 + 行数计数
- `shell-output-toggle`: Shell 命令输出的交互式折叠/展开，Ctrl+B 切换
- `text-selection`: viewport 文本的鼠标选择与系统剪贴板复制
- `diff-view`: Diff 格式文本的 +/- 着色渲染与折叠控制

### Modified Capabilities

- `tui-chat-interface`: 增强 Markdown 渲染（chroma 语法高亮、diff 视图、表格完善）、增强工具卡片（流式输出）、增强进度指示（工具进度事件）

## Impact

- `internal/tui/model.go`: 新增 reasoning/pending 状态字段、ToolProgress 事件处理、文本选择状态
- `internal/tui/transcript.go`: 新增推理块/流式输出块的渲染逻辑
- `internal/tui/toolcard.go`: 新增流式输出渲染、Shell 折叠/展开
- `internal/tui/markdown.go`: 升级 chroma 语法高亮集成、diff 视图渲染
- `internal/tui/view.go`: 新增文本选择渲染、diff 视图渲染
- `internal/tui/components.go`: 可能新增 diff 视图组件
- `internal/event/event.go`: 可能需要新增 ToolProgress 事件类型
- `internal/tui/sink.go`: 可能需要新增 ToolProgress 事件转发
- `go.mod`: 可能需要新增 chroma 依赖（若当前未直接依赖）
