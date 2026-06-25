# clear-confirm Specification

## Purpose
TBD - created by archiving change tui-overlays. Update Purpose after archive.
## Requirements
### Requirement: /clear 确认对话框
系统 SHALL 在用户输入 `/clear` 时弹出确认对话框，防止误操作清除会话。

#### Scenario: 触发确认
- **WHEN** 用户输入 `/clear` 并按 Enter
- **THEN** 确认对话框显示警告文本和 y/n 选项

#### Scenario: 确认清除
- **WHEN** 用户按 y
- **THEN** 会话清除，对话框关闭

#### Scenario: 取消清除
- **WHEN** 用户按 n 或 Esc
- **THEN** 会话保留，对话框关闭

