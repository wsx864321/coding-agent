# Comet Design Handoff

- Change: tui-render-engine
- Phase: design
- Mode: compact
- Context hash: cf49c423e05e920f5163a2e57b5ae060186584548506b7e03e46ea41ffccf0d9

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/tui-render-engine/proposal.md

- Source: openspec/changes/tui-render-engine/proposal.md
- Lines: 1-38
- SHA256: d0c78534487dba4b2987c34c64d421988f58397ecda893aa9e8a531e9d0e1f82

```md
## Why

当前 TUI 的渲染能力仅覆盖基础的消息流、工具卡片和 Markdown 渲染，与 Reasonix 等成熟终端 AI 编码助手的渲染体验存在显著差距。具体表现为：无思考/推理文本展示、工具输出仅展示最终结果而无实时流式反馈、Shell 输出无法交互式折叠/展开、Markdown 代码块缺少语法高亮、无文本选择与复制能力、无 Diff 视图渲染。本次升级将 TUI 渲染引擎提升至生产级水平，对标 Reasonix 的核心渲染体验。

## What Changes

- **新增思考/推理块渲染**：将 LLM 的 reasoning/thinking 文本以折叠块展示（默认折叠），支持 Ctrl+O 切换展开/折叠，实时流式追加
- **新增工具输出实时流式渲染**：引入 ToolProgress 事件类型，工具执行过程中实时流式展示输出（尾部截断），替代当前仅展示最终结果的模式
- **新增 Shell 输出折叠/展开交互**：Shell 命令输出默认折叠（显示前 N 行 + 行数摘要），支持 Ctrl+B 切换完整输出
- **升级 Markdown 渲染**：代码块集成 chroma 语法高亮（当前已声明但未完整启用），新增 Diff 视图渲染（+/- 行着色），完善 GFM 表格渲染
- **新增文本选择与复制**：支持鼠标拖拽选中 viewport 中的文本，Ctrl+C / Super+C 在选中时复制到系统剪贴板
- **新增 Diff 视图渲染**：识别 diff 格式文本并以 +/- 着色渲染，支持 /diff-fold 控制折叠行数

## Capabilities

### New Capabilities

- `reasoning-display`: LLM 思考/推理文本的折叠展示与流式渲染，支持 Ctrl+O 切换展开/折叠
- `tool-streaming`: 工具执行过程中的实时流式输出展示，尾部截断 + 行数计数
- `shell-output-toggle`: Shell 命令输出的交互式折叠/展开，Ctrl+B 切换
- `text-selection`: viewport 文本的鼠标选择与系统剪贴板复制
- `diff-view`: Diff 格式文本的 +/- 着色渲染与折叠控制

### Modified Capabilities

- `tui-chat-interface`: 增强 Markdown 渲染（chroma 语法高亮、diff 视图、表格完善）、增强工具卡片（流式输出）、增强进度指示（工具进度事件）

## Impact

- `internal/tui/model.go`: 新增 reasoning/pending 状态字段、ToolProgress 事件处理、文本选择状态
- `internal/tui/transcript.go`: 新增推理块/流式输出块的渲染逻辑
- `internal/tui/toolcard.go`: 新增流式输出渲染、Shell 折叠/展开
- `internal/tui/markdown.go`: 升级 chroma 语法高亮集成、diff 视图渲染
- `internal/tui/view.go`: 新增文本选择渲染、diff 视图渲染
- `internal/tui/components.go`: 可能新增 diff 视图组件
- `internal/event/event.go`: 可能需要新增 ToolProgress 事件类型
- `internal/tui/sink.go`: 可能需要新增 ToolProgress 事件转发
- `go.mod`: 可能需要新增 chroma 依赖（若当前未直接依赖）
```

## openspec/changes/tui-render-engine/design.md

- Source: openspec/changes/tui-render-engine/design.md
- Lines: 1-91
- SHA256: 3f4fd8db26b080ac53f69d3a2e62c559d5ce1d8b849cb836f71eaa0300e79f82

[TRUNCATED]

