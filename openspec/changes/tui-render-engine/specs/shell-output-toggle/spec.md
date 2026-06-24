## ADDED Requirements

### Requirement: Shell 输出折叠展示
系统 SHALL 对 Shell 命令（bash 工具）的输出默认折叠展示：仅显示前 N 行（默认 8 行）和总行数摘要（如 "⎿ 156 lines, collapsed"）。

#### Scenario: Shell 命令执行完成
- **WHEN** bash 工具返回输出结果
- **THEN** 输出以折叠形式展示：前 8 行 + "⎿ N lines, collapsed" 摘要行

#### Scenario: 短输出不折叠
- **WHEN** bash 工具输出行数 ≤ 8 行
- **THEN** 输出完整展示，不显示折叠摘要

### Requirement: Ctrl+B 切换 Shell 输出展开/折叠
系统 SHALL 支持用户通过 Ctrl+B 快捷键切换最近一个 Shell 输出块的展开/折叠状态。

#### Scenario: 用户展开 Shell 输出
- **WHEN** 存在折叠的 Shell 输出块且用户按下 Ctrl+B
- **THEN** 该输出块展开显示完整内容，摘要行消失

#### Scenario: 用户折叠 Shell 输出
- **WHEN** Shell 输出块已展开且用户按下 Ctrl+B
- **THEN** 该输出块重新折叠为前 8 行 + 摘要行

#### Scenario: 无 Shell 输出时 Ctrl+B 无操作
- **WHEN** 消息流中无 Shell 输出块且用户按下 Ctrl+B
- **THEN** 无任何变化

### Requirement: Shell 输出完整内容存储
系统 SHALL 在内存中存储 Shell 命令的完整输出内容，以支持展开/折叠切换。

#### Scenario: Shell 输出存储
- **WHEN** bash 工具返回输出结果
- **THEN** 完整输出内容存储在 Model 的 shellOutputs map 中（key 为 toolCallID）

#### Scenario: 内存限制
- **WHEN** Shell 输出超过 1MB
- **THEN** 系统截断存储（保留最后 1MB），并在摘要中标注 "output truncated"
