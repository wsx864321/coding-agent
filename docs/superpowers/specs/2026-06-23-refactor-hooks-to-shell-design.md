---
comet_change: refactor-hooks-to-shell
role: technical-design
canonical_spec: openspec
---

# Shell Hook Engine — 技术设计

## 1. 概述

将 coding-agent 的 hook 系统从进程内 Go 函数回调（`hooks.Registry`）改造为外部 shell 命令模式。改造后 hook 通过 JSON 配置文件声明，运行时 spawn 外部命令，通过 stdin JSON payload + exit code 通信。Agent 通过 `ToolHooks` interface 与 hook 实现解耦。

参照项目：DeepSeek-Reasonix (`D:\project\DeepSeek-Reasonix`)。

## 2. ToolHooks Interface

定义在 `internal/agent/hooks.go`（新文件），Agent 包内定义，不 import hook 包。

```go
package agent

import (
    "context"
    "github.com/wsx864321/coding-agent/internal/provider"
)

type ToolHooks interface {
    UserPromptSubmit(ctx context.Context, content string) error
    PreToolUse(ctx context.Context, name string, args map[string]any) (block bool, message string)
    PostToolUse(ctx context.Context, name string, args map[string]any, result string)
    Stop(ctx context.Context, messages []provider.Message) (force string, ok bool)
}
```

Agent struct 的 `hooks` 字段从 `*hooks.Registry` 改为 `ToolHooks`：

```go
type Agent struct {
    // ...
    hooks ToolHooks  // was: *hooks.Registry
    // ...
}
```

`WithHooks` option 改为接受 `ToolHooks`：

```go
type hooksOpt struct{ h ToolHooks }
func WithHooks(h ToolHooks) Option { return hooksOpt{h: h} }
```

### SubsetHooks 包装器

Subagent 只继承 Pre/PostToolUse，通过包装器实现：

```go
type SubsetHooks struct {
    inner ToolHooks
}
func (s *SubsetHooks) UserPromptSubmit(ctx context.Context, content string) error { return nil }
func (s *SubsetHooks) PreToolUse(ctx context.Context, name string, args map[string]any) (bool, string) {
    return s.inner.PreToolUse(ctx, name, args)
}
func (s *SubsetHooks) PostToolUse(ctx context.Context, name string, args map[string]any, result string) {
    s.inner.PostToolUse(ctx, name, args, result)
}
func (s *SubsetHooks) Stop(ctx context.Context, messages []provider.Message) (string, bool) { return "", false }

func NewSubsetHooks(h ToolHooks) ToolHooks { return &SubsetHooks{inner: h} }
```

## 3. 配置文件格式与加载

### 配置路径

| 范围 | 路径 | 加载顺序 |
|------|------|---------|
| 项目级 | `.coding-agent/hooks.json` | 先加载（优先） |
| 全局级 | `~/.coding-agent/hooks.json` | 后加载 |

### JSON 格式

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "match": "bash|shell",
        "command": "node .coding-agent/hooks/check-bash.js",
        "description": "Block dangerous shell commands",
        "timeout": 5000
      }
    ],
    "Stop": [
      { "command": "python3 .coding-agent/hooks/check-todos.py" }
    ]
  }
}
```

### 配置结构（Go）

```go
package hooks

type Event string

const (
    EventPreToolUse        Event = "PreToolUse"
    EventPostToolUse       Event = "PostToolUse"
    EventUserPromptSubmit  Event = "UserPromptSubmit"
    EventStop              Event = "Stop"
)

type HookConfig struct {
    Match       string `json:"match,omitempty"`
    Command     string `json:"command"`
    Description string `json:"description,omitempty"`
    Timeout     int    `json:"timeout,omitempty"`   // ms, default 10000
    Cwd         string `json:"cwd,omitempty"`
}

type Settings struct {
    Hooks map[Event][]HookConfig `json:"hooks"`
}

type Scope string
const (
    ScopeProject Scope = "project"
    ScopeGlobal  Scope = "global"
)

type ResolvedHook struct {
    HookConfig
    Event  Event
    Scope  Scope
    Source string  // settings file absolute path
}
```

### Load 函数

```go
type LoadOptions struct {
    ProjectRoot string
    HomeDir     string  // defaults to os.UserHomeDir()
}

