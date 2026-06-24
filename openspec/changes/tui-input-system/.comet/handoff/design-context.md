# Comet Design Handoff

- Change: tui-input-system
- Phase: design
- Mode: compact
- Context hash: 0eda650553c385b492abc176c15b099dee81d9474e16e16775eada363ba0f5e3

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/tui-input-system/proposal.md

- Source: openspec/changes/tui-input-system/proposal.md
- Lines: 1-41
- SHA256: 9fae66e7752b556e27e94ffbcd6b4bcde8d7279726bc36a5101dd3fa0b87743a

```md
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
```

## openspec/changes/tui-input-system/design.md

- Source: openspec/changes/tui-input-system/design.md
- Lines: 1-51
- SHA256: e8ade5481dcb1f974e86f433fc65488ad2c83f40bc99e8cc643e33a4fd5a3225

```md
## Context

当前 TUI 输入区基于 Bubble Tea textarea 组件，仅支持多行编辑和 Enter 发送。Reasonix 的输入系统在此基础上增加了：斜杠命令补全、输入历史、排队输入、剪贴板粘贴、@引用、#记忆、!shell、模式切换。

## Goals / Non-Goals

**Goals:**
- 斜杠命令自动补全菜单
- 输入历史回溯（↑↓）
- 运行中排队输入
- 剪贴板粘贴（Ctrl+V）
- @文件引用解析
- #快速记忆
- !shell 直接执行
- Plan/YOLO 模式切换

**Non-Goals:**
- 不实现覆盖层/模态系统（Change D）
- 不改变 agent 核心逻辑
- 不实现 MCP prompt 解析

## Decisions

### D1: 补全菜单使用独立 completion 结构体

**选择**: 在 Model 中维护 `completion` 结构体（items、selected、active），在 Update 中检测 `/` 或 `@` 输入后更新补全列表。

**理由**: 与 Reasonix 相同模式。补全菜单作为独立组件，不侵入 textarea 逻辑。

### D2: 输入历史使用环形缓冲区

**选择**: 在 Model 中维护 `submittedInputs []string` + `submittedInputCursor int`，↑↓ 在空闲时回溯。

**理由**: 简单高效，无需持久化。每次发送消息时追加到列表头部。

### D3: 排队输入使用 slice 队列

**选择**: 在 Model 中维护 `pendingInterject []string`，运行中 Enter 将输入追加到队列，TurnDone 时自动发送队首消息。

**理由**: 允许用户在等待 LLM 响应时继续输入多条消息，不阻塞。

### D4: Plan/YOLO 模式通过 Shift+Tab / Ctrl+Y 切换

**选择**: Shift+Tab 在 Normal → Plan 之间切换；Ctrl+Y 在 Normal/Auto ↔ YOLO 之间切换。模式标签显示在状态栏。

**理由**: Reasonix 使用相同快捷键。Plan 模式限制 agent 为只读操作，YOLO 模式自动批准所有工具调用。

## Risks / Trade-offs

- **[安全] YOLO 模式**: 自动批准所有工具调用有风险。→ 在状态栏用醒目颜色（红色）标记 YOLO 模式，且仅在用户显式按 Ctrl+Y 时激活。
- **[性能] 文件引用补全**: 大型仓库中 @ 补全可能慢。→ 限制文件搜索深度和结果数量。
```

## openspec/changes/tui-input-system/tasks.md

- Source: openspec/changes/tui-input-system/tasks.md
- Lines: 1-64
- SHA256: 99813a07c70c6c0a70fe40b71c0d2ef77499ffa77f03a7405e768d3532954bea

```md
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
```

## openspec/changes/tui-input-system/specs/clipboard-paste/spec.md

- Source: openspec/changes/tui-input-system/specs/clipboard-paste/spec.md
- Lines: 1-16
- SHA256: 0a24d9d6648f95bb1b60bb8611fd6e5414529a727ae0ee6b17cbf66920580742

```md
## ADDED Requirements

### Requirement: 剪贴板粘贴
系统 SHALL 支持用户通过 Ctrl+V 将系统剪贴板内容粘贴到输入区。

#### Scenario: 粘贴文本
- **WHEN** 用户按 Ctrl+V 且剪贴板包含文本
- **THEN** 文本插入到输入区光标位置

#### Scenario: 粘贴大文本
- **WHEN** 剪贴板文本超过 500 字符
- **THEN** 文本折叠显示为 "pasted N chars" 标签，发送时展开

#### Scenario: 粘贴图片
- **WHEN** 用户按 Ctrl+V 且剪贴板包含图片
- **THEN** 图片保存为临时文件，输入区插入 @image-ref 引用
```

## openspec/changes/tui-input-system/specs/file-refs/spec.md

