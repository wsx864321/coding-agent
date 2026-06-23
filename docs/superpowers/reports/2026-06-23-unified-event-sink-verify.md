## Verification Report: unified-event-sink

### Summary

| Dimension    | Status           |
|--------------|------------------|
| Completeness | 24/24 tasks ✓, 8 requirements (6 event-sink + 2 shell-hook-engine MODIFIED) |
| Correctness  | 19/22 scenarios fully match spec (3 gaps) |
| Coherence    | 6/7 design decisions fully implemented (D5 partial) |

**Build**: `go build ./...` — exit 0 ✓（2026-06-23 本机 fresh run）
**Tests**: `go test ./... -count=1` — 15 packages PASS ✓（2026-06-23 本机 fresh run）
**Legacy cleanup**: Go 源码零 `StreamEmitter`/`RunStreaming`/`EmitterFromContext`/`WithEmitter`/`EmitterAsker` 引用 ✓
**log.Printf**: `internal/hooks/` 与 `internal/agent/` 零匹配 ✓

---

### 1. Completeness（完整性）

#### 1.1 tasks.md — PASS

24/24 task 均已勾选 `[x]`：

| 阶段 | Tasks | 状态 |
|------|-------|------|
| 1. event 包核心定义 | 1.1–1.4 | ✓ |
| 2. Agent 层迁移 | 2.1–2.6 | ✓ |
| 3. Hook 层迁移 | 3.1–3.5 | ✓ |
| 4. TUI 迁移 | 4.1–4.3 | ✓ |
| 5. CLI 层装配 | 5.1–5.3 | ✓ |
| 6. 清理与验证 | 6.1–6.3 | ✓ |

**产物对照**：

- `internal/event/event.go`、`sink.go`、`textsink.go` — 已创建 ✓
- `internal/agent/emitter.go` — 已删除 ✓
- `internal/agent/option.go` — `WithSink` 已添加 ✓
- `internal/agent/agent.go`、`loop.go`、`todo_guard.go`、`sink_asker.go`、`subagent.go` — 已迁移 ✓
- `internal/hooks/runner.go`、`run.go`、`load.go` — notify 注入 / 静默降级 ✓
- `internal/tui/sink.go`、`runner.go`、`stream.go`、`model.go` — TuiSink + event 分发 ✓
- `cmd/cli/chat_setup.go`、`chat.go`、`once.go`、`tui_runner.go`、`tui.go` — 三种模式装配 ✓

#### 1.2 delta spec (event-sink) — 6 requirements

| Requirement | 实现证据 | 状态 |
|-------------|---------|------|
| 统一事件类型定义 | `internal/event/event.go`：6 Kind + 2 Level + flat Event struct | ✓ |
| Sink 接口 | `internal/event/sink.go`：`Emit`/`FuncSink`/`Discard` | ✓ |
| TextSink 终端输出 | `internal/event/textsink.go` + `textsink_test.go`（6 Kind 覆盖） | ⚠ 见 Correctness |
| Agent 通过 Sink 输出 | `agent.go`/`loop.go`/`option.go`；nil → Discard | ✓ |
| TUI Sink 实现 | `internal/tui/sink.go` + `model.go` event 分发 | ✓ |
| CLI 装配 Sink | `chat_setup.go`/`once.go`/`tui.go` | ✓ |

#### 1.3 delta spec (shell-hook-engine) — 2 MODIFIED requirements

| Requirement | 实现证据 | 状态 |
|-------------|---------|------|
| 外部 hook stdin/exit code + notify | `run.go`/`runner.go`/`load.go`；spawn/marshal/regex → notify；load 静默 | ⚠ exit 2 场景未 notify |
| Spawner 可注入 + notify 参数 | `NewRunner(..., notify)`；nil notify 静默丢弃 | ✓ |

---

### 2. Correctness（正确性）

#### 2.1 Event 类型定义 — PASS

`internal/event/event.go` 与 spec / Design Doc D1 一致：

- 6 种 Kind：`Text`、`ToolDispatch`、`ToolResult`、`ApprovalRequest`、`TurnDone`、`Notice`
- 2 种 Level：`LevelInfo`、`LevelWarn`
- Event struct 含 spec 要求的全部字段；额外扩展 `ApprovalName`/`ApprovalArgs`（Design Doc D1 增补，不冲突）

#### 2.2 Sink 接口 — PASS

