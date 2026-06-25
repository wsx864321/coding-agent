## MODIFIED Requirements

### Requirement: 状态栏信息展示
系统 MUST 在底部状态栏显示三行信息布局：工作行（spinner + elapsed + token↓，仅运行中显示）、模式行（Plan/YOLO/Shell 标签 + effort + git 分支状态）、数据行（模型名 + 上下文仪表 + 缓存率 + 任务数 + 余额）。

#### Scenario: 运行中状态栏
- **WHEN** agent 正在处理 turn
- **THEN** 工作行显示 spinner 动画 + 已耗时间 + 输出 token 数（如 "⣾ thinking 12s · ↓1.2k"）；模式行显示当前模式标签；数据行显示模型名 + 上下文仪表 + 缓存率

#### Scenario: 空闲状态栏
- **WHEN** agent 空闲等待输入
- **THEN** 工作行隐藏；模式行显示模式标签 + effort + git 分支；数据行显示模型名 + 上下文仪表 + 缓存率 + 余额

#### Scenario: 小终端适配
- **WHEN** 终端宽度 < 80 列
- **THEN** 状态栏各行内容自然换行，不截断
