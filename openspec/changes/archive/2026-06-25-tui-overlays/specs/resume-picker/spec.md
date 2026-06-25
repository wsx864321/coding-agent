## ADDED Requirements

### Requirement: 会话恢复选择器
系统 SHALL 在用户输入 `/resume` 时打开会话恢复选择器覆盖层，显示历史会话列表。

#### Scenario: 打开会话恢复选择器
- **WHEN** 用户输入 `/resume` 并按 Enter
- **THEN** 覆盖层显示历史会话列表（日期、模型、预览）

#### Scenario: 恢复会话
- **WHEN** 用户选择会话并按 Enter
- **THEN** 该会话被恢复，覆盖层关闭，消息流显示历史记录

#### Scenario: 关闭选择器
- **WHEN** 用户按 Esc
- **THEN** 会话恢复选择器关闭