```md
## Context

当前 TUI 渲染引擎基于 Bubble Tea v2 + glamour (chroma) 实现基础的消息流、工具卡片和 Markdown 渲染。事件系统定义了 6 种事件类型（Text、ToolDispatch、ToolResult、ApprovalRequest、TurnDone、Notice），TUI Model 通过 channel 接收事件并驱动渲染。

Reasonix 的 TUI 渲染引擎在此基础上增加了：思考/推理块（reasoning block）、工具实时流式输出（ToolProgress）、Shell 输出折叠/展开（Ctrl+B）、文本选择与剪贴板复制、Diff 视图渲染。这些能力需要在当前架构中增量添加。

## Goals / Non-Goals

**Goals:**
- 实现 LLM 思考/推理文本的折叠展示（默认折叠，Ctrl+O 展开）
- 实现工具执行过程中的实时流式输出（ToolProgress 事件 → 尾部截断渲染）
- 实现 Shell 命令输出的交互式折叠/展开（Ctrl+B 切换）
- 升级 Markdown 渲染：chroma 语法高亮、diff 视图、表格完善
- 实现 viewport 文本的鼠标选择与剪贴板复制
- 实现 Diff 格式文本的 +/- 着色渲染

**Non-Goals:**
- 不涉及状态栏重构（Change B）
- 不涉及输入系统改造（Change C）
- 不涉及覆盖层/模态系统（Change D）
- 不改变现有事件系统的核心架构（仅新增事件类型）
- 不引入新的外部依赖（优先使用已有依赖 glamour/chroma + lipgloss）

## Decisions

### D1: 推理文本使用独立事件类型 `ReasoningText`

**选择**: 新增 `event.ReasoningText` 事件类型，携带增量文本 chunk。

**替代方案**:
- 复用 `event.Text` + flag 区分：语义不清晰，增加判断分支
- 在 TurnDone 中携带完整推理文本：失去实时流式体验

**理由**: 推理文本与回答文本在渲染上完全不同（折叠 vs 直接展示），独立事件类型使 Model.Update 的分发逻辑清晰，且不影响现有 Text 事件的处理。

### D2: 工具流式输出使用 `ToolProgress` 事件

**选择**: 新增 `event.ToolProgress` 事件类型，携带 `ToolCallID string` + `Chunk string`。

**替代方案**:
- 复用 `ToolResult` 多次发送：破坏 ToolResult 的"最终结果"语义
- 在 ToolDispatch 后持续更新同一块：需要额外的状态管理，不如事件驱动清晰

**理由**: Reasonix 使用相同模式。ToolProgress 在 ToolDispatch 和 ToolResult 之间发送，Model 在 transcript 中维护一个"流式输出块"，每次收到 ToolProgress 时原地更新该块（尾部截断 + 行数计数）。

### D3: Shell 折叠使用内存 map 存储完整输出

**选择**: 在 Model 中维护 `shellOutputs map[string]string`（toolCallID → 完整输出）和 `shellExpanded map[string]bool`（展开状态），Ctrl+B 切换时原地重写 transcript 块。

**替代方案**:
- 写入临时文件：增加 I/O 开销，且需要管理文件生命周期
- 使用 transcript entry 的 Raw 字段存储完整输出：Raw 字段当前用于工具卡片参数，语义冲突

**理由**: Shell 输出通常在 KB 级别，内存存储足够。map 结构支持 O(1) 查找和更新。

### D4: Markdown 渲染保持 glamour 但启用 chroma 语法高亮

**选择**: 继续使用 glamour 渲染器，配置 `glamour.WithChromaStyle()` 启用 chroma 语法高亮，同时自定义 diff 视图的渲染逻辑（在 glamour 输出之上叠加 lipgloss 着色）。

**替代方案**:
- 替换为 goldmark 直接渲染：增加新依赖，glamour 已满足需求
- 仅使用 lipgloss 手动渲染：代码块语法高亮需自行实现，工作量大

**理由**: glamour 内置 chroma 支持，只需正确配置即可启用语法高亮。Diff 视图在 glamour 渲染后对 `+`/`-` 行进行二次着色。

### D5: 文本选择使用 Bubble Tea 的 MouseMode + 自定义 selection 状态

**选择**: 在 Model 中维护 `selection` 结构体（startLine, startCol, endLine, endCol, active），MouseMode 设为 CellMotion，在 Update 中处理 MouseMsg（左键按下开始选择、拖动扩展、释放结束）。Ctrl+C 在选中时复制到剪贴板（使用 `github.com/atotto/clipboard`）。

**替代方案**:
- 使用终端原生选择（Shift+鼠标）：依赖终端模拟器支持，不可控
- 使用 viewport 内置选择：Bubble Tea viewport 不支持文本选择

**理由**: Reasonix 使用相同方案。Bubble Tea 的 MouseModeCellMotion 提供逐 cell 的鼠标事件，足以实现精确的文本选择。

### D6: Diff 视图在 Markdown 渲染后叠加着色

**选择**: 在 `renderEntry` 中检测内容是否为 diff 格式（连续 `+`/`-`/`@@` 行），若是则对 `+` 行应用绿色、`-` 行应用红色、`@@` 行应用青色。

**替代方案**:
```

