## ADDED Requirements

### Requirement: Diff 格式识别与着色
系统 SHALL 自动识别消息流中的 diff 格式文本，并以 +/- 着色渲染：新增行（`+` 前缀）显示为绿色，删除行（`-` 前缀）显示为红色，hunk 头（`@@` 前缀）显示为青色。

#### Scenario: 助手回复包含 diff 代码块
- **WHEN** 助手回复中的代码块内容为 unified diff 格式（包含 `@@`、`+`、`-` 行）
- **THEN** 代码块内的 `+` 行以绿色渲染，`-` 行以红色渲染，`@@` 行以青色渲染，上下文行保持默认颜色

#### Scenario: 非 diff 代码块不受影响
- **WHEN** 助手回复中的代码块内容不是 diff 格式
- **THEN** 代码块以常规语法高亮渲染，不应用 diff 着色

#### Scenario: 混合内容正确渲染
- **WHEN** 助手回复同时包含普通文本和 diff 代码块
- **THEN** 普通文本正常渲染，仅 diff 代码块应用 +/- 着色

### Requirement: Diff 折叠控制
系统 SHALL 支持通过 `/diff-fold` 命令控制 diff 视图的最大显示行数，超过阈值的 diff 块折叠显示。

#### Scenario: Diff 块超过折叠阈值
- **WHEN** diff 代码块行数超过 diffMaxLines（默认 0 = 不限制）
- **THEN** 仅显示前 diffMaxLines 行 + "N more lines (use /diff-fold to expand)" 提示

#### Scenario: 调整折叠阈值
- **WHEN** 用户输入 `/diff-fold 50`
- **THEN** diffMaxLines 更新为 50，后续 diff 块按新阈值折叠