func Load(opts LoadOptions) []ResolvedHook
```

加载逻辑：
1. 项目级 `<ProjectRoot>/.coding-agent/hooks.json` → 解析 → `ScopeProject`
2. 全局级 `<HomeDir>/.coding-agent/hooks.json` → 解析 → `ScopeGlobal`
3. JSON 解析失败时 log warning 并跳过该文件
4. 合并为 `[]ResolvedHook`，项目级在前

## 4. Spawner 与执行引擎

### Spawner 类型

```go
type SpawnInput struct {
    Command string
    Cwd     string
    Stdin   string
    Timeout time.Duration
}

type SpawnResult struct {
    ExitCode  int
    Stdout    string
    Stderr    string
    TimedOut  bool
    Err       error
}

type Spawner func(ctx context.Context, in SpawnInput) SpawnResult
```

### DefaultSpawner

```go
func DefaultSpawner(ctx context.Context, in SpawnInput) SpawnResult
```

平台检测逻辑（Windows）：
1. 检查 `PATH` 中是否有 `sh`（Git Bash）
2. 有 → `sh -c "<command>"`
3. 无 → `cmd /c "<command>"`

Unix 平台固定 `sh -c`。

实现要点：
- `exec.CommandContext` 支持超时
- stdin 通过 `cmd.Stdin = strings.NewReader(payload)` 传入
- stdout/stderr 通过 `bytes.Buffer` 捕获
- 超时时 `ctx.Done()` 触发进程 kill

### Payload 格式

```go
type Payload struct {
    Event      Event           `json:"event"`
    Cwd        string          `json:"cwd"`
    ToolName   string          `json:"toolName,omitempty"`
    ToolArgs   map[string]any  `json:"toolArgs,omitempty"`
    ToolResult string          `json:"toolResult,omitempty"`
    Prompt     string          `json:"prompt,omitempty"`
}
```

各事件 payload 字段：
- **UserPromptSubmit**: event, cwd, prompt
- **PreToolUse**: event, cwd, toolName, toolArgs
- **PostToolUse**: event, cwd, toolName, toolArgs, toolResult
- **Stop**: event, cwd

### Run 函数

```go
type Decision string
const (
    DecisionPass  Decision = "pass"
    DecisionBlock Decision = "block"
    DecisionWarn  Decision = "warn"
    DecisionError Decision = "error"
)

type Outcome struct {
    Hook     ResolvedHook
    Decision Decision
    ExitCode int
    Stdout   string
    Stderr   string
    TimedOut bool
    Duration time.Duration
}

type Report struct {
    Event    Event
    Outcomes []Outcome
    Blocked  bool
    Force    string  // Stop 事件专用
}

func Run(ctx context.Context, payload Payload, hooks []ResolvedHook, spawner Spawner) Report
```

执行流程：
1. 遍历 hooks，过滤 `event` 不匹配的
2. Pre/PostToolUse 额外过滤 `match` 正则不匹配的（空 match = 全匹配）
3. 构建 payload JSON → spawn → 收集结果
4. `decideOutcome`: exit 0 → pass; exit 2 → 阻塞型事件 block / 非阻塞型 warn; 其他非零 → warn; 超时 → 阻塞型 block / 非阻塞型 warn
5. 阻塞型事件首个 block 即停止后续 hook
6. Stop 事件：exit 2 + stdout 非空 → `Report.Force = stdout`

阻塞型事件：`PreToolUse`、`UserPromptSubmit`。

## 5. Runner 门面

```go
type Runner struct {
    hooks   []ResolvedHook
    cwd     string
    spawner Spawner
}

func NewRunner(hooks []ResolvedHook, cwd string, spawner Spawner) *Runner
```

Runner 实现 `agent.ToolHooks` interface：

```go
func (r *Runner) UserPromptSubmit(ctx context.Context, content string) error {
    payload := Payload{Event: EventUserPromptSubmit, Cwd: r.cwd, Prompt: content}
    rep := Run(ctx, payload, r.hooks, r.spawner)
    if rep.Blocked {
        return fmt.Errorf("blocked: %s", rep.Outcomes[len(rep.Outcomes)-1].Stderr)
    }
    return nil
}

