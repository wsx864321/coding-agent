# rewind-picker Specification

## Purpose
TBD - created by archiving change tui-overlays. Update Purpose after archive.
## Requirements
### Requirement: Rewind 检查点选择器
系统 SHALL 在用户双击 Esc（两次 Esc 间隔 < 600ms）时打开 Rewind 检查点选择器覆盖层。

#### Scenario: 触发 Rewind
- **WHEN** 输入区为空且用户在 600ms 内按两次 Esc
- **THEN** Rewind 选择器打开，显示可用检查点列表

#### Scenario: 回退到检查点
- **WHEN** 用户选择检查点并按 Enter
- **THEN** 文件系统回退到该检查点状态

#### Scenario: 关闭选择器
- **WHEN** 用户按 Esc
- **THEN** Rewind 选择器关闭

