# ask-card Specification

## Purpose
TBD - created by archiving change tui-overlays. Update Purpose after archive.
## Requirements
### Requirement: Ask 多选题卡片
系统 SHALL 在 agent 调用 ask 工具时渲染多选题卡片覆盖层，支持单选、多选和自由输入。

#### Scenario: 单选题
- **WHEN** ask 工具参数中 multiSelect 为 false
- **THEN** 卡片显示选项列表，用户按数字键选择

#### Scenario: 多选题
- **WHEN** ask 工具参数中 multiSelect 为 true
- **THEN** 卡片显示选项列表，用户按空格切换选择，Enter 确认

#### Scenario: 自由输入
- **WHEN** ask 工具包含 "type something" 选项
- **THEN** 用户可键入自定义文本，Enter 提交

#### Scenario: 关闭卡片
- **WHEN** 用户按 Esc
- **THEN** 卡片关闭，返回空答案

