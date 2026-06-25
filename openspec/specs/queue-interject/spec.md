# queue-interject Specification

## Purpose
TBD - created by archiving change tui-input-system. Update Purpose after archive.
## Requirements
### Requirement: 运行中排队输入
系统 SHALL 允许用户在 turn 运行中键入消息，消息排队并在当前 turn 完成后自动发送。

#### Scenario: 运行中键入消息
- **WHEN** agent 正在处理 turn 且用户键入消息并按 Enter
- **THEN** 消息追加到排队队列，显示 "feedback queued" 提示

#### Scenario: 排队消息自动发送
- **WHEN** 当前 turn 完成且排队队列非空
- **THEN** 队首消息自动作为下一个 turn 发送

#### Scenario: 多条排队消息
- **WHEN** 用户连续键入多条消息
- **THEN** 消息按 FIFO 顺序排队，提示显示 "N queued"

