# input-history Specification

## Purpose
TBD - created by archiving change tui-input-system. Update Purpose after archive.
## Requirements
### Requirement: 输入历史回溯
系统 SHALL 支持用户在空闲时通过 ↑↓ 键回溯已发送的消息历史。

#### Scenario: 回溯上一条消息
- **WHEN** TUI 空闲且用户按 ↑
- **THEN** 输入区显示上一条已发送的消息

#### Scenario: 回溯下一条消息
- **WHEN** 用户已回溯到历史消息且按 ↓
- **THEN** 输入区显示下一条历史消息（或回到空白）

#### Scenario: 编辑历史消息后发送
- **WHEN** 用户回溯到历史消息、编辑后按 Enter
- **THEN** 编辑后的消息作为新消息发送，不修改历史

