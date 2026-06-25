# context-gauge Specification

## Purpose
TBD - created by archiving change tui-status-panels. Update Purpose after archive.
## Requirements
### Requirement: 上下文窗口使用率仪表
系统 SHALL 在状态栏数据行显示上下文窗口使用率，格式为 `ctx N/M (P%)`，其中 N 为已用 token 数，M 为窗口总 token 数，P 为百分比。颜色按压缩阈值变化：<50% 绿色、50-80% 黄色、>80% 红色。

#### Scenario: 上下文使用率正常
- **WHEN** 上下文使用率 < 50%
- **THEN** 仪表以绿色显示

#### Scenario: 上下文使用率接近阈值
- **WHEN** 上下文使用率在 50%-80% 之间
- **THEN** 仪表以黄色显示

#### Scenario: 上下文使用率触发压缩
- **WHEN** 上下文使用率 > 80%
- **THEN** 仪表以红色显示

#### Scenario: 上下文窗口信息不可用
- **WHEN** 无法获取上下文窗口信息（如 once 模式）
- **THEN** 仪表不显示

