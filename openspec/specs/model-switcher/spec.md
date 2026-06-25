# model-switcher Specification

## Purpose
TBD - created by archiving change tui-overlays. Update Purpose after archive.
## Requirements
### Requirement: 模型切换器
系统 SHALL 在用户输入 `/model` 时打开模型切换器覆盖层，显示可用模型列表。

#### Scenario: 打开模型切换器
- **WHEN** 用户输入 `/model` 并按 Enter
- **THEN** 覆盖层显示可用模型列表（provider/model），当前模型标记

#### Scenario: 切换模型
- **WHEN** 用户选择模型并按 Enter
- **THEN** 模型切换，覆盖层关闭，状态栏更新模型名

#### Scenario: 关闭切换器
- **WHEN** 用户按 Esc
- **THEN** 模型切换器关闭