- Source: openspec/changes/tui-input-system/specs/file-refs/spec.md
- Lines: 1-12
- SHA256: b61dc7d06778085cc355774ed212b57e61a19f8f4887a68bca90a8a2b7e1bfa3

```md
## ADDED Requirements

### Requirement: @文件引用解析
系统 SHALL 在用户输入 `@` 后触发文件路径补全，支持从工作目录搜索匹配文件。

#### Scenario: 触发文件补全
- **WHEN** 用户输入 `@` 后跟部分文件名
- **THEN** 补全菜单显示匹配的文件路径

#### Scenario: 接受文件引用
- **WHEN** 用户选择文件补全项
- **THEN** 输入区插入 `@path/to/file` 引用，发送时解析为文件内容
```

## openspec/changes/tui-input-system/specs/input-history/spec.md

- Source: openspec/changes/tui-input-system/specs/input-history/spec.md
- Lines: 1-16
- SHA256: 7ff7e1b0395dccdad6ec95ee36258fc4a7cb20bf5f3cd7e81fb5be4ff30171e3

```md
## ADDED Requirements

### Requirement: 输入历史回溯
系统 SHALL 支持用户在空闲时通过 ↑↓ 键回溯已发送的消息历史。

#### Scenario: 回溯上一条消息
- **WHEN** TUI 空闲且用户按 ↑
- **THEN** 输入区显示上一条已发送的消息

#### Scenario: 回溯下一条消息
- **WHEN** 用户已回溯到历史消息且按 ↓
- **THEN** 输入区显示下一条历史消息（或回到空白）

#### Scenario: 编辑历史消息后发送
- **WHEN** 用户回溯到历史消息、编辑后按 Enter
- **THEN** 编辑后的消息作为新消息发送，不修改历史
```

## openspec/changes/tui-input-system/specs/mode-toggle/spec.md

- Source: openspec/changes/tui-input-system/specs/mode-toggle/spec.md
- Lines: 1-23
- SHA256: dac909b8dee0a276debd6c2323d44709b4b5278bb281e35ed21cf97bcde45dfa

```md
## ADDED Requirements

### Requirement: Plan 模式切换
系统 SHALL 支持用户通过 Shift+Tab 在 Normal 和 Plan 模式之间切换。Plan 模式下 agent 仅执行只读操作。

#### Scenario: 进入 Plan 模式
- **WHEN** 用户按 Shift+Tab
- **THEN** 模式切换为 Plan，状态栏显示蓝色 "Plan" 标签

#### Scenario: 退出 Plan 模式
- **WHEN** Plan 模式下用户按 Shift+Tab
- **THEN** 模式切换回 Normal，Plan 标签消失

### Requirement: YOLO 模式切换
系统 SHALL 支持用户通过 Ctrl+Y 切换 YOLO 模式。YOLO 模式下自动批准所有工具调用。

#### Scenario: 进入 YOLO 模式
- **WHEN** 用户按 Ctrl+Y
- **THEN** 模式切换为 YOLO，状态栏显示红色 "YOLO" 标签

#### Scenario: 退出 YOLO 模式
- **WHEN** YOLO 模式下用户按 Ctrl+Y
- **THEN** 模式切换回之前的审批模式
```

## openspec/changes/tui-input-system/specs/queue-interject/spec.md

- Source: openspec/changes/tui-input-system/specs/queue-interject/spec.md
- Lines: 1-16
- SHA256: f1c7cebae2f741e88752291401e5ea4afc16e67dd154e20ff2f27d13d6c3ae42

```md
## ADDED Requirements

### Requirement: 运行中排队输入
系统 SHALL 允许用户在 turn 运行中键入消息，消息排队并在当前 turn 完成后自动发送。

#### Scenario: 运行中键入消息
- **WHEN** agent 正在处理 turn 且用户键入消息并按 Enter
- **THEN** 消息追加到排队队列，显示 "feedback queued" 提示

#### Scenario: 排队消息自动发送
- **WHEN** 当前 turn 完成且排队队列非空
- **THEN** 队首消息自动作为下一个 turn 发送

#### Scenario: 多条排队消息
- **WHEN** 用户连续键入多条消息
- **THEN** 消息按 FIFO 顺序排队，提示显示 "N queued"
```

## openspec/changes/tui-input-system/specs/quick-memory/spec.md

- Source: openspec/changes/tui-input-system/specs/quick-memory/spec.md
- Lines: 1-12
- SHA256: ff8ee16260eec63a5ff7c53492c213ca7a5f6968f9173f00a63e1a8cd6d1190c

```md
## ADDED Requirements

### Requirement: #快速记忆
系统 SHALL 支持用户通过 `# note` 格式直接将内容写入项目记忆文件。

