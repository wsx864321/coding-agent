# Comet Design Handoff

- Change: tui-overlays
- Phase: design
- Mode: compact
- Context hash: 83ebb165e968f010e848e7ce2b8b56ff17aefdfc9eae8a6e39366dbd36846c1a

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/tui-overlays/proposal.md

- Source: openspec/changes/tui-overlays/proposal.md
- Lines: 1-38
- SHA256: a344b820212b0440ad2155063b921493e4e3c70bbac1e8da4cb46e4aa0c31df8

```md
## Why

当前 TUI 仅支持 y/n 审批横幅这一种模态交互，缺少 Skill 选择器、MCP 管理器、会话恢复选择器、Rewind 检查点、模型切换器、/clear 确认、Ask 多选题卡片等覆盖层/模态系统。对标 Reasonix 的覆盖层体系，需要实现完整的模态交互框架。

## What Changes

- **新增 Skill 选择器覆盖层**：/skills 打开，浏览已加载 Skill，启用/禁用，查看详情
- **新增 MCP 管理器覆盖层**：/mcp 打开，查看 MCP 服务器状态，连接/断开
- **新增会话恢复选择器**：/resume 打开，浏览历史会话，选择恢复
- **新增 Rewind 检查点选择器**：Esc-Esc 触发，浏览快照回退
- **新增模型切换器**：/model 打开，选择 provider/model
- **新增 /clear 确认对话框**：/clear 时弹出确认，防止误操作
- **新增 Ask 多选题卡片**：ask 工具的 UI 渲染 + 键盘交互（数字选择/自由输入）
- **增强审批横幅**：显示工具名 + 参数摘要 + y/n/a 选项

## Capabilities

### New Capabilities

- `skill-picker`: Skill 选择器覆盖层（浏览、启用/禁用、查看详情）
- `mcp-manager`: MCP 管理器覆盖层（查看状态、连接/断开）
- `resume-picker`: 会话恢复选择器（浏览历史、选择恢复）
- `rewind-picker`: Rewind 检查点选择器（Esc-Esc 触发，快照回退）
- `model-switcher`: 模型切换器（/model 选择 provider/model）
- `clear-confirm`: /clear 确认对话框
- `ask-card`: Ask 工具多选题卡片渲染与交互
- `approval-enhance`: 审批横幅增强（参数摘要 + y/n/a）

### Modified Capabilities

- `tui-chat-interface`: 修改"交互式审批"需求，增加参数摘要显示和 always 选项

## Impact

- `internal/tui/model.go`: 新增 skillPick、mcp、resumePick、rewind、modelPicker、clearConfirm、chooser 等覆盖层状态字段
- 新建 `internal/tui/overlays/` 子包：各覆盖层的渲染与键盘处理
- `internal/tui/view.go`: 修改 View() 集成覆盖层渲染
- `internal/tui/approval.go`: 增强审批横幅渲染
```

## openspec/changes/tui-overlays/design.md

- Source: openspec/changes/tui-overlays/design.md
- Lines: 1-39
- SHA256: 5eea8238dd36f111e45bf21ac250f7d85d9951f20b344e70d57940ad21b250d6

