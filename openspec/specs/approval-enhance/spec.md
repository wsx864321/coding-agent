# approval-enhance Specification

## Purpose
TBD - created by archiving change tui-overlays. Update Purpose after archive.
## Requirements
### Requirement: 审批横幅参数摘要
系统 SHALL 在审批横幅中显示工具调用的关键参数摘要，帮助用户快速判断是否批准。

#### Scenario: 显示参数摘要
- **WHEN** 审批横幅显示
- **THEN** 横幅包含工具名 + 关键参数值（如 `Bash("go test ./...")`）

#### Scenario: 敏感参数脱敏
- **WHEN** 参数包含敏感 key（password、token、secret 等）
- **THEN** 参数值显示为 "***"

### Requirement: Always 批准选项
系统 SHALL 在审批横幅中提供 "always" 选项（a 键），批准本次及后续同类工具调用。

#### Scenario: Always 批准
- **WHEN** 用户按 a
- **THEN** 本次工具调用被批准，后续同类工具调用自动批准

