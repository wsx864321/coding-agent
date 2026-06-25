# text-selection Specification

## Purpose
TBD - created by archiving change tui-render-engine. Update Purpose after archive.
## Requirements
### Requirement: 鼠标文本选择
系统 SHALL 支持用户通过鼠标拖拽在 viewport 消息流中选择文本。选择区域以反色高亮显示。

#### Scenario: 用户开始选择
- **WHEN** 用户在 viewport 区域按下鼠标左键
- **THEN** 系统记录选择起始位置（行、列），开始跟踪鼠标移动

#### Scenario: 用户拖动扩展选择
- **WHEN** 用户按住鼠标左键并拖动
- **THEN** 选择区域从起始位置扩展到当前鼠标位置，高亮区域实时更新

#### Scenario: 用户释放鼠标完成选择
- **WHEN** 用户释放鼠标左键
- **THEN** 选择区域保持高亮，等待用户复制操作

#### Scenario: 点击取消选择
- **WHEN** 存在选择区域且用户单击鼠标左键（非拖动）
- **THEN** 选择区域取消，高亮消失

### Requirement: 剪贴板复制
系统 SHALL 支持用户通过 Ctrl+C（或 Super+C / Meta+C）将选中的文本复制到系统剪贴板。

#### Scenario: 选中文本后复制
- **WHEN** 存在活动选择区域且用户按下 Ctrl+C
- **THEN** 选中文本被复制到系统剪贴板，选择区域取消，显示 "Copied N characters" 提示

#### Scenario: 无选择时 Ctrl+C 保持原有行为
- **WHEN** 无活动选择区域且用户按下 Ctrl+C
- **THEN** 保持原有行为（运行中取消当前 turn，空闲时清空输入或退出）

#### Scenario: 剪贴板不可用时降级
- **WHEN** 系统剪贴板不可用（如 headless 环境）
- **THEN** 显示 "Copied N characters (clipboard unavailable)" 提示

