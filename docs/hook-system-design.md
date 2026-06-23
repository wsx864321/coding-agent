# Hook 系统设计

## 目标

通过外部 shell 命令扩展 agent 行为：用户在 JSON 配置文件中声明 hook，agent 运行时 spawn 外部进程执行，通过 stdin JSON payload + exit code 通信。Agent 核心循环通过 `ToolHooks` interface 解耦，不直接依赖 hook 实现包。

## 包结构

```
internal/agent/
  hooks.go              // ToolHooks interface + SubsetHooks（subagent 视图）

internal/hooks/
  hook.go               // Event 枚举、HookConfig、ResolvedHook、Payload、Decision 等核心类型
  context.go            // subagent 上下文标记（WithSubagentFlag / IsSubagent）
  load.go               // 从 JSON 配置文件加载 hook 声明
  run.go                // 执行引擎：匹配 → spawn → exit code 决策
  runner.go             // Runner 门面，实现 agent.ToolHooks interface
  spawner.go            // DefaultSpawner（sh -c / cmd /c）
```

## 事件

| 事件             | 触发时机                          | Payload 字段                      | 返回值语义                                                                                                |
|------------------|-----------------------------------|-----------------------------------|-----------------------------------------------------------------------------------------------------------|
| UserPromptSubmit | Run 入口追加 user 消息前          | `event`, `cwd`, `prompt`          | 非阻塞型；error 不阻断主流程                                                                               |
| PreToolUse       | 工具执行前                        | `event`, `cwd`, `toolName`, `toolArgs` | 阻塞型：exit 2 / 超时 → block，首个 block 短路后续 hook                                                   |
| PostToolUse      | 工具执行后（含失败）              | `event`, `cwd`, `toolName`, `toolArgs`, `toolResult` | 非阻塞型；所有 hook 均顺序执行                                                                            |
| Stop             | LLM final answer 后，循环退出前   | `event`, `cwd`                    | exit 2 + stdout 非空 → force 续跑（stdout 作为 user 消息注入）；首个 force 生效后短路                      |

## 执行模型

### 决策矩阵

| 条件 | 阻塞型事件 (PreToolUse) | 非阻塞型事件 (PostToolUse/Stop) |
|------|------------------------|-------------------------------|
| exit 0 | pass | pass |
| exit 2 | **block** | warn（日志） |
| 其它非零 | warn | warn |
| 超时 | **block** | warn |
| spawn 失败 | **error**（视为 block） | warn（日志） |

### 短路语义

- **PreToolUse**：首个 block 立即停止后续 hook，工具调用被阻止
- **PostToolUse**：全部 hook 顺序执行，不短路
- **Stop**：首个 force（exit 2 + stdout 非空）生效后停止后续 hook

## 配置

### JSON 配置文件

两级配置，项目级先加载，全局级后追加：

| 范围 | 路径 |
|------|------|
| 项目级 | `.coding-agent/hooks.json` |
| 全局级 | `~/.coding-agent/hooks.json` |

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "command": "node .coding-agent/hooks/check-bash.js",
        "match": "bash|shell",
        "description": "Block dangerous shell commands",
        "timeout": 5000
      }
    ]
  }
}
```

字段说明：

| 字段 | 必填 | 说明 |
|------|------|------|
| `command` | 是 | shell 命令 |
| `match` | 否 | 工具名正则，仅 PreToolUse/PostToolUse 有效 |
| `description` | 否 | 说明文本 |
| `timeout` | 否 | 超时毫秒，默认 10000 |
| `cwd` | 否 | 工作目录，默认项目根 |

### stdin JSON Payload

hook 进程通过 stdin 接收 JSON：

```json
{
  "event": "PreToolUse",
  "cwd": "/path/to/project",
  "toolName": "bash",
  "toolArgs": {"command": "rm -rf /"}
}
```

## 主循环中的触发点

```text
Run/RunStream(userInput)
  └─ hooks.UserPromptSubmit(userInput)     // 非阻塞型
  └─ for turn in [0, MaxTurns):
       └─ loopStep
            ├─ maybeCompact
            ├─ LLM Stream
            └─ if no tool_calls:
                 ├─ checkTodoGuard()       // 内置续跑检查（先于外部 hook）
                 └─ hooks.Stop(messages)   // 首个 force 强制续跑
            └─ else:
                 └─ for tc in tool_calls:
                      └─ invokeTool
                           ├─ hooks.PreToolUse(name, args) // 首个 block 短路
                           ├─ permission.Checker           // 系统级硬约束（不可被 hook 绕过）
                           ├─ tool.Execute
                           └─ hooks.PostToolUse(name, args, result)
```

## 安全不变式

**hook 可以阻止操作，但不能绕过系统级安全检查。**

执行顺序保证：
1. PreToolUse hook 先执行，可阻断工具调用
2. hook 全部放行后，**仍要**走 `permission.Checker`（系统级 deny/ask）
3. system deny 是不可被 hook 覆盖的安全底线

## 装配点（cmd/cli）

`once.go` 和 `chat_setup.go` 共用相同的 hook 装配流程：

```go
resolved := hooks.Load(workdir)
runner := hooks.NewRunner(resolved, workdir, hooks.DefaultSpawner)
agent.WithHooks(runner)
```

### Subagent Hook 传递

Subagent 通过 `SubsetHooks` 只继承 `PreToolUse` 和 `PostToolUse`，不继承 `UserPromptSubmit` 和 `Stop`。

## TodoGuard（内置续跑逻辑）

TodoGuard 是 agent 核心行为而非用户扩展逻辑，直接内联在 `loop.go` 的 Stop 判断处（`checkTodoGuard`），先于外部 Stop hook 执行。当检测到未完成的 todo 项时，注入 force 消息强制 agent 继续。

## 设计原则

1. **外部化**：hook 逻辑由外部命令承担，agent 只负责触发和决策
2. **分层防御**：hooks（用户级扩展）+ permission.Checker（系统级硬约束）→ 任意一层失守另一层兜底
3. **短路语义**：PreToolUse 首个 block 即短路；Stop 首个 force 即短路；PostToolUse 全部执行
4. **可注入**：Spawner 函数类型支持测试 mock，无需启动真实子进程
5. **跨平台**：DefaultSpawner 在 Unix 用 `sh -c`，Windows 优先 `sh -c`，无 sh 时回退 `cmd /c`
