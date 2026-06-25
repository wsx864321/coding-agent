# cache-hit-display Specification

## Purpose
TBD - created by archiving change tui-status-panels. Update Purpose after archive.
## Requirements
### Requirement: 缓存命中率显示
系统 SHALL 在状态栏数据行显示 prompt cache 命中率百分比（如 "cache 85%"），帮助用户了解 token 成本优化效果。

#### Scenario: 缓存命中率可用
- **WHEN** provider 支持缓存且返回缓存命中统计
- **THEN** 状态栏显示 "cache N%"（N 为命中率整数）

#### Scenario: 缓存命中率不可用
- **WHEN** provider 不支持缓存或无缓存统计数据
- **THEN** 缓存命中率不显示

#### Scenario: 缓存命中率更新
- **WHEN** 每个 turn 完成后
- **THEN** 缓存命中率刷新为最新值

