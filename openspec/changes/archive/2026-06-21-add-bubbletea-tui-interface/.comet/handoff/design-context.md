# Comet Design Handoff

- Change: add-bubbletea-tui-interface
- Phase: design
- Mode: compact
- Context hash: 2054581938f8a7b30fd1c966e8822505a94ca2c87aff1338e1b06b2ef9443766

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/add-bubbletea-tui-interface/proposal.md

- Source: openspec/changes/add-bubbletea-tui-interface/proposal.md
- Lines: 1-26
- SHA256: 071c18dd7ec42e278dbbf55416074f9741bf39f1c785aceacda0cd2bdc5078fb

```md
## Why

当前 `coding-agent` 以传统 CLI 交互为主，面对多轮会话时的信息密度和操作效率有限。引入基于 Bubble Tea 的 TUI 模式可以提升可读性、输入反馈和交互效率，并在保持现有命令兼容的前提下提供更友好的终端体验。

## What Changes

- 新增一个并行的 TUI 入口命令（例如 `coding-agent tui`），不替换现有 `chat` 和 `once`。
- 在首个迭代中实现聊天主界面能力：消息流展示、输入框、基础快捷键、退出控制与错误可视化。
- 复用现有会话与 agent loop 能力，使 TUI 仅作为交互层而非业务重写。
- 明确跨平台终端兼容目标，优先保证 Windows/macOS/Linux 的基础一致行为。

## Capabilities

### New Capabilities

- `tui-chat-interface`: 提供基于 Bubble Tea 的聊天型终端界面，包括消息渲染、输入、快捷键与生命周期控制。

### Modified Capabilities

- 无。

## Impact

- 受影响代码：`cmd/cli` 命令注册与启动逻辑、交互会话相关模块、终端输出适配层。
- 新增依赖：`github.com/charmbracelet/bubbletea`（及必要的 TUI 生态依赖）。
- 运行影响：新增 TUI 运行路径，但不改变现有 CLI 命令默认行为。
```

## openspec/changes/add-bubbletea-tui-interface/design.md

- Source: openspec/changes/add-bubbletea-tui-interface/design.md
- Lines: 1-47
- SHA256: c3144427c6650dbedce72913d90a0c3e69cb2704c8b5b612679f6ceb55fb9e45

```md
## Context

`coding-agent` 当前已经具备较完整的 CLI 交互与会话能力，核心价值在于 Agent Loop、工具调用与会话恢复。现有 `chat` 入口在可视化层面较轻，随着多轮对话增长，用户在信息浏览、输入反馈和状态感知方面效率受限。  
本设计在不破坏既有 CLI 体验的前提下，引入 Bubble Tea 作为独立交互层，优先实现聊天主界面，并以跨平台稳定性（Windows/macOS/Linux）为约束。

## Goals / Non-Goals

**Goals:**

- 提供并行 TUI 入口，不影响 `chat` 与 `once` 的现有行为。
- 实现首版聊天主界面：消息流、输入区、基础快捷键（提交、退出、滚动/导航）、错误可见化。
- 复用现有会话与 Agent 执行能力，避免重复实现业务逻辑。
- 设计可扩展 UI 架构，为后续会话管理/任务面板留出结构空间。

**Non-Goals:**

- 本阶段不替换现有 CLI 主交互模式。
- 不在首版实现完整多面板（会话列表、配置中心、任务看板）。
- 不引入复杂主题系统或鼠标交互优先能力。

## Decisions

1. **新增并行命令入口**
   - 方案：新增 `tui` 子命令（名称可后续微调），与 `chat` 并存。
   - 理由：降低迁移风险，允许用户渐进采用。
   - 备选：直接替换 `chat`。未选原因：回归风险高、对老用户破坏性强。

2. **UI 与业务解耦**
   - 方案：Bubble Tea `Model` 仅负责状态与渲染，消息发送/会话持久化仍调用既有服务层接口。
   - 理由：最大化复用现有逻辑，减少行为偏差。
   - 备选：在 TUI 层重写会话流程。未选原因：重复逻辑、维护成本高。

3. **事件驱动的流式渲染**
   - 方案：将模型输出流映射为内部消息事件（增量 token、完成、错误）驱动 `Update`。
   - 理由：符合 Bubble Tea 的 Elm 架构，便于控制刷新与并发。
   - 备选：阻塞式一次性输出。未选原因：实时反馈差。

4. **跨平台兼容优先**
   - 方案：优先使用 Bubble Tea 官方能力与保守键位，避免依赖高风险终端特性。
   - 理由：降低 Windows 终端差异导致的不可用风险。
   - 备选：先做高级特性再兜底兼容。未选原因：首版稳定性不可控。

## Risks / Trade-offs

- **终端兼容差异（特别是 Windows）** -> 采用最小特性集合并在关键路径增加平台测试矩阵。
- **流式输出与 UI 循环并发复杂度** -> 通过消息队列边界和状态机约束，避免直接跨协程改 UI 状态。
- **新增入口带来维护面增加** -> 复用既有服务层并限制首版范围，控制维护成本。
```

## openspec/changes/add-bubbletea-tui-interface/tasks.md

- Source: openspec/changes/add-bubbletea-tui-interface/tasks.md
- Lines: 1-15
- SHA256: b3fad6a3acda8a8d7fa6638d6d5c8ae8ab7a87d941f23084456b8c4507cb997f

```md
## 1. 命令入口与依赖准备

- [ ] 1.1 新增 TUI 子命令入口并接入 CLI 命令树
- [ ] 1.2 引入 Bubble Tea 依赖并完成基础启动骨架

## 2. 聊天主界面实现

- [ ] 2.1 实现消息流视图与输入区状态模型
- [ ] 2.2 接入现有会话/agent 流程并展示回复结果
- [ ] 2.3 实现基础快捷键（发送、退出、基础导航）与错误提示

## 3. 兼容性与验证

- [ ] 3.1 在 Windows/macOS/Linux 验证启动、输入、回复、退出主路径
- [ ] 3.2 补充文档与使用说明（命令示例、已知限制、后续扩展点）
```

## openspec/changes/add-bubbletea-tui-interface/specs/tui-chat-interface/spec.md

- Source: openspec/changes/add-bubbletea-tui-interface/specs/tui-chat-interface/spec.md
- Lines: 1-40
- SHA256: c3b67845bda146b2861ecabf71b13b9daa5646970b50229f2e3b5b541a4f32a6

```md
## ADDED Requirements

### Requirement: 提供独立 TUI 聊天入口
系统 MUST 提供一个独立于现有 `chat`/`once` 的 TUI 命令入口，用于启动基于 Bubble Tea 的交互会话。

#### Scenario: 用户启动 TUI 命令
- **WHEN** 用户执行 TUI 子命令
- **THEN** 系统进入 TUI 聊天界面并显示初始可交互视图

### Requirement: 支持消息流与输入交互
系统 MUST 在 TUI 中提供可阅读的消息展示区与输入区域，允许用户发起多轮会话并查看模型回复。

#### Scenario: 用户提交一条消息
- **WHEN** 用户在输入区输入文本并触发发送
- **THEN** 用户消息出现在消息流中，系统开始处理并展示助手回复

### Requirement: 支持基础会话控制快捷键
系统 MUST 提供基础快捷键能力以保证可用性，包括发送、退出会话、中断当前轮与基础导航。默认语义为：`Enter` 发送、`Ctrl+C` 退出会话、`Esc` 中断当前轮、`↑↓` 与 `j/k` 导航。

#### Scenario: 用户触发退出快捷键
- **WHEN** 用户在 TUI 中按下定义的退出快捷键
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
```

