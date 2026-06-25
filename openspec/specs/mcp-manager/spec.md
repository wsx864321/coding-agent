# mcp-manager Specification

## Purpose
TBD - created by archiving change tui-overlays. Update Purpose after archive.
## Requirements
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