func (r *Runner) PreToolUse(ctx context.Context, name string, args map[string]any) (bool, string) {
    payload := Payload{Event: EventPreToolUse, Cwd: r.cwd, ToolName: name, ToolArgs: args}
    rep := Run(ctx, payload, r.hooks, r.spawner)
    if rep.Blocked {
        last := rep.Outcomes[len(rep.Outcomes)-1]
        msg := last.Stderr
        if msg == "" { msg = last.Stdout }
        return true, msg
    }
    return false, ""
}

func (r *Runner) PostToolUse(ctx context.Context, name string, args map[string]any, result string) {
    payload := Payload{Event: EventPostToolUse, Cwd: r.cwd, ToolName: name, ToolArgs: args, ToolResult: result}
    _ = Run(ctx, payload, r.hooks, r.spawner)
}

func (r *Runner) Stop(ctx context.Context, messages []provider.Message) (string, bool) {
    payload := Payload{Event: EventStop, Cwd: r.cwd}
    rep := Run(ctx, payload, r.hooks, r.spawner)
    if rep.Force != "" {
        return rep.Force, true
    }
    return "", false
}
```

## 6. TodoGuard 内联

从 `internal/hooks/builtin/todo_guard.go` 迁入 `internal/agent/loop.go`。

在 `loopStepWithText` 的 Stop 判断处，**先于外部 hooks** 检查：

```go
if len(msg.ToolCalls) == 0 {
    if a.memSet != nil {
        a.maybeExtractMemories(ctx)
    }

    // 1. 内置 TodoGuard（优先于外部 hooks）
    if force := a.checkTodoGuard(ctx); force != "" {
        a.messages = append(a.messages, provider.Message{
            Role: provider.RoleUser, Content: force,
        })
        return "", nil
    }

    // 2. 外部 Stop hooks
    if a.hooks != nil {
        force, ok := a.hooks.Stop(ctx, a.messages)
        if ok {
            a.messages = append(a.messages, provider.Message{
                Role: provider.RoleUser, Content: force,
            })
            return "", nil
        }
    }

    return msg.Content, nil
}
```

`checkTodoGuard` 方法直接复用当前 `TodoGuardHook.Handle` 的逻辑，依赖 `evidence.FromContext(ctx)` 获取 Ledger。

## 7. Agent 触发点改造

4 个触发点全部从 `a.hooks.Trigger*()` 改为 `a.hooks.*()` interface 调用：

| 位置 | 旧调用 | 新调用 |
|------|--------|--------|
| `agent.go` Run/RunStreaming | `a.hooks.TriggerUserPromptSubmit(ctx, input)` | `a.hooks.UserPromptSubmit(ctx, input)` |
| `loop.go` invokeTool | `a.hooks.TriggerPreToolUse(ctx, name, args)` | `a.hooks.PreToolUse(ctx, name, args)` |
| `loop.go` invokeTool | `a.hooks.TriggerPostToolUse(ctx, name, args, out)` | `a.hooks.PostToolUse(ctx, name, args, out)` |
| `loop.go` loopStepWithText | `a.hooks.TriggerStop(ctx, a.messages)` | `a.hooks.Stop(ctx, a.messages)` |

nil 检查保持不变（`if a.hooks != nil`）。

## 8. CLI 装配

```go
// cmd/cli/once.go & chat_setup.go
hookRunner := hooks.NewRunner(
    hooks.Load(hooks.LoadOptions{ProjectRoot: workdir}),
    workdir,
    hooks.DefaultSpawner,
)
// hookRunner 可能无 hooks（[]ResolvedHook 为空），仍然安全注入
opts = append(opts, agent.WithHooks(hookRunner))
```

## 9. 移除清单

| 文件/目录 | 操作 |
|-----------|------|
| `internal/hooks/builtin/` | 整目录删除 |
| `internal/hooks/hooks.go` | 重写为新的类型定义（删除 Registry/Register*/Trigger*） |
| `internal/hooks/hooks_test.go` | 重写为新的测试 |

保留 `internal/hooks/` 包路径，内容完全替换。
