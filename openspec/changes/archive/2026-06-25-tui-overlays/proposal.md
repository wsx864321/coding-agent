## Why

当前 TUI 仅支持 y/n 审批横幅这一种模态交互，缺少 Skill 选择器、MCP 管理器、会话恢复选择器、Rewind 检查点、模型切换器、/clear 确认、Ask 多选题卡片等覆盖层/模态系统。对标 Reasonix 的覆盖层体系，需要实现完整的模态交互框架。

## What Changes

- **新增 Skill 选择器覆盖层**：/skills 打开，浏览已加载 Skill，启用/禁用，查看详情
- **新增 MCP 管理器覆盖层**：/mcp 打开，查看 MCP 服务器状态，连接/断开
- **新增会话恢复选择器**：/resume 打开，浏览历史会话，选择恢复
- **新增 Rewind 检查点选择器**：Esc-Esc 触发，浏览快照回退
- **新增模型切换器**：/model 打开，选择 provider/model
- **新增 /clear 确认对话框**：/clear 时弹出确认，防止误操作
- **新增 Ask 多选题卡片**：ask 工具的 UI 渲染 + 键盘交互（数字选择/自由输入）
- **增强审批横幅**：显示工具名 + 参数摘要 + y/n/a 选项

## Capabilities

### New Capabilities

- `skill-picker`: Skill 选择器覆盖层（浏览、启用/禁用、查看详情）
- `mcp-manager`: MCP 管理器覆盖层（查看状态、连接/断开）
- `resume-picker`: 会话恢复选择器（浏览历史、选择恢复）
- `rewind-picker`: Rewind 检查点选择器（Esc-Esc 触发，快照回退）
- `model-switcher`: 模型切换器（/model 选择 provider/model）
- `clear-confirm`: /clear 确认对话框
- `ask-card`: Ask 工具多选题卡片渲染与交互
- `approval-enhance`: 审批横幅增强（参数摘要 + y/n/a）

### Modified Capabilities

- `tui-chat-interface`: 修改"交互式审批"需求，增加参数摘要显示和 always 选项

## Impact

- `internal/tui/model.go`: 新增 skillPick、mcp、resumePick、rewind、modelPicker、clearConfirm、chooser 等覆盖层状态字段
- 新建 `internal/tui/overlays/` 子包：各覆盖层的渲染与键盘处理
- `internal/tui/view.go`: 修改 View() 集成覆盖层渲染
- `internal/tui/approval.go`: 增强审批横幅渲染
