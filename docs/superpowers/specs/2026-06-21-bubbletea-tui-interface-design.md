---
comet_change: add-bubbletea-tui-interface
role: technical-design
canonical_spec: openspec
archived-with: 2026-06-21-add-bubbletea-tui-interface
status: final
---

# Bubble Tea TUI Chat Design

## Context

现有 `coding-agent chat` 已具备成熟的会话循环、权限检查、hooks、skills、memory 与 session 持久化能力。当前问题不在业务能力，而在交互形态：纯文本 REPL 在多轮会话中可读性和状态可视性较弱。

本设计在不破坏既有 `chat`/`once` 行为前提下，新增 `tui` 并行入口，采用 Bubble Tea 构建聊天主界面，优先完成首版可用闭环与跨平台稳定性。

## Goals

- 新增独立 `tui` 命令入口，保持与现有命令并存。
- 复用现有 agent 会话执行路径，避免重写核心业务逻辑。
- 提供消息流、输入框、实时流式输出、错误提示。
- 提供基础键位：`Enter` 发送、`Esc` 中断当前轮、`Ctrl+C` 退出、`↑↓` 与 `j/k` 导航。
- 保证 Windows/macOS/Linux 基础行为一致。

## Non-Goals

- 不替换现有 `chat` 命令默认体验。
- 不在首版引入多面板 UI（会话列表、配置中心、任务看板）。
- 不在首版引入高级主题系统、复杂鼠标交互或动画效果。

## Architecture

采用“适配层桥接”结构：

1. **CLI 入口层**
   - 在 `cmd/cli` 新增 `tui` 子命令。
   - 负责加载与 `chat` 一致的配置、工具注册、权限检查、skill store、memory、hooks、job manager。

2. **TUI 适配层（新）**
   - 负责 Bubble Tea `model` 状态机：输入、消息列表、光标/滚动、状态栏、错误栏。
   - 把用户交互事件（发送、退出、中断）转换为会话动作。
   - 把会话动作回传（token 增量、完成、错误、中断）转换为 UI 事件。

3. **会话执行层（复用）**
   - 复用现有 agent turn 调用路径与上下文管理能力。
   - 不改变既有 `chat` 的语义，仅通过桥接层接入不同 I/O 介质。

## Data Flow

1. 用户在输入框输入文本，按 `Enter`。
2. TUI 适配层发出“提交消息”事件，锁定当前输入并追加用户消息到消息流。
3. 会话执行层开始处理并通过事件通道返回增量 token。
4. TUI 适配层接收 token 事件并增量刷新同一条 assistant 消息。
5. 完成时发送“完成事件”，解除输入锁定，进入下一轮可输入状态。
6. 按 `Esc` 时发送“中断当前轮事件”，保留已产生消息并恢复可输入状态。
7. 按 `Ctrl+C` 时执行退出流程，清理资源并返回终端。

## Error Handling

- **可恢复错误**（网络波动、单轮执行失败）：
  - 在界面错误区展示清晰错误信息。
  - 当前轮标记失败状态，但会话保持可继续输入。
- **中断错误**（用户 `Esc` 触发取消）：
  - 显示“本轮已中断”提示。
  - 保留历史并允许继续发起下一轮。
- **不可恢复错误**（初始化失败）：
  - 返回命令层错误并退出 TUI。

## Key Trade-offs and Risks

- **取舍：先复用再抽象**
  - 首版不先做共享会话引擎重构，以最小改动快速交付。
  - 代价是后续若扩展多界面形态，可能需要二次抽象。

- **风险：流式输出并发边界**
  - token 回调与 Bubble Tea 事件循环需严格隔离，避免并发写 UI 状态。
  - 缓解方式：统一通过消息通道入 `Update`，禁止跨协程直接写 model。

- **风险：终端兼容差异**
  - 不同终端对键位和 ANSI 行为存在差异，Windows 风险更高。
  - 缓解方式：使用 Bubble Tea 官方推荐键位处理与保守渲染策略。

## Testing Strategy

1. **Model 级测试**
   - `Update` 键位行为：发送、导航、`Esc` 中断、`Ctrl+C` 退出。
   - 流式 token 事件追加与完成状态切换。
   - 错误事件展示与恢复输入状态。

2. **命令级集成测试（冒烟）**
   - `coding-agent tui` 启动成功。
   - 发送一条消息后可接收流式回复。
   - 输出中按 `Esc` 可中断当前轮并继续下一轮输入。
   - `Ctrl+C` 可稳定退出。

3. **跨平台验证**
   - 至少覆盖 Windows + Linux/macOS 各一套终端。
   - 验证启动、发送、流式接收、中断、退出五条主路径。

## Spec Patch Plan

- 回写 `openspec/changes/add-bubbletea-tui-interface/specs/tui-chat-interface/spec.md`：
  - 在“基础快捷键”要求中明确 `Ctrl+C` 为退出、`Esc` 为中断当前轮。
  - 增加“中断当前轮但保留会话可继续”的验收场景。
