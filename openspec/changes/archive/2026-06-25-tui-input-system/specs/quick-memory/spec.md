## ADDED Requirements

### Requirement: #快速记忆
系统 SHALL 支持用户通过 `# note` 格式直接将内容写入项目记忆文件。

#### Scenario: 写入记忆
- **WHEN** 用户输入 `# 这是一个重要的注意事项` 并按 Enter
- **THEN** 内容写入 REASONIX.md（或 AGENTS.md），显示确认提示

#### Scenario: 空记忆
- **WHEN** 用户输入 `#` 后无内容
- **THEN** 显示 "memory: empty note" 提示，不写入
