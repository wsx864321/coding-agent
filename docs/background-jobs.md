# Background Jobs 后台任务系统

## 概述

Background Jobs 让 agent 能以后台方式执行长命令（install/build/test/deploy）和耗时子任务，避免阻塞整个 agent loop。后台任务跨 turn 持续运行，agent 可以继续做其他工作，需要时再读取输出、等待完成或终止任务。

- **非阻塞启动**：`bash(run_in_background=true)` 立即返回 job id，命令在后台持续运行
- **跨 turn 存活**：后台任务的生命周期是整个 chat 会话（session 级），不受单 turn 超时限制
- **增量读取**：`bash_output` 非阻塞读取自上次以来的新输出，支持正则过滤
- **完成通知**：后台任务完成后，下一轮 `Run` 自动注入完成摘要到 user 消息
- **session 隔离**：subagent 启动的 job 归属父 session，session 级操作只看到自己的 job

## 架构

```
┌──────────────────────────────────────────────────────────┐
│                      Parent Agent                         │
│                                                           │
│  jobMgr: *jobs.Manager (session-scoped context)           │
│                                                           │
│  ┌─────────────┐    ┌──────────────┐    ┌─────────────┐  │
│  │   bash       │    │    task       │    │ bash_output │  │
│  │ run_in_bg    │    │ run_in_bg    │    │ kill_shell  │  │
│  │ =true        │    │ =true        │    │ wait        │  │
│  └──────┬───────┘    └──────┬───────┘    └──────┬──────┘  │
│         │                   │                   │         │
│         ▼                   ▼                   │         │
│  ┌──────────────────────────────────┐           │         │
│  │       jobs.Manager               │◄──────────┘         │
│  │  ┌────────────────────────────┐  │                     │
│  │  │ root context (session)     │  │                     │
│  │  │ ┌─────────┐  ┌─────────┐  │  │                     │
│  │  │ │ bash-1  │  │ task-1  │  │  │                     │
│  │  │ │ goroutine│  │ goroutine│  │  │                     │
│  │  │ │ buf+done│  │ result  │  │  │                     │
│  │  │ └─────────┘  └─────────┘  │  │                     │
│  │  └────────────────────────────┘  │                     │
│  │  completed[] → DrainCompletedNote│                     │
│  └──────────────────────────────────┘                     │
│                                                           │
│  Run() 开头:                                               │
│    note = jobMgr.DrainCompletedNote()                     │
│    userInput = note + "\n\n" + userInput                  │
└──────────────────────────────────────────────────────────┘
```

## 当前实现的能力

### 1. `jobs.Manager` — 会话级后台任务注册表

后台任务的核心管理器，持有 session 级 context（生命周期跨越多个 turn）。

**关键设计**：
- **session-scoped context**：`root` context 的生命周期是整个 chat 会话，job 跨 turn 存活，只在 `Close()` 或 `kill_shell` 时取消
- **流式输出缓冲**：每个 job 有独立 `bytes.Buffer`，`bash_output` 用 `readOffset` 做增量读取
- **10MB 缓冲上限**：`jobWriter` 在超过 `maxJobBufferBytes` 后静默丢弃写入，防止失控命令导致 OOM
- **双重发布顺序**：`recordCompletion`（入队 drain note）先于终态 status 翻转，防止 `Wait` 抢跑导致 race
- **session 隔离**：`SessionID` 字段 + `*ForSession` 方法，subagent 的 job 不污染父 agent 视图

**源码**：`internal/jobs/jobs.go`

### 2. `bash(run_in_background=true)` — 后台执行 shell 命令

在 `bash` 工具上新增 `run_in_background` 参数。设为 `true` 时：
- 通过 `jobs.Manager.StartForSession` 启动后台 goroutine
- stdout/stderr 流入 job buffer（不阻塞返回）
- 立即返回 job id + 使用提示
- 不受前台 `timeout` 限制

```json
{
  "command": "npm install",
  "run_in_background": true
}
```

返回：`已启动后台任务 "bash-1"。它跨 turn 持续运行；用 bash_output(job_id="bash-1") 读取输出，wait 等待完成，kill_shell(job_id="bash-1") 终止。`

**源码**：`internal/tools/bash.go` 的 `runBackground` 方法

### 3. `task(run_in_background=true)` — 后台执行子 agent

在 `task` 工具上新增 `run_in_background` 参数。设为 `true` 时：
- 子 agent 在后台 goroutine 中运行
- 最终回答作为 job 的 `result`，通过 `bash_output` 读取
- 适合耗时的探索性子任务

**源码**：`internal/tools/task.go` 的 `runBackground` 方法

### 4. `bash_output` — 增量读取后台输出（非阻塞）

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `job_id` | string | 是 | 后台任务 ID |
| `filter` | string | 否 | 正则表达式，只返回匹配的行 |

返回自上次调用以来的增量输出 + 当前状态（running/done/failed/killed）。task job 不写 buffer，终态后首次调用呈现 `result`。