```md
## Context

当前 TUI 仅有一个模态交互：审批横幅（y/n）。Reasonix 拥有 8+ 种覆盖层/模态交互。本次实现覆盖层框架和全部覆盖层类型。

## Goals / Non-Goals

**Goals:**
- 覆盖层框架：统一的生命周期（打开 → 键盘独占 → 关闭）
- Skill 选择器、MCP 管理器、会话恢复、Rewind、模型切换、/clear 确认、Ask 卡片
- 审批横幅增强

**Non-Goals:**
- 不实现底层能力（检查点存储、MCP 连接管理等），仅 UI 层
- 不改变 agent 核心逻辑

## Decisions

### D1: 覆盖层使用独立结构体 + Model 字段

**选择**: 每个覆盖层类型对应一个独立结构体（如 `skillPicker`、`mcpManager`），在 Model 中以指针字段存储（nil = 关闭）。Update 中按优先级检查覆盖层状态，有覆盖层时键盘事件优先路由到覆盖层。

**理由**: 与 Reasonix 相同模式。每个覆盖层独立管理自己的状态，Model 仅负责路由。

### D2: 覆盖层渲染在 View() 中内联

**选择**: 覆盖层渲染为独立区域，显示在 viewport 和输入区之间（或替换 viewport）。每个覆盖层有自己的 `render()` 方法。

**理由**: 覆盖层需要占用屏幕空间，放在 viewport 和输入区之间是最自然的布局。

### D3: Ask 卡片使用 chooser 结构体

**选择**: Ask 工具的多选题卡片使用 `chooser` 结构体，支持单选（数字键）、多选（空格切换 + Enter 确认）、自由输入（键入文本 + Enter）。

**理由**: Reasonix 使用相同设计。chooser 覆盖了 ask 工具的所有交互模式。

## Risks / Trade-offs

- **[复杂度] 覆盖层数量**: 8 种覆盖层增加 Model 复杂度。→ 每种覆盖层独立文件，通过接口统一生命周期。
- **[键盘冲突] 覆盖层与全局快捷键**: 覆盖层激活时某些全局快捷键（如 Ctrl+C）仍需工作。→ 在覆盖层处理中保留 Ctrl+C 退出逻辑。
```

## openspec/changes/tui-overlays/tasks.md

- Source: openspec/changes/tui-overlays/tasks.md
- Lines: 1-69
- SHA256: fb07c339fb421b6763d700272112f6e29dca5702b8831dc11f1b15bdd630c555

```md
## 1. 覆盖层框架

- [ ] 1.1 定义覆盖层接口（Open、Close、Update、View、Active）
- [ ] 1.2 在 `internal/tui/model.go` 中新增覆盖层路由逻辑（按优先级检查覆盖层状态）
- [ ] 1.3 修改 `internal/tui/view.go` 集成覆盖层渲染区域

## 2. Skill 选择器

- [ ] 2.1 新建 `internal/tui/overlays/skillpicker.go`：实现 skillPicker 结构体
- [ ] 2.2 实现 Skill 列表渲染（名称、描述、启用/禁用状态）
- [ ] 2.3 实现键盘交互：↑↓ 导航、Enter 查看详情、Space 启用/禁用、Esc 关闭
- [ ] 2.4 在 Model 中处理 `/skills` 命令打开选择器

## 3. MCP 管理器

- [ ] 3.1 新建 `internal/tui/overlays/mcpmanager.go`：实现 mcpManager 结构体
- [ ] 3.2 实现 MCP 服务器列表渲染（名称、状态、工具数）
- [ ] 3.3 实现键盘交互：↑↓ 导航、Enter 连接/断开、R 重连、Esc 关闭
- [ ] 3.4 在 Model 中处理 `/mcp` 命令打开管理器

## 4. 会话恢复选择器

- [ ] 4.1 新建 `internal/tui/overlays/resumepicker.go`：实现 resumePicker 结构体
- [ ] 4.2 实现会话列表渲染（日期、模型、预览）
- [ ] 4.3 实现键盘交互：↑↓ 导航、Enter 恢复、Esc 关闭
- [ ] 4.4 在 Model 中处理 `/resume` 命令打开选择器

## 5. Rewind 检查点选择器

- [ ] 5.1 新建 `internal/tui/overlays/rewindpicker.go`：实现 rewindPicker 结构体
- [ ] 5.2 实现检查点列表渲染（时间、描述）
- [ ] 5.3 实现键盘交互：↑↓ 导航、Enter 回退、Esc 关闭
- [ ] 5.4 在 Model 中处理 Esc-Esc 打开选择器

## 6. 模型切换器

- [ ] 6.1 新建 `internal/tui/overlays/modelpicker.go`：实现 modelPicker 结构体
- [ ] 6.2 实现模型列表渲染（provider/model、当前标记）
- [ ] 6.3 实现键盘交互：↑↓ 导航、Enter 切换、Esc 关闭
- [ ] 6.4 在 Model 中处理 `/model` 命令打开切换器

## 7. /clear 确认对话框

- [ ] 7.1 新建 `internal/tui/overlays/clearconfirm.go`：实现 clearConfirm 结构体
- [ ] 7.2 实现确认对话框渲染（警告文本 + y/n 选项）
- [ ] 7.3 实现键盘交互：y 确认清除、n/Esc 取消
- [ ] 7.4 在 Model 中处理 `/clear` 命令打开确认框

## 8. Ask 多选题卡片

- [ ] 8.1 新建 `internal/tui/overlays/chooser.go`：实现 chooser 结构体
- [ ] 8.2 实现单选题渲染（数字选项 + 描述）
- [ ] 8.3 实现多选题渲染（空格切换 + Enter 确认）
- [ ] 8.4 实现自由输入模式（键入文本 + Enter 提交）
- [ ] 8.5 在 Model 的 ApprovalRequest 处理中检测 ask 工具并打开 chooser

## 9. 审批横幅增强

- [ ] 9.1 修改 `internal/tui/approval.go`：增加参数摘要显示（工具名 + 关键参数）
- [ ] 9.2 增加 "always" 选项（a 键）：批准本次及后续同类工具调用
- [ ] 9.3 增加工具类别图标（读/写/执行/进程）

## 10. 集成测试

- [ ] 10.1 为覆盖层路由逻辑编写单元测试
- [ ] 10.2 为 Skill 选择器编写单元测试
- [ ] 10.3 为 Ask 卡片编写单元测试
- [ ] 10.4 为审批横幅增强编写单元测试
- [ ] 10.5 运行全量测试套件确认无回归
```

