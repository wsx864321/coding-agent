# todo-panel Specification

## Purpose
TBD - created by archiving change tui-status-panels. Update Purpose after archive.
## Requirements
### Requirement: Todo 任务面板
系统 SHALL 在输入区上方渲染 Todo 任务面板，解析 agent 的 todo_write 工具调用，以结构化列表展示任务状态和名称。

#### Scenario: agent 调用 todo_write
- **WHEN** agent 发起 todo_write 工具调用
- **THEN** 系统解析 JSON 参数中的任务列表，在输入区上方渲染 Todo 面板

#### Scenario: Todo 面板渲染格式
- **WHEN** Todo 面板有任务
- **THEN** 每个任务显示状态图标（⏳ pending / ⟳ in_progress / ✓ completed）和任务名称，当前进行中的任务高亮

#### Scenario: 任务列表为空
- **WHEN** todo_write 参数中任务列表为空
- **THEN** Todo 面板不渲染（不占空间）

#### Scenario: 跨 turn 保持
- **WHEN** 一个 turn 完成且新的 turn 开始
- **THEN** Todo 面板保持上一次 todo_write 的状态，直到新的 todo_write 更新