**源码**：`internal/tools/bgjobs.go`

### 5. `kill_shell` — 终止后台任务

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `job_id` | string | 是 | 要终止的后台任务 ID |

同步翻转为 `Killed` 状态（不等 goroutine 退出），然后 cancel context。已结束或未知的 job 为 no-op。

**源码**：`internal/tools/bgjobs.go`

### 6. `wait` — 阻塞等待后台任务完成

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `job_ids` | string[] | 否 | 要等待的 ID 列表，省略则等所有运行中的 |
| `timeout_seconds` | int | 否 | 最大阻塞秒数，到期返回当前进度 |

阻塞直到目标 job 终态 / ctx 取消 / 超时。返回每个 job 的状态与最终输出。

**源码**：`internal/tools/bgjobs.go`

### 7. 完成通知注入

每次 `Agent.Run()` 开头，调用 `jobMgr.DrainCompletedNote()` 获取自上次以来完成的 job 摘要，注入到 user 消息前缀：

```
后台任务已完成（自上一条消息以来）: bash-1 (npm install) — done。可用 bash_output 读取输出，或 wait 等待其他任务。

<用户实际输入>
```

这让模型在下一轮自然感知到后台任务的完成，无需手动轮询。

**源码**：`internal/agent/agent.go` 的 `Run` 方法

### 8. System Prompt 引导

当后台任务工具被注册时，system prompt 自动追加使用指引：

```
对于长命令（install/build/test/deploy）或耗时子任务，使用后台执行避免阻塞：
- bash 的 run_in_background=true：立即返回 job_id，跨 turn 持续运行，不受 timeout 限制
- task 的 run_in_background=true：子 agent 在后台运行，最终回答通过 bash_output 读取
- 用 bash_output(job_id=...) 增量读取输出（非阻塞，可带 filter 正则过滤行）
- 用 wait(job_ids=[...]) 阻塞等待完成（可设 timeout_seconds）
- 用 kill_shell(job_id=...) 终止运行中的后台任务
- 后台任务完成后，下一轮会自动收到完成通知
- 不要对短命令（< 5 秒）使用后台执行，直接同步跑更快
```

**源码**：`internal/agent/prompt.go`

### 9. CLI 集成

- `chat` 命令启动时创建 `jobs.Manager` 并通过 `WithJobManager` 注入 agent
- `defer jobMgr.Close()` 确保退出时等待所有 job goroutine 退出
- `/jobs` slash 命令查看运行中的后台任务

**源码**：`cmd/cli/chat.go`

## Session 隔离设计

### 为什么保留 session 隔离

虽然当前是单 CLI 进程，但 session 隔离为以下场景提供保障：

1. **subagent + background 组合**：subagent 启动的后台 job 归属父 session，`bash_output`/`kill_shell`/`wait` 不被 subagent 继承（加入 `SubagentMetaTools`），只有主 agent 能操作
2. **未来多 session 扩展**：保留 `SessionID` + `*ForSession` 方法，未来加 web UI 多用户或 session 切换时无需重做

### 实现方式

- `Job.SessionID` 字段标记归属
- `OutputForSession` / `KillForSession` / `WaitForSession` / `RunningForSession` / `DrainCompletedNoteForSession` 做 session 过滤
- 空 session ID（`""`）表示全局/无作用域，保持兼容行为
- `WithSession` / `SessionFromContext` 通过 context 传递 session ID

### 与 Reasonix 的差异

| 特性 | Reasonix | 当前实现 |
|------|----------|---------|
| `SessionID` + `*ForSession` | ✅ | ✅ 保留 |
| `DrainCompletedNoteForSession` | ✅ | ✅ 保留 |
| `SetActiveSession` / `DestroySession` | ✅ | ❌ 未实现（单 session 不需要） |
| `event.Sink` 通知 | ✅ | ❌ 去掉（无状态栏 UI） |
| `destroying` map | ✅ | ❌ 去掉 |

去掉的 `SetActiveSession` / `DestroySession` 是多 session 共用一个 Manager 时的销毁窗口管理，单 CLI 不需要。未来若要多 session，再补回即可。

## Subagent 行为

subagent 的后台任务行为：

| 行为 | subagent 可用 | 说明 |
|------|---------------|------|
| `bash(run_in_background=true)` | ⚠️ 降级 | subagent 无 `jobMgr`，返回"后台执行不可用"错误 |
| `task(run_in_background=true)` | ⚠️ 降级 | 同上 |
| `bash_output` | ❌ 不继承 | 在 `SubagentMetaTools` 中排除 |
| `kill_shell` | ❌ 不继承 | 同上 |
| `wait` | ❌ 不继承 | 同上 |

这是有意设计：subagent 是同步阻塞运行的（`task` 工具等待子 agent 完成），后台执行在 subagent 内没有意义——子 agent 会在后台 job 完成前就返回了。

