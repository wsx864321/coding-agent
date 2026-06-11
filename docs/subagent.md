# Subagent 子 Agent 系统

## 概述

Subagent 允许父 agent 将聚焦的子任务委派给独立运行的子 agent。子 agent 拥有全新的对话历史、独立的 evidence ledger，但共享文件系统和 LLM client。这种设计使得：

- **上下文隔离**：冗长的探索过程不会污染父 agent 的对话窗口
- **职责分离**：子任务自包含执行，只有最终回答返回给父 agent
- **安全传递**：子 agent 继承父 agent 的 hooks（权限检查、日志）和 permission checker

## 架构

```
┌─────────────────────────────────────────────────┐
│                   Parent Agent                   │
│                                                  │
│  registry: [bash, read_file, ..., task, todo_*]  │
│  hooks: [PreToolUse, PostToolUse, Stop, ...]     │
│  messages: [system, user, assistant, ...]        │
│                                                  │
│  ┌────────────────────┐                          │
│  │    task tool        │ ──── prompt ────┐       │
│  │    (Execute)        │ <── answer ─────┤       │
│  └────────────────────┘                  │       │
│                                          ▼       │
│  ┌─────────────────────────────────────────┐     │
│  │           RunSubAgent()                  │     │
│  │                                          │     │
│  │  子 registry: [bash, read_file, ...]     │     │
│  │  (排除 task / todo_write / complete_step)│     │
│  │                                          │     │
│  │  messages: [sub-system, user, ...]       │     │
│  │  ledger: 独立实例                         │     │
│  │  hooks: 继承父 agent                      │     │
│  │  checker: 继承父 agent                    │     │
│  └─────────────────────────────────────────┘     │
└─────────────────────────────────────────────────┘
```

## 当前实现的能力

### 1. `task` 工具

通用子任务委派工具。LLM 可以通过调用 `task` 工具将子任务委派给子 agent。

**参数**：
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `prompt` | string | 是 | 子 agent 的任务描述，必须自包含 |
| `description` | string | 否 | 简短标签（3-7 词），用于日志 |

**源码**：`internal/tools/task.go`

### 2. `FilterRegistry` 工具过滤

从父 registry 构建子 registry，排除指定的 meta 工具。

```go
child := tools.FilterRegistry(parent, "task", "todo_write", "complete_step")
```

排除的 meta 工具及原因：
- **`task`**：防止递归 spawn（只允许一层嵌套）
- **`todo_write`**：子 agent 的 todo 状态不应影响父 agent
- **`complete_step`**：同上

**源码**：`internal/tools/registry.go`

### 3. `RunSubAgent` 核心运行函数

在全新 session 中运行子 agent，返回最终回答。

```go
answer, err := agent.RunSubAgent(ctx, parentAgent, "请分析所有调用 Foo 的地方", agent.SubagentOptions{
    SystemPrompt: "",      // 空 → 使用默认 subagent prompt
    MaxTurns:     0,       // 0 → 自动计算（parent.MaxTurns / 2, 最少 5）
    Registry:     nil,     // nil → 自动从 parent 过滤
    Hooks:        nil,     // nil → 不使用 hooks
    Checker:      nil,     // nil → 不做权限检查
})
```

**隔离边界**：
| 资源 | 隔离方式 |
|------|----------|
| Messages 历史 | 全新（不继承父对话） |
| Evidence Ledger | 独立实例 |
| OpenAI Client | 共享（同一连接池） |
| 文件系统 | 共享（写操作持久化，工具实例共享 workdir 配置） |
| Hooks | 只继承 PreToolUse/PostToolUse（排除 Stop/UserPromptSubmit） |
| Permission Checker | 可选继承 |

> **为什么排除 Stop/UserPromptSubmit hooks？**
>
> Stop hooks（SummaryHook / TodoGuardHook）是 parent 级别的生命周期事件。
> 如果 subagent 继承了 Stop hooks，SummaryHook 会在 subagent 结束时输出
> "session used N tool calls"，与 parent 后续的工具调用日志交织，造成混淆。
> TodoGuardHook 对 subagent 也无意义（subagent 没有 todo 状态）。
> `hooks.Registry.WithoutStopAndPrompt()` 方法用于构建过滤后的副本。

**源码**：`internal/agent/subagent.go`

### 4. 延迟连线 (`WireTaskTool`)