## openspec/changes/tui-overlays/specs/approval-enhance/spec.md

- Source: openspec/changes/tui-overlays/specs/approval-enhance/spec.md
- Lines: 1-19
- SHA256: da483ee9f84a50008c27bd04d9e8e0fbd1b732e51570e57e2eb536696dd2ca6e

```md
## ADDED Requirements

### Requirement: 审批横幅参数摘要
系统 SHALL 在审批横幅中显示工具调用的关键参数摘要，帮助用户快速判断是否批准。

#### Scenario: 显示参数摘要
- **WHEN** 审批横幅显示
- **THEN** 横幅包含工具名 + 关键参数值（如 `Bash("go test ./...")`）

#### Scenario: 敏感参数脱敏
- **WHEN** 参数包含敏感 key（password、token、secret 等）
- **THEN** 参数值显示为 "***"

### Requirement: Always 批准选项
系统 SHALL 在审批横幅中提供 "always" 选项（a 键），批准本次及后续同类工具调用。

#### Scenario: Always 批准
- **WHEN** 用户按 a
- **THEN** 本次工具调用被批准，后续同类工具调用自动批准
```

## openspec/changes/tui-overlays/specs/ask-card/spec.md

- Source: openspec/changes/tui-overlays/specs/ask-card/spec.md
- Lines: 1-20
- SHA256: c7fb6677c2bae9d47ef1b29342f5000069eebc2a82b63f3d91047a2ec8b42865

```md
## ADDED Requirements

### Requirement: Ask 多选题卡片
系统 SHALL 在 agent 调用 ask 工具时渲染多选题卡片覆盖层，支持单选、多选和自由输入。

#### Scenario: 单选题
- **WHEN** ask 工具参数中 multiSelect 为 false
- **THEN** 卡片显示选项列表，用户按数字键选择

#### Scenario: 多选题
- **WHEN** ask 工具参数中 multiSelect 为 true
- **THEN** 卡片显示选项列表，用户按空格切换选择，Enter 确认

#### Scenario: 自由输入
- **WHEN** ask 工具包含 "type something" 选项
- **THEN** 用户可键入自定义文本，Enter 提交

#### Scenario: 关闭卡片
- **WHEN** 用户按 Esc
- **THEN** 卡片关闭，返回空答案
```

