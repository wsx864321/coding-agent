# Comet Design Handoff

- Change: refactor-hooks-to-shell
- Phase: design
- Mode: compact
- Context hash: 1b161c4d91a1a3298ba191d39ae09ead2f32bf58c9b52528b5a8012ec074e884

Generated-by: comet-handoff.sh

OpenSpec remains the canonical capability spec. This handoff is a deterministic, source-traceable context pack, not an agent-authored summary.

## openspec/changes/refactor-hooks-to-shell/proposal.md

- Source: openspec/changes/refactor-hooks-to-shell/proposal.md
- Lines: 1-31
- SHA256: 8861247d3f0415ad524989775861481bb5ae4e926b91116b07c2a846e93c2195

```md
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
```

## openspec/changes/refactor-hooks-to-shell/design.md

- Source: openspec/changes/refactor-hooks-to-shell/design.md
- Lines: 1-178
- SHA256: 8bf786be3f21303a2a112c32ad875720d63e5e6ffd1b304e542077d2f0292ab3

[TRUNCATED]

```md
## Context

当前 hook 系统（`internal/hooks/`）使用 `Registry` 结构体持有 4 类 Go 函数切片，通过 `builtin.NewDefault()` 在 CLI 启动时硬编码注册 5 个内置 hook。Agent 直接引用 `*hooks.Registry`，hook 逻辑只能用 Go 编写。

DeepSeek-Reasonix 采用完全不同的模式：hook 通过 JSON 配置声明，运行时 spawn 外部 shell 命令，通过 stdin JSON payload + exit code 通信。Agent 通过 `ToolHooks` interface 解耦，不直接 import hook 包。

## Goals / Non-Goals

**Goals:**
- 将 hook 执行模型从进程内 Go 回调改为外部 shell 命令
- 引入 `ToolHooks` interface 解耦 Agent 与 hook 实现
- 支持 JSON 配置文件声明式管理 hook（全局 + 项目级）
- 保留 4 个 hook 点位：UserPromptSubmit、PreToolUse、PostToolUse、Stop
- TodoGuardHook 业务逻辑迁入 agent 核心循环

**Non-Goals:**
- 不新增 Reasonix 的额外事件（PostLLMCall、SessionStart 等）
- 不实现项目信任机制（trust.json）
- 不实现桌面端 hook 管理 UI
- 不实现 hook 热加载（需重启会话）

## Decisions

### D1: ToolHooks interface 定义

**选择**：Agent 通过 interface 调用 hook，不直接依赖 hook 包。

```go
// internal/agent/hooks.go（新文件）
type ToolHooks interface {
    UserPromptSubmit(ctx context.Context, content string) error
    PreToolUse(ctx context.Context, name string, args map[string]any) (block bool, message string)
    PostToolUse(ctx context.Context, name string, args map[string]any, result string)
    Stop(ctx context.Context, messages []provider.Message) (force string, ok bool)
}
```

**理由**：Reasonix 的 `ToolHooks` interface 方案已验证可行，且便于测试和替换实现。

### D2: 配置文件格式与加载

**选择**：两级 JSON 配置，项目级优先于全局级。

```json
// ~/.coding-agent/hooks.json 或 .coding-agent/hooks.json
{
  "hooks": {
    "PreToolUse": [
      {
        "match": "bash",
        "command": "node .coding-agent/hooks/check-bash.js",
        "description": "Block dangerous shell commands",
        "timeout": 5000
      }
    ]
  }
}
```

字段：
- `command`（必填）：shell 命令
- `match`（可选）：工具名正则，仅 PreToolUse/PostToolUse 有效
- `description`（可选）：说明
- `timeout`（可选，毫秒）：默认 10000ms
- `cwd`（可选）：工作目录，默认项目根

加载顺序：项目 hooks → 全局 hooks。同 scope 内按 JSON 数组顺序。

**理由**：与 Reasonix 格式一致，用户可复用已有 hook 脚本。

### D3: Spawner 与执行模型

**选择**：`Spawner` 函数类型 + `DefaultSpawner`（`sh -c` / `cmd /c`）。

```go
type SpawnInput struct {
    Command string
    Cwd     string
    Stdin   string        // JSON payload
    Timeout time.Duration
```

Full source: openspec/changes/refactor-hooks-to-shell/design.md

## openspec/changes/refactor-hooks-to-shell/tasks.md

- Source: openspec/changes/refactor-hooks-to-shell/tasks.md
- Lines: 1-46
- SHA256: 4a7e23ca9ea06351e688750c99a231092253da6862d269f7f00e649f24668135

```md
## 1. 核心类型与接口定义

- [ ] 1.1 创建 `internal/agent/hooks.go`，定义 `ToolHooks` interface（4 个方法：UserPromptSubmit、PreToolUse、PostToolUse、Stop）
- [ ] 1.2 创建 `internal/hooks/hook.go`（重写），定义 Event 枚举、HookConfig、ResolvedHook、Payload、SpawnInput/SpawnResult、Spawner 类型、决策枚举（pass/block/warn）
- [ ] 1.3 实现 `Load()` 函数：从全局和项目级 JSON 配置文件加载 hook 声明，返回 `[]ResolvedHook`
- [ ] 1.4 编写 Load 单元测试：覆盖全局+项目合并、仅全局、无配置、JSON 格式错误场景

## 2. Spawner 与执行引擎

- [ ] 2.1 实现 `DefaultSpawner`：Unix 用 `sh -c`，Windows 用 `cmd /c`，支持 stdin、timeout、stdout/stderr 捕获
- [ ] 2.2 实现 `Run()` 函数：接收 Payload + []ResolvedHook + Spawner，按事件过滤、match 正则匹配、顺序执行、首 block 短路
- [ ] 2.3 实现 `decideOutcome()`：根据 event 类型和 exit code 映射为 pass/block/warn 决策
- [ ] 2.4 编写 Run 单元测试：覆盖 pass/block/warn、match 过滤、短路、超时、Stop force 语义

## 3. Runner 门面

- [ ] 3.1 创建 `internal/hooks/runner.go`，实现 Runner struct（持有 []ResolvedHook + cwd + Spawner）
- [ ] 3.2 Runner 实现 `agent.ToolHooks` interface 的 4 个方法，每个方法构建 Payload 并调用 Run()
- [ ] 3.3 编写 Runner 集成测试：验证 Runner 作为 ToolHooks 的完整调用链

## 4. Agent 集成改造

- [ ] 4.1 修改 `internal/agent/option.go`：`WithHooks` 参数从 `*hooks.Registry` 改为 `ToolHooks` interface
- [ ] 4.2 修改 `internal/agent/agent.go`：UserPromptSubmit 触发点改用 `ToolHooks.UserPromptSubmit()`
- [ ] 4.3 修改 `internal/agent/loop.go`：PreToolUse、PostToolUse 触发点改用 `ToolHooks` interface 调用
- [ ] 4.4 修改 `internal/agent/loop.go`：将 TodoGuardHook 逻辑内联到 Stop 判断处（在外部 hook 之前检查）
- [ ] 4.5 修改 `internal/agent/loop.go`：Stop 触发点改用 `ToolHooks.Stop()`，处理 force 续跑返回值
- [ ] 4.6 修改 `internal/agent/agent.go`：subagent hook 传递改为 ToolHooks interface（PreToolUse + PostToolUse only）

## 5. CLI 装配层适配

- [ ] 5.1 修改 `cmd/cli/once.go`：用 `hooks.Load()` + `hooks.NewRunner()` 替代 `builtin.NewDefault()`
- [ ] 5.2 修改 `cmd/cli/chat_setup.go`：同 5.1 的 hook 装配变更

## 6. 清理旧代码

- [ ] 6.1 移除 `internal/hooks/builtin/` 整个目录（default.go、sink.go、log.go、large_output.go、context_inject.go、todo_guard.go、summary.go）
- [ ] 6.2 移除旧 `internal/hooks/hooks.go` 中的 Registry、Register*、Trigger* 相关代码
- [ ] 6.3 更新 `internal/agent/agent_test.go`：适配 ToolHooks interface，移除 Registry 依赖

## 7. 测试与验证

- [ ] 7.1 编写端到端测试：创建临时 hooks.json + 简单 shell 脚本，验证 PreToolUse block 和 pass 流程
- [ ] 7.2 验证无配置时 agent 正常运行（零 hook 降级）
- [ ] 7.3 验证 TodoGuardHook 逻辑在 agent 主循环中正常工作
- [ ] 7.4 `go build ./...` 编译通过，`go test ./...` 全量测试通过
```

## openspec/changes/refactor-hooks-to-shell/specs/shell-hook-engine/spec.md

- Source: openspec/changes/refactor-hooks-to-shell/specs/shell-hook-engine/spec.md
- Lines: 1-120
- SHA256: 33ec5b3d0645b081c0a3a1b9be0d2cd95d4081835b7e08e9768664825448598e

[TRUNCATED]

```md
## ADDED Requirements

### Requirement: 支持通过 JSON 配置文件声明外部 shell hook
系统 MUST 支持从 JSON 配置文件加载 hook 声明。配置文件路径为全局 `~/.coding-agent/hooks.json` 和项目级 `.coding-agent/hooks.json`。加载顺序为项目级优先，然后全局级。同 scope 内按 JSON 数组顺序执行。

#### Scenario: 加载项目级和全局级 hooks
- **WHEN** agent 启动且项目目录存在 `.coding-agent/hooks.json`，用户主目录存在 `~/.coding-agent/hooks.json`
- **THEN** 系统先加载项目 hooks 再加载全局 hooks，合并为有序列表

#### Scenario: 仅存在全局配置
- **WHEN** 项目目录无 `.coding-agent/hooks.json` 但全局配置存在
- **THEN** 系统仅加载全局 hooks

#### Scenario: 无任何配置文件
- **WHEN** 两个配置路径均不存在
- **THEN** 系统正常运行，无外部 hook 被触发

#### Scenario: 配置文件 JSON 格式错误
- **WHEN** 配置文件存在但 JSON 解析失败
- **THEN** 系统记录警告日志，跳过该文件，继续加载其他配置

### Requirement: 外部 hook 通过 stdin JSON payload 和 exit code 通信
系统 MUST 通过 stdin 向外部命令传入 JSON payload，并根据 exit code 决定行为。payload 包含 `event`、`cwd` 及事件相关字段（`toolName`、`toolArgs`、`toolResult`、`prompt`、`messages`）。

#### Scenario: PreToolUse hook 接收 JSON payload
- **WHEN** PreToolUse 事件触发，匹配到已注册的外部 hook
- **THEN** 系统 spawn 外部命令，通过 stdin 传入 `{"event":"PreToolUse","cwd":"...","toolName":"bash","toolArgs":{...}}` 格式的 JSON

#### Scenario: exit 0 表示 pass
- **WHEN** 外部命令返回 exit code 0
- **THEN** 系统视为 pass，继续执行后续 hook 和操作

#### Scenario: exit 2 在阻塞型事件中表示 block
- **WHEN** PreToolUse 或 UserPromptSubmit 事件中外部命令返回 exit code 2
- **THEN** 系统阻止该操作，将 stderr 或 stdout 作为阻止原因传回 agent

#### Scenario: exit 2 在非阻塞型事件中表示 warn
- **WHEN** PostToolUse 或 Stop 事件中外部命令返回 exit code 2
- **THEN** 系统记录警告日志，不阻止操作

### Requirement: 支持 PreToolUse hook 按工具名正则匹配
系统 MUST 支持 hook 配置中的 `match` 字段，使用正则表达式匹配工具名。仅 PreToolUse 和 PostToolUse 事件支持 `match` 过滤。

#### Scenario: match 字段匹配工具名
- **WHEN** PreToolUse 触发工具 `bash`，hook 配置 `match: "bash|shell"`
- **THEN** 该 hook 被执行

#### Scenario: match 字段不匹配工具名
- **WHEN** PreToolUse 触发工具 `read_file`，hook 配置 `match: "bash"`
- **THEN** 该 hook 被跳过

#### Scenario: 无 match 字段
- **WHEN** hook 配置未设置 `match` 字段
- **THEN** 该 hook 对该事件的所有工具调用均触发

### Requirement: 外部 hook 支持超时控制
系统 MUST 支持 hook 配置中的 `timeout` 字段（毫秒），默认 10000ms。超时时在阻塞型事件中视为 block，非阻塞型事件中视为 warn。

#### Scenario: hook 在超时前完成
- **WHEN** 外部命令在 timeout 内完成
- **THEN** 按 exit code 正常决策

#### Scenario: hook 超时（阻塞型事件）
- **WHEN** PreToolUse hook 超过配置的 timeout 仍未完成
- **THEN** 系统终止进程，视为 block

#### Scenario: hook 超时（非阻塞型事件）
- **WHEN** PostToolUse hook 超过 timeout 仍未完成
- **THEN** 系统终止进程，记录警告日志

### Requirement: 首个 block 结果短路后续 hook 执行
系统 MUST 在阻塞型事件中，首个返回 block 的 hook 立即停止后续 hook 执行。非阻塞型事件中所有 hook 均顺序执行。

#### Scenario: PreToolUse 首个 hook block
- **WHEN** PreToolUse 事件注册了 3 个 hook，第 1 个返回 exit 2
- **THEN** 第 2、3 个 hook 不被执行，工具调用被阻止

#### Scenario: PostToolUse 全部执行
- **WHEN** PostToolUse 事件注册了 3 个 hook，第 1 个返回 exit 2
- **THEN** 第 2、3 个 hook 仍然执行
```

Full source: openspec/changes/refactor-hooks-to-shell/specs/shell-hook-engine/spec.md