Full source: openspec/changes/tui-render-engine/design.md

## openspec/changes/tui-render-engine/tasks.md

- Source: openspec/changes/tui-render-engine/tasks.md
- Lines: 1-61
- SHA256: f8e71fee055482aa2ef2a77310ef6cbf64bd93b9ed6005812d21daa8c93d8857

```md
## 1. 事件系统扩展

- [ ] 1.1 在 `internal/event/event.go` 中新增 `ReasoningText` 事件类型（Kind 枚举 + Text 字段复用）
- [ ] 1.2 在 `internal/event/event.go` 中新增 `ToolProgress` 事件类型（Kind 枚举 + ToolCallID + Chunk 字段）
- [ ] 1.3 在 `internal/tui/sink.go` 中确保新事件类型正确转发到 TUI channel

## 2. 推理文本渲染

- [ ] 2.1 在 `internal/tui/model.go` 中新增 reasoning 相关状态字段（reasoning builder、reasoningLineIdx、showReasoning、thinkStart）
- [ ] 2.2 在 `internal/tui/model.go` 的 Update 中处理 `ReasoningText` 事件：累积推理文本、更新摘要行
- [ ] 2.3 在 `internal/tui/transcript.go` 中实现推理块渲染（折叠摘要行 + dim 样式展开内容）
- [ ] 2.4 在 `internal/tui/model.go` 中实现 Ctrl+O 切换 `showReasoning` 状态
- [ ] 2.5 在 `internal/tui/model.go` 中处理推理完成（Text 事件到达时提交推理摘要行）

## 3. 工具流式输出

- [ ] 3.1 在 `internal/tui/model.go` 中新增流式输出状态字段（toolStreamIdx、toolStreamID、toolTail、toolPartial、toolLineCount）
- [ ] 3.2 在 `internal/tui/model.go` 的 Update 中处理 `ToolProgress` 事件：追加 chunk、尾部截断、更新行数计数
- [ ] 3.3 在 `internal/tui/toolcard.go` 中实现流式输出块渲染（"⎿ working · Ns" + 尾部行 + 行数摘要）
- [ ] 3.4 在 `internal/tui/model.go` 中处理 ToolResult 事件时将流式输出转为折叠摘要
- [ ] 3.5 实现事件合并（drain loop）：高频 ToolProgress 事件批量处理，每帧最多重渲染一次

## 4. Shell 输出折叠/展开

- [ ] 4.1 在 `internal/tui/model.go` 中新增 `shellOutputs map[string]string` 和 `shellExpanded map[string]bool`
- [ ] 4.2 在 `internal/tui/model.go` 的 ToolResult 处理中识别 Shell 工具（bash），存储完整输出到 shellOutputs
- [ ] 4.3 在 `internal/tui/toolcard.go` 中实现 Shell 输出折叠渲染（前 8 行 + "⎿ N lines, collapsed"）
- [ ] 4.4 在 `internal/tui/model.go` 中实现 Ctrl+B 切换最近 Shell 输出块的展开/折叠
- [ ] 4.5 在 `internal/tui/transcript.go` 中实现展开/折叠时原地重写 transcript 块

## 5. Markdown 渲染升级

- [ ] 5.1 在 `internal/tui/markdown.go` 中配置 glamour 启用 chroma 语法高亮（`glamour.WithChromaStyle()`）
- [ ] 5.2 验证代码块语法高亮在常见语言（Go、Python、JavaScript、Bash、Diff）上正确工作
- [ ] 5.3 在 `internal/tui/markdown.go` 中新增 Diff 视图渲染：检测 diff 格式代码块，对 +/- 行叠加 lipgloss 着色
- [ ] 5.4 在 `internal/tui/transcript.go` 的 `renderEntry` 中集成 diff 视图检测与渲染

## 6. 文本选择与复制

- [ ] 6.1 在 `internal/tui/model.go` 中新增 `selection` 结构体（startLine、startCol、endLine、endCol、active）
- [ ] 6.2 在 `internal/tui/model.go` 的 Update 中处理 MouseMsg：左键按下开始选择、拖动扩展、释放结束
- [ ] 6.3 在 `internal/tui/view.go` 中实现选择区域的反色高亮渲染
- [ ] 6.4 在 `internal/tui/model.go` 中实现 Ctrl+C 在选中时复制到剪贴板（使用 `github.com/atotto/clipboard`）
- [ ] 6.5 处理选择与滚动的交互（选择时滚轮/PgUp/PgDn 扩展选择范围）
- [ ] 6.6 添加 `github.com/atotto/clipboard` 依赖到 go.mod

## 7. Diff 视图渲染

- [ ] 7.1 在 `internal/tui/model.go` 中新增 `diffMaxLines` 字段（默认 0 = 不限制）
- [ ] 7.2 在 `internal/tui/toolcard.go` 或新建 `internal/tui/diffview.go` 中实现 diff 格式检测与 +/- 着色
- [ ] 7.3 实现 `/diff-fold` 斜杠命令处理（在 TUI 输入中识别并更新 diffMaxLines）
- [ ] 7.4 实现 diff 块超过阈值时的折叠渲染（"N more lines" 提示）

## 8. 集成测试与验证

- [ ] 8.1 为推理文本渲染编写单元测试（ReasoningText 事件处理、折叠/展开切换）
- [ ] 8.2 为工具流式输出编写单元测试（ToolProgress 事件处理、尾部截断、行数计数）
- [ ] 8.3 为 Shell 输出折叠编写单元测试（存储、展开/折叠、Ctrl+B 切换）
- [ ] 8.4 为文本选择编写单元测试（选择状态机、剪贴板复制）
- [ ] 8.5 为 Diff 视图编写单元测试（格式检测、着色、折叠）
- [ ] 8.6 运行全量测试套件确认无回归
```

