## ADDED Requirements

### Requirement: 自定义状态行命令
系统 SHALL 支持用户通过配置指定一个外部命令，其 stdout 第一行替换内置数据行显示。

#### Scenario: 配置了自定义状态行命令
- **WHEN** 用户配置了 `statusline.command`
- **THEN** 状态栏数据行显示该命令的 stdout 第一行（而非内置数据行）

#### Scenario: 自定义命令执行失败
- **WHEN** 自定义状态行命令执行失败或超时
- **THEN** 数据行回退为内置显示

#### Scenario: 自定义命令刷新
- **WHEN** TUI 启动时和每个 turn 完成后
- **THEN** 自定义状态行命令被异步执行，结果更新到数据行