由于 `TaskTool` 的 runner 闭包需要捕获已构造完成的 Agent，采用两阶段构造：

```go
// 阶段 1: preset 注册 TaskTool（runner=nil）
registry := tools.DefaultRegistry(workdir)

// 阶段 2: NewAgent 构造完成后连线
a, _ := agent.NewAgent(cfg, agent.WithRegistry(registry), ...)
a.WireTaskTool()  // 注入 runner 闭包
```

**源码**：`internal/agent/agent.go` 的 `WireTaskTool` 方法

### 5. System Prompt 引导

当 `task` 工具被注册时，system prompt 自动追加使用指引：

```
使用 task 工具委派子任务给独立的子 agent：
- 子 agent 在独立的 session 中运行（全新对话），只有最终回答返回
- 适合探索性工作（如搜索代码模式、分析调用链）或自包含的修改任务
- prompt 必须自包含——子 agent 看不到你的对话历史
- 不要用 task 做简单的单步操作（如读一个文件），直接调对应工具更快
- 子 agent 不能再派生子 agent（只支持一层嵌套）
```

**源码**：`internal/agent/prompt.go`

## Step Budget 策略

子 agent 的 `MaxTurns` 默认为父 agent 的一半（最少 5 轮），也可通过 `SubagentOptions.MaxTurns` 显式指定。

| 父 MaxTurns | 子 MaxTurns（默认） |
|-------------|-------------------|
| 20 | 10 |
| 10 | 5 |
| 6 | 5（最少 5） |
| 4 | 5（最少 5） |

## 测试覆盖

| 测试文件 | 覆盖内容 |
|----------|----------|
| `internal/tools/registry_test.go` | FilterRegistry 排除、空排除、独立性 |
| `internal/tools/task_test.go` | Name/Schema/Execute 成功/空 prompt/nil runner/runner 错误/SetRunner |
| `internal/agent/subagent_test.go` | 基本成功、空 prompt、自定义 MaxTurns/SystemPrompt/Registry、工具调用、消息隔离、MetaTools、WireTaskTool |

## 未来扩展方向

### Phase 2: 异步并发执行

当前实现是同步阻塞的——父 agent 调用 `task` 后会等待子 agent 完成。未来可扩展为：

- **并行 task**：LLM 在一轮中调用多个 `task` tool call，父 agent 并发执行子 agent
- **实现路径**：在 `loop.go` 的 `executeToolCall` 循环中检测多个 `task` 调用，使用 `sync.WaitGroup` 并发运行
- **关键约束**：文件系统写冲突需要 advisory lock 或分工约定

### Phase 3: 技能化子 Agent (Skill Subagents)

参考 Claude Code 的 `subagent_type` 设计，提供预配置的专业子 agent：

| 技能 | 工具集 | System Prompt | 适用场景 |
|------|--------|---------------|----------|
| `explore` | read_file, glob_file, bash(只读) | 只读探索 | 代码搜索、架构分析 |
| `review` | read_file, glob_file | 代码审查 | PR review、质量检查 |
| `test` | bash, read_file, write_file | 测试编写 | 自动生成测试用例 |
| `refactor` | 全量（不含 meta） | 重构助手 | 大规模代码变更 |

**实现路径**：
1. 扩展 `SubagentOptions`，增加 `Skill` 字段
2. 每个 skill 预定义 system prompt + 工具白名单
3. `task` 工具新增 `skill` 参数（可选），不指定时使用通用模式

### Phase 4: Prompt 缓存优化

当前子 agent 每次创建全新 session，system prompt 无法复用缓存。优化方向：

- **固定前缀**：所有子 agent 共享相同的 system prompt 前缀（工具描述部分），技能差异通过后缀 append
- **Client 复用**：已实现（`sub.client = parent.client`），连接池共享

### Phase 5: 上下文注入

允许父 agent 向子 agent 传递精选的上下文片段：

```go
type SubagentOptions struct {
    // ...existing fields...
    Context []ContextItem  // 注入的上下文片段
}

type ContextItem struct {
    Type    string // "file", "snippet", "summary"
    Content string
}
```

**用途**：父 agent 已读取的关键文件内容可直接传给子 agent，避免重复工具调用。

### Phase 6: 流式输出与进度汇报

- 子 agent 的工具调用过程实时流式输出到父 agent 的 UI（可选）
- 长时间运行的子 agent 定期汇报进度（通过 hook 机制）