#### Scenario: 写入记忆
- **WHEN** 用户输入 `# 这是一个重要的注意事项` 并按 Enter
- **THEN** 内容写入 REASONIX.md（或 AGENTS.md），显示确认提示

#### Scenario: 空记忆
- **WHEN** 用户输入 `#` 后无内容
- **THEN** 显示 "memory: empty note" 提示，不写入
```

## openspec/changes/tui-input-system/specs/shell-direct/spec.md

- Source: openspec/changes/tui-input-system/specs/shell-direct/spec.md
- Lines: 1-16
- SHA256: 645e734a40a95025527735568f034eeb6b8bd78893028047e39d837a689e34e4

```md
## ADDED Requirements

### Requirement: !shell 直接执行
系统 SHALL 支持用户通过 `!cmd` 格式绕过模型直接执行 shell 命令。

#### Scenario: 执行 shell 命令
- **WHEN** 用户输入 `!ls -la` 并按 Enter
- **THEN** 命令直接执行，输出渲染到消息流中

#### Scenario: 空命令
- **WHEN** 用户输入 `!` 后无内容
- **THEN** 显示提示，不执行

#### Scenario: Shell 模式视觉指示
- **WHEN** 输入区以 `!` 开头
- **THEN** 输入区边框变色（如黄色），状态栏显示 "Shell" 标签
```

## openspec/changes/tui-input-system/specs/slash-completion/spec.md

- Source: openspec/changes/tui-input-system/specs/slash-completion/spec.md
- Lines: 1-20
- SHA256: b48be5af11d9a92a06456c25cf67a5071ca5deafdf370e3fece098868c07356d

```md
## ADDED Requirements

### Requirement: 斜杠命令自动补全
系统 SHALL 在用户输入 `/` 后显示可用命令的补全菜单，支持键盘导航和选择。

#### Scenario: 触发补全菜单
- **WHEN** 用户输入以 `/` 开头的文本
- **THEN** 补全菜单显示匹配的命令列表（如 /help、/skills、/model 等）

#### Scenario: 键盘导航
- **WHEN** 补全菜单显示且用户按 ↑↓
- **THEN** 高亮项在菜单中上下移动

#### Scenario: 接受补全
- **WHEN** 用户按 Tab 或 Enter 选择补全项
- **THEN** 输入区替换为完整命令，补全菜单关闭

#### Scenario: 关闭补全
- **WHEN** 补全菜单显示且用户按 Esc
- **THEN** 补全菜单关闭，输入区保持当前文本
```

## openspec/changes/tui-input-system/specs/tui-chat-interface/spec.md

- Source: openspec/changes/tui-input-system/specs/tui-chat-interface/spec.md
- Lines: 1-28
- SHA256: 46c78368310412a0bafa0e69b5409163ada8dc876d20f1e818441afbf9bc6c99

```md
## MODIFIED Requirements

### Requirement: 支持基础会话控制快捷键
系统 MUST 提供基础快捷键能力以保证可用性，包括发送、退出会话、中断当前轮、基础导航、模式切换、剪贴板粘贴与输入历史回溯。默认语义为：`Enter` 发送、`Ctrl+C` 退出会话、`Esc` 中断当前轮、`Shift+Enter` 换行、PgUp/PgDn/Home/End 翻页、鼠标滚轮滚动、`Shift+Tab` 切换 Plan 模式、`Ctrl+Y` 切换 YOLO 模式、`Ctrl+V` 粘贴剪贴板、`↑↓` 回溯输入历史。

#### Scenario: 用户触发退出快捷键
- **WHEN** 用户在 TUI 中按下 Ctrl+C
- **THEN** 系统安全结束 TUI 会话并返回终端

#### Scenario: 用户中断当前轮但继续会话
- **WHEN** 模型正在流式输出且用户按下 `Esc`
- **THEN** 系统中断当前轮处理，保留当前会话历史并允许用户继续输入下一条消息

#### Scenario: 用户切换 Plan 模式
- **WHEN** 用户按下 Shift+Tab
- **THEN** 模式在 Normal 和 Plan 之间切换，状态栏显示对应标签

#### Scenario: 用户切换 YOLO 模式
- **WHEN** 用户按下 Ctrl+Y
- **THEN** 模式在 Normal/Auto 和 YOLO 之间切换，状态栏显示对应标签

#### Scenario: 用户粘贴剪贴板
- **WHEN** 用户按下 Ctrl+V
- **THEN** 剪贴板内容粘贴到输入区

#### Scenario: 用户回溯输入历史
- **WHEN** 空闲时用户按 ↑
- **THEN** 输入区显示上一条已发送消息
```

