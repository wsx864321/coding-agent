## 1. 覆盖层框架

- [ ] 1.1 定义覆盖层接口（Open、Close、Update、View、Active）
- [ ] 1.2 在 `internal/tui/model.go` 中新增覆盖层路由逻辑（按优先级检查覆盖层状态）
- [ ] 1.3 修改 `internal/tui/view.go` 集成覆盖层渲染区域

## 2. Skill 选择器

- [ ] 2.1 新建 `internal/tui/overlays/skillpicker.go`：实现 skillPicker 结构体
- [ ] 2.2 实现 Skill 列表渲染（名称、描述、启用/禁用状态）
- [ ] 2.3 实现键盘交互：↑↓ 导航、Enter 查看详情、Space 启用/禁用、Esc 关闭
- [ ] 2.4 在 Model 中处理 `/skills` 命令打开选择器

## 3. MCP 管理器

- [ ] 3.1 新建 `internal/tui/overlays/mcpmanager.go`：实现 mcpManager 结构体
- [ ] 3.2 实现 MCP 服务器列表渲染（名称、状态、工具数）
- [ ] 3.3 实现键盘交互：↑↓ 导航、Enter 连接/断开、R 重连、Esc 关闭
- [ ] 3.4 在 Model 中处理 `/mcp` 命令打开管理器

## 4. 会话恢复选择器

- [ ] 4.1 新建 `internal/tui/overlays/resumepicker.go`：实现 resumePicker 结构体
- [ ] 4.2 实现会话列表渲染（日期、模型、预览）
- [ ] 4.3 实现键盘交互：↑↓ 导航、Enter 恢复、Esc 关闭
- [ ] 4.4 在 Model 中处理 `/resume` 命令打开选择器

## 5. Rewind 检查点选择器

- [ ] 5.1 新建 `internal/tui/overlays/rewindpicker.go`：实现 rewindPicker 结构体
- [ ] 5.2 实现检查点列表渲染（时间、描述）
- [ ] 5.3 实现键盘交互：↑↓ 导航、Enter 回退、Esc 关闭
- [ ] 5.4 在 Model 中处理 Esc-Esc 打开选择器

## 6. 模型切换器

- [ ] 6.1 新建 `internal/tui/overlays/modelpicker.go`：实现 modelPicker 结构体
- [ ] 6.2 实现模型列表渲染（provider/model、当前标记）
- [ ] 6.3 实现键盘交互：↑↓ 导航、Enter 切换、Esc 关闭
- [ ] 6.4 在 Model 中处理 `/model` 命令打开切换器

## 7. /clear 确认对话框

- [ ] 7.1 新建 `internal/tui/overlays/clearconfirm.go`：实现 clearConfirm 结构体
- [ ] 7.2 实现确认对话框渲染（警告文本 + y/n 选项）
- [ ] 7.3 实现键盘交互：y 确认清除、n/Esc 取消
- [ ] 7.4 在 Model 中处理 `/clear` 命令打开确认框

## 8. Ask 多选题卡片

- [ ] 8.1 新建 `internal/tui/overlays/chooser.go`：实现 chooser 结构体
- [ ] 8.2 实现单选题渲染（数字选项 + 描述）
- [ ] 8.3 实现多选题渲染（空格切换 + Enter 确认）
- [ ] 8.4 实现自由输入模式（键入文本 + Enter 提交）
- [ ] 8.5 在 Model 的 ApprovalRequest 处理中检测 ask 工具并打开 chooser

## 9. 审批横幅增强

- [ ] 9.1 修改 `internal/tui/approval.go`：增加参数摘要显示（工具名 + 关键参数）
- [ ] 9.2 增加 "always" 选项（a 键）：批准本次及后续同类工具调用
- [ ] 9.3 增加工具类别图标（读/写/执行/进程）

## 10. 集成测试

- [ ] 10.1 为覆盖层路由逻辑编写单元测试
- [ ] 10.2 为 Skill 选择器编写单元测试
- [ ] 10.3 为 Ask 卡片编写单元测试
- [ ] 10.4 为审批横幅增强编写单元测试
- [ ] 10.5 运行全量测试套件确认无回归