## openspec/changes/tui-render-engine/specs/diff-view/spec.md

- Source: openspec/changes/tui-render-engine/specs/diff-view/spec.md
- Lines: 1-27
- SHA256: ccb6d3f2a98f3e31350ecfaa4feb88ad8190e98568768cf10441ff8157fca392

```md
## ADDED Requirements

### Requirement: Diff 格式识别与着色
系统 SHALL 自动识别消息流中的 diff 格式文本，并以 +/- 着色渲染：新增行（`+` 前缀）显示为绿色，删除行（`-` 前缀）显示为红色，hunk 头（`@@` 前缀）显示为青色。

#### Scenario: 助手回复包含 diff 代码块
- **WHEN** 助手回复中的代码块内容为 unified diff 格式（包含 `@@`、`+`、`-` 行）
- **THEN** 代码块内的 `+` 行以绿色渲染，`-` 行以红色渲染，`@@` 行以青色渲染，上下文行保持默认颜色

#### Scenario: 非 diff 代码块不受影响
- **WHEN** 助手回复中的代码块内容不是 diff 格式
- **THEN** 代码块以常规语法高亮渲染，不应用 diff 着色

#### Scenario: 混合内容正确渲染
- **WHEN** 助手回复同时包含普通文本和 diff 代码块
- **THEN** 普通文本正常渲染，仅 diff 代码块应用 +/- 着色

### Requirement: Diff 折叠控制
系统 SHALL 支持通过 `/diff-fold` 命令控制 diff 视图的最大显示行数，超过阈值的 diff 块折叠显示。

#### Scenario: Diff 块超过折叠阈值
- **WHEN** diff 代码块行数超过 diffMaxLines（默认 0 = 不限制）
- **THEN** 仅显示前 diffMaxLines 行 + "N more lines (use /diff-fold to expand)" 提示

#### Scenario: 调整折叠阈值
- **WHEN** 用户输入 `/diff-fold 50`
- **THEN** diffMaxLines 更新为 50，后续 diff 块按新阈值折叠
```