## openspec/changes/tui-overlays/specs/clear-confirm/spec.md

- Source: openspec/changes/tui-overlays/specs/clear-confirm/spec.md
- Lines: 1-16
- SHA256: 8eb0383ebccf69aa88c2ea029c0922801c268156f37e03c231987d0606b4dd38

```md
## ADDED Requirements

### Requirement: /clear 确认对话框
系统 SHALL 在用户输入 `/clear` 时弹出确认对话框，防止误操作清除会话。

#### Scenario: 触发确认
- **WHEN** 用户输入 `/clear` 并按 Enter
- **THEN** 确认对话框显示警告文本和 y/n 选项

#### Scenario: 确认清除
- **WHEN** 用户按 y
- **THEN** 会话清除，对话框关闭

#### Scenario: 取消清除
- **WHEN** 用户按 n 或 Esc
- **THEN** 会话保留，对话框关闭
```

## openspec/changes/tui-overlays/specs/mcp-manager/spec.md

- Source: openspec/changes/tui-overlays/specs/mcp-manager/spec.md
- Lines: 1-16
- SHA256: d3ff20d2bb4ea3971c39a8915549ddc12c4d6b052519e6befbac1a784ebf20a9

```md
## ADDED Requirements

### Requirement: MCP 管理器覆盖层
系统 SHALL 在用户输入 `/mcp` 时打开 MCP 管理器覆盖层，显示已配置的 MCP 服务器及其连接状态。

#### Scenario: 打开 MCP 管理器
- **WHEN** 用户输入 `/mcp` 并按 Enter
- **THEN** 覆盖层显示 MCP 服务器列表（名称、状态、工具数）

#### Scenario: 连接/断开服务器
- **WHEN** 用户选择服务器并按 Enter
- **THEN** 服务器连接状态切换（连接 ↔ 断开）

#### Scenario: 关闭管理器
- **WHEN** 用户按 Esc
- **THEN** MCP 管理器关闭
```

## openspec/changes/tui-overlays/specs/model-switcher/spec.md

- Source: openspec/changes/tui-overlays/specs/model-switcher/spec.md
- Lines: 1-16
- SHA256: 785f0216a2e814ed1ce41816173a32bd5e048fffe92afa3a10e4f95bf52a6a15

```md
## ADDED Requirements

### Requirement: 模型切换器
系统 SHALL 在用户输入 `/model` 时打开模型切换器覆盖层，显示可用模型列表。

#### Scenario: 打开模型切换器
- **WHEN** 用户输入 `/model` 并按 Enter
- **THEN** 覆盖层显示可用模型列表（provider/model），当前模型标记

#### Scenario: 切换模型
- **WHEN** 用户选择模型并按 Enter
- **THEN** 模型切换，覆盖层关闭，状态栏更新模型名

#### Scenario: 关闭切换器
- **WHEN** 用户按 Esc
- **THEN** 模型切换器关闭
```

## openspec/changes/tui-overlays/specs/resume-picker/spec.md

- Source: openspec/changes/tui-overlays/specs/resume-picker/spec.md
- Lines: 1-16
- SHA256: 072e1be2c16acdbebf05ece09dee2547ab8bd32a366b9e2b08adc449ad13af55

```md
## ADDED Requirements

### Requirement: 会话恢复选择器
系统 SHALL 在用户输入 `/resume` 时打开会话恢复选择器覆盖层，显示历史会话列表。

#### Scenario: 打开会话恢复选择器
- **WHEN** 用户输入 `/resume` 并按 Enter
- **THEN** 覆盖层显示历史会话列表（日期、模型、预览）

#### Scenario: 恢复会话
- **WHEN** 用户选择会话并按 Enter
- **THEN** 该会话被恢复，覆盖层关闭，消息流显示历史记录

#### Scenario: 关闭选择器
- **WHEN** 用户按 Esc
- **THEN** 会话恢复选择器关闭
```

