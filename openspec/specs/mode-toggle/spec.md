# mode-toggle Specification

## Purpose
TBD - created by archiving change tui-input-system. Update Purpose after archive.
## Requirements
### Requirement: Plan 模式切换
系统 SHALL 支持用户通过 Shift+Tab 在 Normal 和 Plan 模式之间切换。Plan 模式下 agent 仅执行只读操作。

#### Scenario: 进入 Plan 模式
- **WHEN** 用户按 Shift+Tab
- **THEN** 模式切换为 Plan，状态栏显示蓝色 "Plan" 标签

#### Scenario: 退出 Plan 模式
- **WHEN** Plan 模式下用户按 Shift+Tab
- **THEN** 模式切换回 Normal，Plan 标签消失

### Requirement: YOLO 模式切换
系统 SHALL 支持用户通过 Ctrl+Y 切换 YOLO 模式。YOLO 模式下自动批准所有工具调用。

#### Scenario: 进入 YOLO 模式
- **WHEN** 用户按 Ctrl+Y
- **THEN** 模式切换为 YOLO，状态栏显示红色 "YOLO" 标签

#### Scenario: 退出 YOLO 模式
- **WHEN** YOLO 模式下用户按 Ctrl+Y
- **THEN** 模式切换回之前的审批模式