- 单方法 `Emit(Event)` ✓
- `FuncSink` 适配器 + `Discard` 丢弃实现 ✓
- `sink_test.go` 覆盖 FuncSink/Discard ✓
- 接口注释要求 goroutine-safe；`TextSink`/`TuiSink` 均加 `sync.Mutex` ✓

#### 2.3 TextSink 渲染 — PARTIAL

| Kind | Spec 要求 | 实现 | 状态 |
|------|----------|------|------|
| Text | 直接写 Out，无换行 | `io.WriteString(s.Out, e.Text)` | ✓ |
| ToolDispatch | 工具名+参数摘要写 Err | 仅 `⚡ %s`（ToolName），无 ToolArgs | ⚠ |
| ToolResult OK | 成功摘要写 Err | `✓ %s` | ✓ |
| ToolResult Err | 错误摘要写 Err | 仅 `✗ %s`（ToolName），无 ToolOutput | ⚠ |
| Notice info/warn | 信息/警告格式写 Err | `·` / `⚠` 前缀 | ✓ |
| ApprovalRequest | — | 无输出（审批走 Asker） | ✓ |
| TurnDone | — | 无输出 | ✓ |

实现与 Design Doc D3（superpowers 版）一致，但与 delta spec event-sink 的 ToolDispatch/ToolResult 场景描述存在偏差。

#### 2.4 Agent 集成 (D4) — PASS

- `WithSink(event.Sink)` Option ✓
- `NewAgent` nil sink → `event.Discard` ✓
- `loop.go`：`onText` → `Text`；`invokeTool` → `ToolDispatch`/`ToolResult` ✓
- `Run()` 为唯一入口，`TurnDone` 在 defer 中 emit ✓
- `RunSubAgent` 不传 WithSink，默认 Discard ✓
- `SinkAsker` 通过 `ApprovalRequest` 事件桥接审批 ✓
- `todo_guard.go` → `Notice`（LevelInfo）✓
- `emitter.go` 已删除，零 StreamEmitter 引用 ✓

#### 2.5 Hook notify (D5) — PARTIAL

| 场景 | Spec 要求 | 实现 | 状态 |
|------|----------|------|------|
| spawn 失败 | notify，无 log.Printf | `run.go:42` notify | ✓ |
| marshal 失败 | notify | `run.go:27` notify | ✓ |
| invalid regex | notify | `matchesHook` notify | ✓ |
| 配置加载失败 | 静默降级 | `load.go` 无 log，skip | ✓ |
| PreToolUse exit 2 block | 阻止 + notify | 阻止 ✓，notify ✗ | ⚠ |
| PostToolUse/Stop exit 2 warn | notify 警告 | DecisionWarn ✓，notify ✗ | ⚠ |
| notify nil 静默 | 空函数 | `NewRunner`/`Run` 均处理 | ✓ |

`run_test.go` 覆盖 spawn 失败与 invalid regex 的 notify；**无 exit 2 warn/block 的 notify 测试**。

#### 2.6 TUI TuiSink (D6) — PASS

- `TuiSink` 持可变 channel + `SetChan` ✓
- `Emit` 非 ApprovalRequest 使用 non-blocking send（channel 满时丢弃） ✓
- `ApprovalRequest` blocking send 保证审批不丢 ✓
- `model.go` 按 `event.Event.Kind` 分发（Text/ToolDispatch/ToolResult/ApprovalRequest/Notice/TurnDone） ✓
- 旧 message type（`StreamChunkMsg` 等）已删除；`stream.go` 仅保留 `streamClosedMsg` ✓
- `Runner` 接口仅 `RunTurn(ctx, prompt) error` ✓
- `tui_runner.go` 调用 `agent.Run()` ✓

#### 2.7 CLI 装配 (D7) — PASS

| 模式 | 要求 | 实现 | 状态 |
|------|------|------|------|
| chat | TextSink(stdout, stderr) + notify 桥接 | `setupAgentWithAsker` | ✓ |
| tui | TuiSink + SinkAsker + notify 桥接 | `setupTuiAgent` | ✓ |
| once | TextSink；quiet 时 Err=Discard | `once.go:59-62` | ✓ |
| chat REPL slash | 保持 fmt.Print* | `chat.go` handleSlashCommand | ✓ |
| runOneTurn | 不重复 print 最终回答 | 仅 `fmt.Println()` 换行分隔 | ✓ |

#### 2.8 遗留机制清理 — PASS