## openspec/changes/tui-overlays/specs/rewind-picker/spec.md

- Source: openspec/changes/tui-overlays/specs/rewind-picker/spec.md
- Lines: 1-16
- SHA256: 777b731838db239d4011d82d7de489a233cbee64e019973a98ed7bf22a93060c

```md
## ADDED Requirements

### Requirement: Rewind 检查点选择器
系统 SHALL 在用户双击 Esc（两次 Esc 间隔 < 600ms）时打开 Rewind 检查点选择器覆盖层。

#### Scenario: 触发 Rewind
- **WHEN** 输入区为空且用户在 600ms 内按两次 Esc
- **THEN** Rewind 选择器打开，显示可用检查点列表

#### Scenario: 回退到检查点
- **WHEN** 用户选择检查点并按 Enter
- **THEN** 文件系统回退到该检查点状态

#### Scenario: 关闭选择器
- **WHEN** 用户按 Esc
- **THEN** Rewind 选择器关闭
```

## openspec/changes/tui-overlays/specs/skill-picker/spec.md

- Source: openspec/changes/tui-overlays/specs/skill-picker/spec.md
- Lines: 1-20
- SHA256: d8b36a7f9fb5b24f25cf886fd0057d19dda3fe72dcd3eeff9dbeb2d00fadc051

```md
## ADDED Requirements

### Requirement: Skill 选择器覆盖层
系统 SHALL 在用户输入 `/skills` 时打开 Skill 选择器覆盖层，显示已加载的 Skill 列表及其启用/禁用状态。

#### Scenario: 打开 Skill 选择器
- **WHEN** 用户输入 `/skills` 并按 Enter
- **THEN** 覆盖层显示 Skill 列表（名称、描述、状态）

#### Scenario: 浏览 Skill
- **WHEN** Skill 选择器打开且用户按 ↑↓
- **THEN** 高亮项在列表中移动

#### Scenario: 查看 Skill 详情
- **WHEN** 用户按 Enter 选择某个 Skill
- **THEN** 显示该 Skill 的详细描述

#### Scenario: 关闭选择器
- **WHEN** 用户按 Esc
- **THEN** Skill 选择器关闭
```

## openspec/changes/tui-overlays/specs/tui-chat-interface/spec.md

- Source: openspec/changes/tui-overlays/specs/tui-chat-interface/spec.md
- Lines: 1-20
- SHA256: 3b2ddcbd08ac6d9b5c605386b6e7e512f9c2cbc5dca0be12d44822c8c722008d

```md
## MODIFIED Requirements

### Requirement: 交互式审批
系统 MUST 对需要权限确认的工具调用提供交互式审批横幅，允许用户在 TUI 内批准或拒绝操作。横幅 MUST 显示工具名称和关键参数摘要。支持 y（批准）、n（拒绝）、a（批准本次及后续同类调用）三种操作。

#### Scenario: 需要权限的工具调用
- **WHEN** agent 发起需要用户确认的工具调用
- **THEN** TUI 显示审批横幅，包含工具名、参数摘要和操作选项（`[y]es` / `[n]o` / `[a]lways`）

#### Scenario: 用户批准操作
- **WHEN** 审批横幅显示中用户按 y
- **THEN** 工具调用被执行，审批横幅消失

#### Scenario: 用户拒绝操作
- **WHEN** 审批横幅显示中用户按 n
- **THEN** 工具调用被拒绝并返回 "Permission denied" 结果

#### Scenario: 用户选择 Always
- **WHEN** 审批横幅显示中用户按 a
- **THEN** 本次及后续同类工具调用自动批准
```