**源码**：`internal/agent/subagent.go` 的 `SubagentMetaTools`

## 测试覆盖

| 测试文件 | 覆盖内容 |
|----------|----------|
| `internal/jobs/jobs_test.go` | Manager 创建、Start/Done/Failed/Killed、ID 递增、Output 增量读取、task result 呈现、Kill 运行中/已结束/未知、Wait 指定ID/全部/超时/空、Running 快照、DrainCompletedNote、Close 取消、session 隔离（Output/Kill/Wait/Drain/Running）、jobWriter 缓冲上限、context 注入、并发安全 |
| `internal/tools/bgjobs_test.go` | bash_output 成功/filter/无新输出/未知ID/无Manager/空ID/无效filter、kill_shell 运行中/已结束/未知/无Manager/空ID、wait 成功/全部/无job/超时/无Manager、filterLines、session 隔离 |
| `internal/tools/bash_test.go` | run_in_background 成功/无Manager/产生输出/AllowedDirs/可Kill、Schema 含 run_in_background |
| `internal/tools/task_test.go` | run_in_background 成功/无Manager/结果/runner错误、Schema 含 run_in_background |
| `internal/agent/bgjobs_integration_test.go` | 端到端 bash bg + wait、drain note 注入、kill_shell 通过 loop、bash_output 通过 loop、无 Manager 降级、prompt 引导、WithJobManager 注入/nil |

## 使用示例

### 场景 1：后台执行长命令

```
用户: 帮我安装依赖并运行测试

Agent:
  1. 调用 bash(command="npm install", run_in_background=true)
     → 返回 "已启动后台任务 bash-1"
  2. 调用 bash(command="npm test", run_in_background=true)
     → 返回 "已启动后台任务 bash-2"
  3. 调用 wait(job_ids=["bash-1", "bash-2"])
     → 等待两个 job 完成，返回结果
  4. 给出最终回答
```

### 场景 2：增量读取 + 过滤

```
Agent:
  1. 调用 bash(command="go test ./... -v", run_in_background=true)
     → "已启动后台任务 bash-1"
  2. 调用 bash_output(job_id="bash-1", filter="FAIL|PASS")
     → 只返回包含 FAIL 或 PASS 的行
  3. 调用 wait(job_id=["bash-1"])
     → 等待完成
```

### 场景 3：终止失控任务

```
Agent:
  1. 调用 bash(command="npm run build", run_in_background=true)
  2. 发现 build 卡住，调用 kill_shell(job_id="bash-1")
     → "已终止后台任务 bash-1"
```

### 场景 4：跨 turn 通知

```
Turn 1:
  用户: 后台跑一下 npm install
  Agent: 调用 bash(run_in_background=true) → "已启动 bash-1"

（agent 返回，用户输入下一条）

Turn 2:
  用户: 检查安装结果
  Agent 收到的 user 消息:
    "后台任务已完成（自上一条消息以来）: bash-1 (npm install) — done。
     可用 bash_output 读取输出，或 wait 等待其他任务。

     检查安装结果"
  Agent: 调用 bash_output(job_id="bash-1") 读取输出
```

## 未来扩展方向

### Phase 2: 进程树管理

当前 Windows 上 `cmd.exe /C` 的进程树在 cancel 时不一定能完全清理。未来可：
- Unix：设置 `Setpgid` + kill 进程组
- Windows：使用 Job Object 绑定子进程树
- 添加 `cmd.WaitDelay` 防止僵尸进程

### Phase 3: 启发式自动后台

当前完全依赖模型显式设置 `run_in_background=true`。未来可加启发式兜底：
- 关键词匹配（install/build/test/deploy）自动后台
- 命令预估耗时超阈值自动后台
- 参考 `learn-claude-code/s13` 的 `is_slow_operation` 实现

### Phase 4: 多 session 支持

当前是单 session CLI。未来若支持多 session（web UI / session 切换）：
- 实现 `SetActiveSession` / `DestroySession` / `FinishDestroySession`
- 加 `destroying` map 管理销毁窗口
- 接入 `event.Sink` 做状态栏通知

### Phase 5: 后台任务持久化

当前后台任务仅在内存中，进程退出即丢失。未来可：
- 把 job 状态序列化到 session 文件
- 重启后恢复 running 状态的 job（实际进程已死，标记为 killed）
- 跨会话查看历史 job 输出

### Phase 6: 流式进度条

当前后台任务输出只能通过 `bash_output` 主动拉取。未来可：
- 接入 `event.Sink` 做实时流式输出到 CLI
- 长命令的进度条展示（如 `npm install` 的百分比）
- 后台任务状态栏（`/jobs` 的实时版）

### Phase 7: 后台任务优先级与并发限制

当前无并发上限，模型可启动任意数量后台 job。未来可：
- 限制最大并发后台 job 数
- 优先级队列（重要任务先执行）
- 资源占用感知（CPU/内存水位高时拒绝新 job）
