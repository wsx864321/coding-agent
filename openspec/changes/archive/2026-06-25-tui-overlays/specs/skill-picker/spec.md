## ADDED Requirements

### Requirement: Skill 选择器覆盖层
系统 SHALL 在用户输入 `/skills` 时打开 Skill 选择器覆盖层，显示已加载的 Skill 列表及其启用/禁用状态。

#### Scenario: 打开 Skill 选择器
- **WHEN** 用户输入 `/skills` 并按 Enter
- **THEN** 覆盖层显示 Skill 列表（名称、描述、状态）

#### Scenario: 浏览 Skill
- **WHEN** Skill 选择器打开且用户按 ↑↓
- **THEN** 高亮项在列表中移动

#### Scenario: 查看 Skill 详情
- **WHEN** 用户按 Enter 选择某个 Skill
- **THEN** 显示该 Skill 的详细描述

#### Scenario: 关闭选择器
- **WHEN** 用户按 Esc
- **THEN** Skill 选择器关闭
