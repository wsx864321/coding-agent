## MODIFIED Requirements

### Requirement: 交互式审批
系统 MUST 对需要权限确认的工具调用提供交互式审批横幅，允许用户在 TUI 内批准或拒绝操作。横幅 MUST 显示工具名称和关键参数摘要。支持 y（批准）、n（拒绝）、a（批准本次及后续同类调用）三种操作。

#### Scenario: 需要权限的工具调用
- **WHEN** agent 发起需要用户确认的工具调用
- **THEN** TUI 显示审批横幅，包含工具名、参数摘要和操作选项（`[y]es` / `[n]o` / `[a]lways`）

#### Scenario: 用户批准操作
- **WHEN** 审批横幅显示中用户按 y
- **THEN** 工具调用被执行，审批横幅消失

#### Scenario: 用户拒绝操作
- **WHEN** 审批横幅显示中用户按 n
- **THEN** 工具调用被拒绝并返回 "Permission denied" 结果

#### Scenario: 用户选择 Always
- **WHEN** 审批横幅显示中用户按 a
- **THEN** 本次及后续同类工具调用自动批准
