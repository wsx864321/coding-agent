# tui-chat-interface Specification

## Purpose
TBD - created by archiving change add-bubbletea-tui-interface. Update Purpose after archive.
## Requirements
### Requirement: 提供独立 TUI 聊天入口
系统 MUST 提供一个独立于现有 `chat`/`once` 的 TUI 命令入口，用于启动基于 Bubble Tea 的交互会话。

#### Scenario: 用户启动 TUI 命令
- **WHEN** 用户执行 TUI 子命令
- **THEN** 系统进入 TUI 聊天界面并显示初始可交互视图

### Requirement: 支持消息流与输入交互
系统 MUST 在 TUI 中提供基于 bubbles/v2 textarea 组件的多行输入区域，支持光标移动、多行编辑（Shift+Enter/Alt+Enter 换行）、粘贴和 IME 输入。消息展示区 MUST 使用 bubbles/v2 viewport 组件，支持鼠标滚轮、PgUp/PgDn/Home/End 翻页和滚动条指示。

#### Scenario: 用户提交一条消息
- **WHEN** 用户在 textarea 输入区输入文本并按 Enter 触发发送
- **THEN** 用户消息出现在消息流中，系统开始处理并展示助手回复

#### Scenario: 用户进行多行输入
- **WHEN** 用户在 textarea 中按 Shift+Enter 或 Alt+Enter
- **THEN** 输入区插入换行而非提交消息，textarea 高度动态扩展（最大 5 行）

#### Scenario: 用户使用鼠标滚轮滚动
- **WHEN** 用户在消息区使用鼠标滚轮
- **THEN** viewport 按滚动步长上下翻滚，滚动条位置同步更新

#### Scenario: 用户使用键盘翻页
- **WHEN** 用户按 PgUp/PgDn/Home/End
- **THEN** viewport 按页翻滚或跳到顶部/底部

### Requirement: 支持基础会话控制快捷键
系统 MUST 提供基础快捷键能力以保证可用性，包括发送、退出会话、中断当前轮与基础导航。默认语义为：`Enter` 发送、`Ctrl+C` 退出会话、`Esc` 中断当前轮、`Shift+Enter` 换行、PgUp/PgDn/Home/End 翻页、鼠标滚轮滚动。

#### Scenario: 用户触发退出快捷键
- **WHEN** 用户在 TUI 中按下 Ctrl+C
- **THEN** 系统安全结束 TUI 会话并返回终端

#### Scenario: 用户中断当前轮但继续会话
- **WHEN** 模型正在流式输出且用户按下 `Esc`
- **THEN** 系统中断当前轮处理，保留当前会话历史并允许用户继续输入下一条消息

### Requirement: 提供可见错误反馈
当请求处理、模型调用或内部执行失败时，系统 MUST 在 TUI 界面中展示可见且可理解的错误信息，而不是静默失败。

#### Scenario: 发生请求错误
- **WHEN** 会话处理中发生可恢复错误
- **THEN** 界面显示错误提示且用户可继续输入后续消息或退出

### Requirement: 保持跨平台基础一致行为
系统 MUST 在 Windows、macOS、Linux 上保持基础行为一致，包括启动、输入、消息展示和退出路径。

#### Scenario: 跨平台运行一致
- **WHEN** 用户分别在不同平台启动并完成一次基础对话
- **THEN** 各平台均可完成启动、发送消息、接收回复和正常退出

### Requirement: 支持 Markdown ANSI 渲染
系统 MUST 将助手回复中的 Markdown 文本渲染为带 ANSI 样式的终端输出。MUST 支持的元素：标题（带颜色/粗体）、段落、有序/无序列表、代码块（带 chroma 语法高亮）、内联代码（背景色区分）、粗体/斜体、引用块、GFM 表格。

#### Scenario: 助手回复包含代码块
- **WHEN** 助手回复包含 fenced code block（``````` ```language ... ``` ```````）
- **THEN** 代码块以缩进 + 语法高亮样式渲染，语言标识显示在代码块上方或旁边

#### Scenario: 助手回复包含表格
- **WHEN** 助手回复包含 GFM 风格的 Markdown 表格
- **THEN** 表格以对齐的列格式渲染，表头与数据行有视觉区分

#### Scenario: 流式输出中的 Markdown 渲染
- **WHEN** 助手回复正在流式输出
- **THEN** 系统按段落边界刷新渲染，未完成的代码块围栏不会被半渲染

### Requirement: 工具调用可视化
系统 MUST 在聊天流中展示工具调用的名称和参数摘要，以及工具执行结果的折叠展示。用户 MUST 能看到 agent 正在执行什么工具操作。

#### Scenario: agent 调用工具
- **WHEN** agent 发起一次工具调用
- **THEN** 消息流中显示工具卡片，包含工具名称和参数摘要（如 `● Read("src/main.go")`）

#### Scenario: 工具执行完成
- **WHEN** 工具调用返回结果
- **THEN** 工具卡片下方显示结果摘要（超过阈值行数时折叠，显示行数提示）

#### Scenario: 工具执行报错
- **WHEN** 工具调用返回错误
- **THEN** 工具卡片显示红色错误标记和错误消息

### Requirement: 交互式审批
系统 MUST 对需要权限确认的工具调用提供交互式审批横幅，允许用户在 TUI 内批准或拒绝操作，替代当前的全自动拒绝行为。

#### Scenario: 需要权限的工具调用
- **WHEN** agent 发起需要用户确认的工具调用（由 permission.Checker 判定）
- **THEN** TUI 显示审批横幅，包含工具名、参数摘要和操作选项（`[y]es` / `[n]o`）

#### Scenario: 用户批准操作
- **WHEN** 审批横幅显示中用户按 y
- **THEN** 工具调用被执行，审批横幅消失，消息流继续

#### Scenario: 用户拒绝操作
- **WHEN** 审批横幅显示中用户按 n
- **THEN** 工具调用被拒绝并返回 "Permission denied" 结果，agent 收到拒绝结果并继续

### Requirement: 进度指示
系统 MUST 在 LLM 推理或工具执行期间显示动画 spinner 和已耗时间，使用户能够区分"正在处理"和"卡住了"。

#### Scenario: LLM 开始推理
- **WHEN** 用户提交消息后 LLM 开始处理
- **THEN** 状态区域显示 spinner 动画和已耗时间（如 `⣾ thinking (3s)`）

#### Scenario: 工具正在执行
- **WHEN** agent 正在执行工具调用
- **THEN** 状态区域显示 spinner 和工具名称（如 `⣾ running Read...`）

#### Scenario: 处理完成
- **WHEN** LLM 返回最终回答
- **THEN** spinner 消失，状态区域显示空闲状态

### Requirement: 状态栏信息展示
系统 MUST 在底部状态栏显示当前模型名称和基础状态信息。

#### Scenario: 状态栏内容
- **WHEN** TUI 处于任何状态
- **THEN** 底部状态栏显示当前使用的模型名称

### Requirement: CJK 字符正确显示
系统 MUST 使用字符显示宽度（而非 UTF-8 编码字节数）计算文本换行和布局对齐，确保 CJK 字符（宽度=2）和 ASCII 字符（宽度=1）不会错位。

#### Scenario: 中文消息换行
- **WHEN** 用户或助手消息包含中文字符
- **THEN** 文本按字符显示宽度正确换行，不超出终端边界，不出现对齐错位

