# tool-streaming Specification

## Purpose
TBD - created by archiving change tui-render-engine. Update Purpose after archive.
## Requirements
### Requirement: 工具实时流式输出
系统 SHALL 在工具执行过程中实时展示流式输出，而非仅在工具完成后展示最终结果。流式输出以尾部截断方式渲染（保留最后 N 行），并显示实时行数计数。

#### Scenario: 工具开始执行
- **WHEN** agent 发起工具调用且工具开始产生输出
- **THEN** 工具卡片下方出现流式输出块，实时追加新行

#### Scenario: 流式输出持续更新
- **WHEN** 工具持续产生输出行
- **THEN** 流式输出块保留最后 N 行（尾部截断），行数计数实时更新（如 "⎿ 156 lines"）

#### Scenario: 工具执行完成
- **WHEN** 工具调用返回最终结果
- **THEN** 流式输出块转为折叠摘要（显示前 M 行 + 总行数），与当前 ToolResult 行为一致

#### Scenario: 高频输出不阻塞 UI
- **WHEN** 工具以高频产生输出（如每秒数百行）
- **THEN** 系统合并事件批量更新，每帧最多重渲染一次，UI 保持响应

### Requirement: ToolProgress 事件类型
系统 SHALL 定义 ToolProgress 事件类型，携带工具调用标识（ToolCallID）和增量输出文本（Chunk），在 ToolDispatch 和 ToolResult 之间发送。

#### Scenario: ToolProgress 事件发送
- **WHEN** 工具执行过程中产生输出
- **THEN** 系统发送 ToolProgress 事件到 TUI 事件通道

#### Scenario: ToolProgress 事件被 TUI 消费
- **WHEN** TUI Model 收到 ToolProgress 事件
- **THEN** Model 找到对应工具的流式输出块并追加新内容，触发 viewport 重渲染

