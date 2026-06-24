# Brainstorm Summary

- Change: tui-overlays
- Date: 2026-06-24

## 确认的技术方案

### 覆盖层框架
- 每个覆盖层独立结构体（skillPicker、mcpManager、resumePicker、rewindPicker、modelPicker、clearConfirm、chooser）
- Model 中以指针字段存储（nil = 关闭）
- Update 中按优先级检查覆盖层状态，有覆盖层时键盘事件优先路由到覆盖层
- 覆盖层渲染在 viewport 和输入区之间

### Skill 选择器
- /skills 打开，↑↓ 导航，Enter 查看详情，Esc 关闭
- 显示名称、描述、启用/禁用状态

### MCP 管理器
- /mcp 打开，显示服务器列表（名称、状态、工具数）
- Enter 连接/断开，R 重连，Esc 关闭

### 会话恢复选择器
- /resume 打开，显示历史会话（日期、模型、预览）
- Enter 恢复，Esc 关闭

### Rewind 检查点选择器
- Esc-Esc（600ms 内双击）触发
- 显示检查点列表，Enter 回退，Esc 关闭

### 模型切换器
- /model 打开，显示可用模型列表
- Enter 切换，Esc 关闭

### /clear 确认对话框
- /clear 弹出确认，y 清除，n/Esc 取消

### Ask 多选题卡片
- chooser 结构体支持单选（数字键）、多选（空格+Enter）、自由输入
- ask 工具触发时打开

### 审批横幅增强
- 显示工具名 + 参数摘要 + y/n/a 选项
- 敏感参数脱敏（***）

## 关键取舍与风险

| 取舍/风险 | 缓解 |
|----------|------|
| 覆盖层数量多 | 独立文件 + 统一接口 |
| 键盘冲突 | 覆盖层激活时保留 Ctrl+C 退出 |
| Rewind 依赖检查点系统 | UI 层先实现，底层后续 |

## 测试策略

- 单元测试：覆盖层路由、各覆盖层键盘交互
- 集成测试：Skill 选择器、Ask 卡片、审批横幅
- 回归测试：全量 TUI 测试套件

## Spec Patch

无
