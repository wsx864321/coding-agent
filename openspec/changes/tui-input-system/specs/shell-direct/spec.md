## ADDED Requirements

### Requirement: !shell 直接执行
系统 SHALL 支持用户通过 `!cmd` 格式绕过模型直接执行 shell 命令。

#### Scenario: 执行 shell 命令
- **WHEN** 用户输入 `!ls -la` 并按 Enter
- **THEN** 命令直接执行，输出渲染到消息流中

#### Scenario: 空命令
- **WHEN** 用户输入 `!` 后无内容
- **THEN** 显示提示，不执行

#### Scenario: Shell 模式视觉指示
- **WHEN** 输入区以 `!` 开头
- **THEN** 输入区边框变色（如黄色），状态栏显示 "Shell" 标签
