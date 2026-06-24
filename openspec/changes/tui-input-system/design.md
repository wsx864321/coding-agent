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