## openspec/changes/tui-render-engine/specs/reasoning-display/spec.md

- Source: openspec/changes/tui-render-engine/specs/reasoning-display/spec.md
- Lines: 1-27
- SHA256: 5ebb5a49fa1e05d73df0806330d16158aea251ff0faa4b04c57e2f4c2795a593

```md
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
```

## openspec/changes/tui-render-engine/specs/shell-output-toggle/spec.md

- Source: openspec/changes/tui-render-engine/specs/shell-output-toggle/spec.md
- Lines: 1-38
- SHA256: 8c5323e915d794711c2d09be927c3c60c3672e2c5e87516dc4aad1dba3bc66b8

```md
## ADDED Requirements

### Requirement: Shell 输出折叠展示
系统 SHALL 对 Shell 命令（bash 工具）的输出默认折叠展示：仅显示前 N 行（默认 8 行）和总行数摘要（如 "⎿ 156 lines, collapsed"）。

#### Scenario: Shell 命令执行完成
- **WHEN** bash 工具返回输出结果
- **THEN** 输出以折叠形式展示：前 8 行 + "⎿ N lines, collapsed" 摘要行

#### Scenario: 短输出不折叠
- **WHEN** bash 工具输出行数 ≤ 8 行
- **THEN** 输出完整展示，不显示折叠摘要

### Requirement: Ctrl+B 切换 Shell 输出展开/折叠
系统 SHALL 支持用户通过 Ctrl+B 快捷键切换最近一个 Shell 输出块的展开/折叠状态。

#### Scenario: 用户展开 Shell 输出
- **WHEN** 存在折叠的 Shell 输出块且用户按下 Ctrl+B
- **THEN** 该输出块展开显示完整内容，摘要行消失

#### Scenario: 用户折叠 Shell 输出
- **WHEN** Shell 输出块已展开且用户按下 Ctrl+B
- **THEN** 该输出块重新折叠为前 8 行 + 摘要行

#### Scenario: 无 Shell 输出时 Ctrl+B 无操作
- **WHEN** 消息流中无 Shell 输出块且用户按下 Ctrl+B
- **THEN** 无任何变化

### Requirement: Shell 输出完整内容存储
系统 SHALL 在内存中存储 Shell 命令的完整输出内容，以支持展开/折叠切换。

#### Scenario: Shell 输出存储
- **WHEN** bash 工具返回输出结果
- **THEN** 完整输出内容存储在 Model 的 shellOutputs map 中（key 为 toolCallID）

#### Scenario: 内存限制
- **WHEN** Shell 输出超过 1MB
- **THEN** 系统截断存储（保留最后 1MB），并在摘要中标注 "output truncated"
```

## openspec/changes/tui-render-engine/specs/text-selection/spec.md

- Source: openspec/changes/tui-render-engine/specs/text-selection/spec.md
- Lines: 1-35
- SHA256: 8431e34b6ad096a482a1c0372a21c0e2facc8cab66e04fb9850c7cbcd868d68a