```
grep -r "StreamEmitter|RunStreaming|EmitterFromContext|WithEmitter|EmitterAsker" --include="*.go" .
→ 零匹配

grep -r "log\.Printf" --include="*.go" internal/hooks/ internal/agent/
→ 零匹配
```

---

### 3. Coherence（一致性）

#### 3.1 Design Doc D1–D7 对照

| 决策 | 状态 | 说明 |
|------|------|------|
| D1 Event 类型 | ✓ | 实现匹配，含 ApprovalName/ApprovalArgs 扩展 |
| D2 Sink 接口 | ✓ | |
| D3 TextSink | ✓ | 与 superpowers Design Doc 一致 |
| D4 Agent 集成 | ✓ | Run 单入口、Discard 默认、TurnDone defer |
| D5 Hook notify | ⚠ | D5 迁移表仅列 marshal/spawn/regex；delta spec 额外要求 exit 2 → notify，未实现 |
| D6 TUI | ✓ | TuiSink + event 分发 |
| D7 CLI | ✓ | 三种模式装配正确 |

#### 3.2 delta spec 与 Design Doc 一致性

- `openspec/.../design.md` D3 ToolDispatch 含 `(args)` 格式；superpowers Design Doc D3 与实现均不含 args — **spec 内部小矛盾**，实现跟随 superpowers 版
- shell-hook-engine delta spec 的 exit 2 notify 场景超出 D5 迁移表范围 — **实现跟随 D5 表，未覆盖 delta spec 增量**

#### 3.3 代码 pattern 一致性

- 所有前端均通过 `event.Sink` + `Emit` 统一路径 ✓
- notify 桥接模式一致：`sink.Emit(Notice, LevelWarn, msg)` ✓
- 测试命名从 `*Msg` 迁移到 `event.Event`（如 `approval_test.go` 函数名仍称 `ApprovalRequestMsg`，实际测 `event.Event`）— 轻微命名遗留，无功能影响

---

### Issues by Priority

#### CRITICAL

None

#### WARNING

1. **Hook exit 2 结果未走 notify 回调**（shell-hook-engine MODIFIED requirement）
   - `internal/hooks/run.go` 在 hook 正常 spawn 后 exit 2（warn/block）时仅记录 `Decision`，不调用 `notify`
   - 影响：PostToolUse/Stop 的 exit 2 警告、PreToolUse exit 2 阻断消息在 TUI/chat 中不可见（TUI 模式下 hook warn 仍不可见，与 change 目标相悖）
   - 建议：`decideOutcome` 非 pass 时通过 `FormatOutcome` 或等效逻辑调用 `notify`

2. **TextSink ToolDispatch 未输出 ToolArgs 摘要**（event-sink delta spec scenario）
   - delta spec："格式化为工具名和参数摘要写入 Err"
   - 实现：仅输出 ToolName
   - 影响：chat/once 模式工具进度信息不完整（UX 轻微降级）

3. **TextSink ToolResult 错误未输出 ToolOutput 摘要**（event-sink delta spec scenario）
   - delta spec："格式化为错误摘要写入 Err"
   - 实现：仅输出 `✗ ToolName`，不含错误内容

#### SUGGESTION

1. 补充 `TestRun_NotifyOnExit2Warn` 与 `TestRun_NotifyOnPreToolUseBlock` 测试，锁定 notify 行为
2. 补充 Agent 层 mock Sink 集成测试（`loop_test.go` 当前仅用 `event.Discard`，无事件序列断言）
3. `internal/tui/approval_test.go` 测试函数名 `TestApprovalRequestMsgEntersModal` 可重命名为 `TestApprovalRequestEventEntersModal` 以反映新类型体系
4. superpowers Design Doc 迁移检查清单（§迁移检查清单）仍为 `[ ]` 未勾选，建议同步更新为已完成

---

### Final Assessment

**Verdict: PASS WITH WARNINGS — 可进入 archive，建议先修复 WARNING #1**

unified-event-sink change 的核心架构目标已达成：`internal/event` 包、Sink 统一事件流、Agent/TUI/CLI 三层迁移、StreamEmitter 与 log.Printf 清理均已完成。`go build ./...` 与 `go test ./... -count=1` 全部通过，24/24 task 已勾选，6/6 event-sink requirement 主体实现到位。

主要遗留 gap 是 **hook exit 2 warn/block 场景未调用 notify**（WARNING #1），这与 change 的核心动机（hook 消息在 TUI 可见）直接相关，建议在 archive 前修复。TextSink 工具摘要格式偏差（WARNING #2/#3）为 UX 级别，可后续迭代。
