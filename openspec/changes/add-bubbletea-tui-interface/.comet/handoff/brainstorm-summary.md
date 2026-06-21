# Brainstorm Summary

- Change: add-bubbletea-tui-interface
- Date: 2026-06-21

## 确认的技术方案

- 已确认采用并行入口策略：新增 TUI 命令，不替换现有 `chat`/`once`。
- 已确认首版范围：聊天主界面（消息流、输入框、基础快捷键、错误提示）。
- 已确认跨平台优先：Windows/macOS/Linux 基础行为一致。
- 已确认接入边界：保留现有 `runChat` 主流程，新增 TUI 适配层桥接输入/输出到 agent。
- 已确认回复渲染模式：按 token/片段进行增量流式渲染。
- 已确认首版键位：Enter 发送、Ctrl+C 退出、↑↓滚动、j/k 备用导航。
- 已确认中断策略：支持软中断当前轮，保留历史并允许继续输入。
- 已确认技术路径：方案 A（适配层桥接，保留 chat 主流程）。
- 已确认按键冲突解法：`Esc` 中断当前轮，`Ctrl+C` 退出会话。
- 设计方案状态：用户已确认，可进入 Design Doc 创建与 Spec Patch 回写阶段。

## 关键取舍与风险

- 取舍：优先复用现有 agent/session 逻辑，避免重写业务路径。
- 风险：token 流式输出与 Bubble Tea 事件循环并发边界复杂，需明确消息桥接机制。
- 风险：Windows 终端键位与渲染行为可能存在差异。
- 缓解：通过统一桥接层串行化 UI 事件投递，并以跨平台冒烟用例做回归守护。

## 测试策略

- 先补组件级模型测试（Bubble Tea model 的 Update/View 状态转换与键位事件）。
- 再做命令级集成冒烟（tui 启动、发送、流式接收、Esc 中断、Ctrl+C 退出）。
- 跨平台验证至少覆盖 Windows + Linux/macOS 各一套终端环境。

## Spec Patch

- 在 `tui-chat-interface/spec.md` 补充“Esc 中断当前轮、保留会话”验收场景。
- 在快捷键相关 requirement 中明确 `Ctrl+C` 为退出、`Esc` 为中断当前轮，避免语义歧义。
