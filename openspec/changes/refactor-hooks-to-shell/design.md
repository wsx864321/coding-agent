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
}
type SpawnResult struct {
    ExitCode int
    Stdout   string
    Stderr   string
    TimedOut bool
}
type Spawner func(ctx context.Context, in SpawnInput) SpawnResult
```

决策规则（与 Reasonix 一致）：
| 条件 | 阻塞型事件(PreToolUse/UserPromptSubmit) | 非阻塞型事件(PostToolUse/Stop) |
|------|----------------------------------------|-------------------------------|
| exit 0 | pass | pass |
| exit 2 | **block** | warn（日志） |
| 其它非零 | warn | warn |
| 超时 | **block** | warn |

Stop 事件特殊处理：exit 2 + stdout 非空 → 将 stdout 作为 force 消息注入（等价于当前 TodoGuardHook 的 force 续跑语义）。

**理由**：Spawner 注入支持测试 mock，exit code 语义清晰。

### D4: Runner 门面

**选择**：`Runner` 实现 `ToolHooks` interface，封装配置加载 + hook 执行。

```go
type Runner struct {
    hooks   []ResolvedHook
    cwd     string
    spawner Spawner
}

func NewRunner(hooks []ResolvedHook, cwd string, spawner Spawner) *Runner
```

`Runner` 同时实现 `agent.ToolHooks` interface，CLI 装配层创建 `Runner` 后传入 Agent。

### D5: TodoGuardHook 迁移

**选择**：将 TodoGuardHook 的逻辑直接内联到 `agent.loopStep` 的 Stop 判断处。

```go
// loop.go Stop 处理
if len(msg.ToolCalls) == 0 {
    // 1. 先检查内置续跑条件（原 TodoGuardHook）
    if force := a.checkTodoGuard(ctx); force != "" {
        a.messages = append(a.messages, provider.Message{Role: provider.RoleUser, Content: force})
        return "", nil
    }
    // 2. 再触发外部 Stop hooks
    if a.hooks != nil {
        force, ok := a.hooks.Stop(ctx, a.messages)
        if ok { ... }
    }
    return msg.Content, nil
}
```

**理由**：TodoGuardHook 是 agent 的核心行为而非用户扩展逻辑。内联后不受外部 hook 失败影响。

### D6: 内置 hook 替代方案

| 原内置 hook | 处理方式 |
|-------------|---------|
| LogHook (PreToolUse) | 移除。agent 主循环的 emitter 已覆盖工具调用日志 |
| LargeOutputHook (PostToolUse) | 移除。用户如需可配置外部脚本 |
| ContextInjectHook (UserPromptSubmit) | 移除。已有系统 prompt 覆盖 |
| SummaryHook (Stop) | 移除。TUI/chat 层已有会话统计 |
| TodoGuardHook (Stop) | 迁入 agent 核心循环（D5） |

### D7: JSON Payload 格式

stdin 传给外部命令的 JSON：

```json
{
  "event": "PreToolUse",
  "cwd": "/path/to/project",
  "toolName": "bash",
  "toolArgs": {"command": "rm -rf /"},
  "prompt": "",
  "messages": []
}
```

各事件的 payload 字段：
- **UserPromptSubmit**: `event`, `cwd`, `prompt`
- **PreToolUse**: `event`, `cwd`, `toolName`, `toolArgs`
- **PostToolUse**: `event`, `cwd`, `toolName`, `toolArgs`, `toolResult`
- **Stop**: `event`, `cwd`, `messages`（最近 N 条摘要）

## Risks / Trade-offs

- **[性能]** 每次 hook 触发都 spawn 子进程，有 ~10-50ms 开销 → 可接受，hook 不在热路径上
- **[兼容]** 移除内置 hook 是 **BREAKING** 变更 → 用户如依赖 LogHook 等需迁移为外部脚本
- **[Windows]** `sh -c` 在 Windows 不可用 → DefaultSpawner 检测平台，Windows 用 `cmd /c`
- **[超时]** 外部脚本挂起可阻塞 agent → 默认 10s 超时 + 超时视为 block（阻塞型事件）或 warn（非阻塞型事件）
