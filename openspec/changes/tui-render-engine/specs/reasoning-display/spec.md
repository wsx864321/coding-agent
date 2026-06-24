## ADDED Requirements

### Requirement: 推理文本折叠展示
系统 SHALL 将 LLM 的思考/推理文本（reasoning_content）以折叠块形式展示在消息流中，默认折叠仅显示摘要行（如 "▎ thought for 3s"），用户可通过 Ctrl+O 切换展开/折叠。

#### Scenario: 推理文本流式到达
- **WHEN** LLM 返回 reasoning_content 增量 chunk
- **THEN** 消息流中显示实时更新的推理摘要行（spinner + "thinking…"），推理文本在后台累积

#### Scenario: 推理文本完成
- **WHEN** LLM 完成推理并开始返回回答文本
- **THEN** 推理摘要行更新为 "▎ thought for Ns"（N 为推理耗时秒数），推理文本折叠隐藏

#### Scenario: 用户展开推理文本
- **WHEN** 用户按下 Ctrl+O 且存在折叠的推理文本
- **THEN** 推理文本以 dim 样式展开显示在消息流中

#### Scenario: 用户折叠推理文本
- **WHEN** 推理文本已展开且用户按下 Ctrl+O
- **THEN** 推理文本重新折叠为摘要行

### Requirement: 推理文本与回答文本分离渲染
系统 SHALL 将推理文本与回答文本在视觉上区分：推理文本使用 dim/faint 样式，回答文本使用正常样式。

#### Scenario: 推理和回答同时可见
- **WHEN** 用户展开推理文本
- **THEN** 推理文本以 dim 样式渲染在回答文本上方，两者有明确视觉边界
