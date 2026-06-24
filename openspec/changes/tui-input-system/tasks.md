## 1. 斜杠命令自动补全

- [ ] 1.1 在 `internal/tui/model.go` 中新增 `completion` 结构体（items、selected、active）
- [ ] 1.2 新建 `internal/tui/completion.go`：实现补全菜单渲染（lipgloss 列表样式）
- [ ] 1.3 在 Update 中检测 `/` 输入，根据前缀过滤可用命令列表
- [ ] 1.4 实现补全菜单键盘交互：↑↓ 导航、Tab/Enter 接受、Esc 关闭
- [ ] 1.5 在 `cmd/cli/tui.go` 中传递可用 slash commands 列表到 TUI model

## 2. 输入历史回溯

- [ ] 2.1 在 `internal/tui/model.go` 中新增 `submittedInputs`、`submittedInputCursor` 字段
- [ ] 2.2 在发送消息时调用 `rememberSubmittedInput()` 追加到历史
- [ ] 2.3 在空闲时 ↑↓ 调用 `recallSubmittedInput()` 回溯历史
- [ ] 2.4 输入新字符时重置回溯光标

## 3. 运行中排队输入

- [ ] 3.1 在 `internal/tui/model.go` 中新增 `pendingInterject` 字段
- [ ] 3.2 在运行中按 Enter 时将输入追加到 pendingInterject
- [ ] 3.3 在 TurnDone 处理中自动发送 pendingInterject 队首消息
- [ ] 3.4 显示排队提示（如 "✎ feedback queued"）

## 4. 剪贴板粘贴

- [ ] 4.1 在 Update 中处理 Ctrl+V 按键
- [ ] 4.2 调用 `github.com/atotto/clipboard` 读取剪贴板文本
- [ ] 4.3 支持图片粘贴（检测剪贴板中的图片数据，保存为临时文件并插入 @引用）
- [ ] 4.4 大文本粘贴时折叠显示（"pasted N lines"）

## 5. @文件引用解析

- [ ] 5.1 在 Update 中检测 `@` 输入，触发文件路径补全
- [ ] 5.2 实现文件搜索（基于工作目录的 glob 或 walk）
- [ ] 5.3 在补全菜单中显示匹配的文件路径
- [ ] 5.4 接受补全时插入 `@path` 引用

## 6. #快速记忆

- [ ] 6.1 在 Enter 处理中检测 `# note` 格式
- [ ] 6.2 调用 memory 模块的 QuickAdd 写入项目记忆
- [ ] 6.3 显示确认提示（如 "memory: wrote to REASONIX.md"）

## 7. !shell 直接执行

- [ ] 7.1 在 Enter 处理中检测 `!cmd` 格式
- [ ] 7.2 绕过模型直接执行 shell 命令
- [ ] 7.3 将命令输出渲染到消息流中
- [ ] 7.4 输入区边框变色指示 Shell 模式

## 8. Plan/YOLO 模式切换

- [ ] 8.1 在 `internal/tui/model.go` 中新增 `planMode`、`yoloMode` 字段
- [ ] 8.2 实现 Shift+Tab 切换 Plan 模式
- [ ] 8.3 实现 Ctrl+Y 切换 YOLO 模式
- [ ] 8.4 模式标签显示在状态栏（Plan=蓝色、YOLO=红色、Normal=默认）
- [ ] 8.5 Plan 模式下用户消息前添加 plan mode 标记

## 9. 集成测试

- [ ] 9.1 为斜杠命令补全编写单元测试
- [ ] 9.2 为输入历史回溯编写单元测试
- [ ] 9.3 为排队输入编写单元测试
- [ ] 9.4 为模式切换编写单元测试
- [ ] 9.5 运行全量测试套件确认无回归