```md
## ADDED Requirements

### Requirement: 鼠标文本选择
系统 SHALL 支持用户通过鼠标拖拽在 viewport 消息流中选择文本。选择区域以反色高亮显示。

#### Scenario: 用户开始选择
- **WHEN** 用户在 viewport 区域按下鼠标左键
- **THEN** 系统记录选择起始位置（行、列），开始跟踪鼠标移动

#### Scenario: 用户拖动扩展选择
- **WHEN** 用户按住鼠标左键并拖动
- **THEN** 选择区域从起始位置扩展到当前鼠标位置，高亮区域实时更新

#### Scenario: 用户释放鼠标完成选择
- **WHEN** 用户释放鼠标左键
- **THEN** 选择区域保持高亮，等待用户复制操作

#### Scenario: 点击取消选择
- **WHEN** 存在选择区域且用户单击鼠标左键（非拖动）
- **THEN** 选择区域取消，高亮消失

### Requirement: 剪贴板复制
系统 SHALL 支持用户通过 Ctrl+C（或 Super+C / Meta+C）将选中的文本复制到系统剪贴板。

#### Scenario: 选中文本后复制
- **WHEN** 存在活动选择区域且用户按下 Ctrl+C
- **THEN** 选中文本被复制到系统剪贴板，选择区域取消，显示 "Copied N characters" 提示

#### Scenario: 无选择时 Ctrl+C 保持原有行为
- **WHEN** 无活动选择区域且用户按下 Ctrl+C
- **THEN** 保持原有行为（运行中取消当前 turn，空闲时清空输入或退出）

#### Scenario: 剪贴板不可用时降级
- **WHEN** 系统剪贴板不可用（如 headless 环境）
- **THEN** 显示 "Copied N characters (clipboard unavailable)" 提示
```

## openspec/changes/tui-render-engine/specs/tool-streaming/spec.md

- Source: openspec/changes/tui-render-engine/specs/tool-streaming/spec.md
- Lines: 1-31
- SHA256: eac60c4973e6e465d7b37da0eb98b6fe29608e3620f09f9f1a498788ea068b27

```md
## ADDED Requirements

### Requirement: 工具实时流式输出
系统 SHALL 在工具执行过程中实时展示流式输出，而非仅在工具完成后展示最终结果。流式输出以尾部截断方式渲染（保留最后 N 行），并显示实时行数计数。

#### Scenario: 工具开始执行
- **WHEN** agent 发起工具调用且工具开始产生输出
- **THEN** 工具卡片下方出现流式输出块，实时追加新行

#### Scenario: 流式输出持续更新
- **WHEN** 工具持续产生输出行
- **THEN** 流式输出块保留最后 N 行（尾部截断），行数计数实时更新（如 "⎿ 156 lines"）

#### Scenario: 工具执行完成
- **WHEN** 工具调用返回最终结果
- **THEN** 流式输出块转为折叠摘要（显示前 M 行 + 总行数），与当前 ToolResult 行为一致

#### Scenario: 高频输出不阻塞 UI
- **WHEN** 工具以高频产生输出（如每秒数百行）
- **THEN** 系统合并事件批量更新，每帧最多重渲染一次，UI 保持响应

### Requirement: ToolProgress 事件类型
系统 SHALL 定义 ToolProgress 事件类型，携带工具调用标识（ToolCallID）和增量输出文本（Chunk），在 ToolDispatch 和 ToolResult 之间发送。

#### Scenario: ToolProgress 事件发送
- **WHEN** 工具执行过程中产生输出
- **THEN** 系统发送 ToolProgress 事件到 TUI 事件通道

#### Scenario: ToolProgress 事件被 TUI 消费
- **WHEN** TUI Model 收到 ToolProgress 事件
- **THEN** Model 找到对应工具的流式输出块并追加新内容，触发 viewport 重渲染
```

## openspec/changes/tui-render-engine/specs/tui-chat-interface/spec.md

- Source: openspec/changes/tui-render-engine/specs/tui-chat-interface/spec.md
- Lines: 1-58
- SHA256: 5dce409b8e2dcf7751aa18be479b734b94a8cee295fa49159c009619b51df4a6

```md
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
```

