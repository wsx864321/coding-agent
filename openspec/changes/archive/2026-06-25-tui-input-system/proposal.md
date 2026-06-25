## Why

当前 TUI 输入区仅支持纯文本输入 + Enter 发送，缺少斜杠命令自动补全、输入历史回溯、运行中排队输入、剪贴板粘贴、@文件引用、#快速记忆、!shell 直接执行、Plan/YOLO 模式切换等交互能力。对标 Reasonix 的输入系统，需要大幅增强输入区的交互能力。

## What Changes

- **新增斜杠命令自动补全菜单**：输入 `/` 触发补全菜单，↑↓ 导航，Tab/Enter 接受，Esc 关闭
- **新增输入历史回溯**：↑↓ 在空闲时回溯已发送消息
- **新增运行中排队输入**：turn 运行中键入的消息排队，turn 结束后自动发送
- **新增剪贴板粘贴**：Ctrl+V 粘贴文本和图片
- **新增 @文件引用解析**：输入 `@` 触发文件路径补全
- **新增 #快速记忆**：`# note` 直接写入项目记忆
- **新增 !shell 直接执行**：`!cmd` 绕过模型直接执行 shell 命令
- **新增 Plan 模式切换**：Shift+Tab 切换 Plan 模式
- **新增 YOLO 模式切换**：Ctrl+Y 切换 YOLO 模式（自动批准工具调用）
- **新增输入区自适应高度**：多行输入时 textarea 自动扩展

## Capabilities

### New Capabilities

- `slash-completion`: 斜杠命令自动补全菜单（/help、/skills、/model 等 + 自定义命令）
- `input-history`: 输入历史回溯（↑↓ 浏览已发送消息）
- `queue-interject`: 运行中排队输入系统（turn 中键入 → turn 后自动发送）
- `clipboard-paste`: 剪贴板粘贴（Ctrl+V 文本 + 图片）
- `file-refs`: @文件引用解析与补全
- `quick-memory`: #快速记忆（# note → 写入 REASONIX.md）
- `shell-direct`: !shell 直接执行（!cmd 绕过模型）
- `mode-toggle`: Plan/YOLO 模式切换（Shift+Tab / Ctrl+Y）

### Modified Capabilities

- `tui-chat-interface`: 修改"基础会话控制快捷键"需求，增加 Shift+Tab、Ctrl+Y、Ctrl+V、↑↓ 历史回溯等快捷键

## Impact

- `internal/tui/model.go`: 新增 submittedInputs、pendingInterject、planMode、yoloMode、completion 等字段
- `internal/tui/model.go` Update: 大量新增按键处理逻辑
- `internal/tui/components.go`: textarea 配置调整（自适应高度）
- `cmd/cli/tui.go`: 可能需要传递 slash commands 列表到 TUI model
- 新增 `internal/tui/completion.go`: 自动补全菜单渲染与逻辑
