## ADDED Requirements

### Requirement: 余额显示
系统 SHALL 在状态栏数据行显示 provider 账户余额（如 "¥110.00"），异步刷新，不阻塞 UI。

#### Scenario: 余额可用
- **WHEN** provider 支持余额查询且查询成功
- **THEN** 状态栏显示余额（如 "¥110.00"）

#### Scenario: 余额不可用
- **WHEN** provider 不支持余额查询或查询失败
- **THEN** 余额不显示（静默降级）

#### Scenario: 余额刷新
- **WHEN** TUI 启动时和每个 turn 完成后
- **THEN** 余额异步刷新
