## Why

当前 hook 系统采用进程内 Go 函数回调模式（`hooks.Registry` + 类型化函数切片），所有 hook 必须用 Go 编写并在编译时绑定。这限制了用户使用其他语言编写扩展逻辑的能力，也无法通过配置文件声明式管理 hook。参照 DeepSeek-Reasonix 的外部 shell 命令 hook 模式，改造为语言无关、配置驱动的 hook 体系。

## What Changes

- **移除** `internal/hooks/` 下的 `Registry` 和全部内置 hook（LogHook、LargeOutputHook、ContextInjectHook、SummaryHook）
- **新增** 基于外部 shell 命令的 hook 执行引擎：`Runner` + `Spawner` + `HookConfig` 配置加载
- **新增** `ToolHooks` interface 解耦 Agent 与 hook 包的直接依赖
- **新增** JSON 配置文件加载（全局 `~/.coding-agent/hooks.json` + 项目 `.coding-agent/hooks.json`）
- **迁移** TodoGuardHook 逻辑从 hook 体系移入 agent 主循环，成为核心行为
- **修改** Agent 触发点从 `Registry.Trigger*()` 改为 `ToolHooks` interface 调用
- **修改** CLI 装配层（`cmd/cli/`）适配新的 hook 加载和注入方式

## Capabilities

### New Capabilities
- `shell-hook-engine`: 外部 shell 命令 hook 的执行引擎，包括 Spawner、Runner、配置加载、JSON payload 通信、exit code 决策

### Modified Capabilities
- `tui-chat-interface`: hook 注入方式变更（从 `builtin.NewDefault()` 改为配置加载），不涉及 spec 级行为变更

## Impact

- `internal/hooks/` — 全部重写（Registry → Runner/Spawner/Config）
- `internal/hooks/builtin/` — 全部移除
- `internal/agent/agent.go` — 引入 `ToolHooks` interface，修改 hook 触发方式
- `internal/agent/loop.go` — TodoGuardHook 逻辑内联，hook 触发改为 interface 调用
- `internal/agent/option.go` — `WithHooks` 参数类型变更
- `cmd/cli/once.go`, `cmd/cli/chat_setup.go` — hook 装配逻辑重写
- `internal/agent/agent_test.go`, `internal/hooks/hooks_test.go` — 测试重写
