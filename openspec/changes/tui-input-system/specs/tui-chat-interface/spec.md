## MODIFIED Requirements

### Requirement: 支持基础会话控制快捷键
系统 MUST 提供基础快捷键能力以保证可用性，包括发送、退出会话、中断当前轮、基础导航、模式切换、剪贴板粘贴与输入历史回溯。默认语义为：`Enter` 发送、`Ctrl+C` 退出会话、`Esc` 中断当前轮、`Shift+Enter` 换行、PgUp/PgDn/Home/End 翻页、鼠标滚轮滚动、`Shift+Tab` 切换 Plan 模式、`Ctrl+Y` 切换 YOLO 模式、`Ctrl+V` 粘贴剪贴板、`↑↓` 回溯输入历史。

#### Scenario: 用户触发退出快捷键
- **WHEN** 用户在 TUI 中按下 Ctrl+C
- **THEN** 系统安全结束 TUI 会话并返回终端

#### Scenario: 用户中断当前轮但继续会话
- **WHEN** 模型正在流式输出且用户按下 `Esc`
- **THEN** 系统中断当前轮处理，保留当前会话历史并允许用户继续输入下一条消息

#### Scenario: 用户切换 Plan 模式
- **WHEN** 用户按下 Shift+Tab
- **THEN** 模式在 Normal 和 Plan 之间切换，状态栏显示对应标签

#### Scenario: 用户切换 YOLO 模式
- **WHEN** 用户按下 Ctrl+Y
- **THEN** 模式在 Normal/Auto 和 YOLO 之间切换，状态栏显示对应标签

#### Scenario: 用户粘贴剪贴板
- **WHEN** 用户按下 Ctrl+V
- **THEN** 剪贴板内容粘贴到输入区

#### Scenario: 用户回溯输入历史
- **WHEN** 空闲时用户按 ↑
- **THEN** 输入区显示上一条已发送消息
