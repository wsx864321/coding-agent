## MODIFIED Requirements

### Requirement: 支持 Markdown ANSI 渲染
系统 MUST 将助手回复中的 Markdown 文本渲染为带 ANSI 样式的终端输出。MUST 支持的元素：标题（带颜色/粗体）、段落、有序/无序列表、代码块（带 chroma 语法高亮）、内联代码（背景色区分）、粗体/斜体、引用块、GFM 表格。MUST 支持 diff 格式代码块的 +/- 着色渲染。

#### Scenario: 助手回复包含代码块
- **WHEN** 助手回复包含 fenced code block（``````` ```language ... ``` ```````）
- **THEN** 代码块以缩进 + chroma 语法高亮样式渲染，语言标识显示在代码块上方或旁边

#### Scenario: 助手回复包含表格
- **WHEN** 助手回复包含 GFM 风格的 Markdown 表格
- **THEN** 表格以对齐的列格式渲染，表头与数据行有视觉区分

#### Scenario: 流式输出中的 Markdown 渲染
- **WHEN** 助手回复正在流式输出
- **THEN** 系统按段落边界刷新渲染，未完成的代码块围栏不会被半渲染

#### Scenario: 助手回复包含 diff 代码块
- **WHEN** 助手回复包含 diff 格式代码块（```diff ... ```）
- **THEN** 新增行（+）以绿色渲染，删除行（-）以红色渲染，hunk 头（@@）以青色渲染

### Requirement: 工具调用可视化
系统 MUST 在聊天流中展示工具调用的名称和参数摘要，以及工具执行结果的折叠展示。对于支持流式输出的工具（如 bash），MUST 在工具执行过程中实时展示输出。用户 MUST 能看到 agent 正在执行什么工具操作。

#### Scenario: agent 调用工具
- **WHEN** agent 发起一次工具调用
- **THEN** 消息流中显示工具卡片，包含工具名称和参数摘要（如 `● Read("src/main.go")`）

#### Scenario: 工具执行过程中产生输出
- **WHEN** 工具执行过程中产生增量输出（如 bash 命令的 stdout）
- **THEN** 工具卡片下方实时展示流式输出（尾部截断 + 行数计数）

#### Scenario: 工具执行完成
- **WHEN** 工具调用返回结果
- **THEN** 工具卡片下方显示结果摘要（超过阈值行数时折叠，显示行数提示）。Shell 命令输出支持 Ctrl+B 展开/折叠

#### Scenario: 工具执行报错
- **WHEN** 工具调用返回错误
- **THEN** 工具卡片显示红色错误标记和错误消息

### Requirement: 进度指示
系统 MUST 在 LLM 推理或工具执行期间显示动画 spinner 和已耗时间，使用户能够区分"正在处理"和"卡住了"。MUST 在推理阶段显示 "thinking…" 指示，在工具执行阶段显示工具名称。

#### Scenario: LLM 开始推理
- **WHEN** 用户提交消息后 LLM 开始处理
- **THEN** 状态区域显示 spinner 动画和已耗时间（如 `⣾ thinking (3s)`）

#### Scenario: LLM 进入推理阶段
- **WHEN** LLM 返回 reasoning_content（思考文本）
- **THEN** 状态区域显示 "thinking…"，推理文本在消息流中以折叠块累积

#### Scenario: 工具正在执行
- **WHEN** agent 正在执行工具调用
- **THEN** 状态区域显示 spinner 和工具名称（如 `⣾ running Bash...`）

#### Scenario: 处理完成
- **WHEN** LLM 返回最终回答
- **THEN** spinner 消失，状态区域显示空闲状态
