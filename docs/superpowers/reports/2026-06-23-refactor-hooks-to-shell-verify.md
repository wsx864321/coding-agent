## Verification Report: refactor-hooks-to-shell

### Summary

| Dimension    | Status           |
|--------------|------------------|
| Completeness | 26/26 tasks ✓, 9 requirements |
| Correctness  | 21/23 scenarios fully match spec (2 accepted deviations) |
| Coherence    | 7/7 design decisions implemented, 2 spec sync gaps |

**Build**: `go build ./...` — exit 0 ✓ (verified 2026-06-23)
**Tests**: `go test ./... -count=1` — 15 packages PASS ✓ (verified 2026-06-23)
**Legacy cleanup**: Go 源码无 `hooks/builtin`、`hooks.Registry`、`Trigger*` 残留引用 ✓

---

### 1. Completeness（完整性）

#### 1.1 tasks.md — PASS

26/26 task 均已勾选 `[x]`，覆盖 7 个阶段：

| 阶段 | Tasks | 状态 |
|------|-------|------|
| 1. 核心类型与接口 | 1.1–1.4 | ✓ |
| 2. Spawner 与执行引擎 | 2.1–2.4 | ✓ |
| 3. Runner 门面 | 3.1–3.3 | ✓ |
| 4. Agent 集成改造 | 4.1–4.6 | ✓ |
| 5. CLI 装配层 | 5.1–5.2 | ✓ |
| 6. 清理旧代码 | 6.1–6.3 | ✓ |
| 7. 测试与验证 | 7.1–7.4 | ✓ |

**产物对照**：

- `internal/agent/hooks.go` — ToolHooks + SubsetHooks ✓
- `internal/hooks/hook.go` — Event/HookConfig/Payload/Spawner 类型 ✓
- `internal/hooks/load.go` — Load() ✓
- `internal/hooks/run.go` — Run() + decideOutcome() ✓
- `internal/hooks/spawner.go` — DefaultSpawner ✓
- `internal/hooks/runner.go` — Runner 实现 ToolHooks ✓
- `internal/hooks/builtin/` — 已删除（`go list` 确认无 Go 文件）✓
- `cmd/cli/once.go`、`chat_setup.go` — Load + NewRunner 装配 ✓

#### 1.2 delta spec 场景覆盖 — 23 scenarios

| Req | 场景 | 实现/测试证据 | 状态 |
|-----|------|--------------|------|
| **R1** JSON 配置加载 | 项目+全局合并 | `Load()` 先 project 后 global；`TestLoad_ProjectAndGlobalMerge` | ✓ |
| | 仅全局 | `TestLoad_GlobalOnly` | ✓ |
| | 无配置 | `TestLoad_NoConfig`、`TestZeroHookDegradation_LoadEmptyRunner` | ✓ |
| | JSON 格式错误 | `TestLoad_InvalidJSON`（log warning + skip） | ✓ |
| **R2** stdin JSON + exit code | PreToolUse payload | `TestRun_PassesPayloadAsStdin`、`TestRunner_PostToolUse_CallsRun` | ✓ |
| | exit 0 → pass | `TestRun_PreToolUse_Pass`、`TestDecideOutcome` | ✓ |
| | exit 2 阻塞型 → block | PreToolUse：`TestRun_PreToolUse_BlockShortCircuit` | ✓ |
| | exit 2 阻塞型 → block | UserPromptSubmit：实现为 **warn/非阻塞** | ⚠ 偏差 |
| | exit 2 非阻塞型 → warn | PostToolUse/Stop：`TestRun_PostToolUse_NonBlockingWarn` | ✓ |
| **R3** match 正则 | 匹配工具名 | `TestRun_PreToolUse_MatchFilter`（反向：不匹配则跳过） | ✓ |
| | 不匹配跳过 | 同上 | ✓ |
| | 无 match 全触发 | `matchesHook` 空 match 返回 true | ✓ |
| **R4** timeout | 超时前正常决策 | `TestRun_PassesTimeoutToSpawner` | ✓ |
| | 阻塞型超时 → block | `TestRun_Timeout_BlockOnPreToolUse` | ✓ |
| | 非阻塞型超时 → warn | `TestRun_Timeout_WarnOnPostToolUse` | ✓ |
| **R5** 首 block 短路 | PreToolUse 首 block 停止后续 | `TestRun_PreToolUse_BlockShortCircuit`（2 hooks，仅执行 1 个） | ✓ |
| | PostToolUse 全部执行 | 代码逻辑正确（`DecisionWarn` 不 break）；**无多 hook 显式测试** | ✓* |
| **R6** Stop force | exit 2 + stdout → force 续跑 | `TestRun_Stop_ForceFromStdout`、`TestRunner_Stop_ForceSemantic`、`TestRun_Stop_ForceFirstWins` | ✓ |
| | 全部 exit 0 正常结束 | `TestRunner_Stop_NoForce`、`TestRun_Stop_ForceRequiresStdout` | ✓ |
| **R7** ToolHooks 解耦 | nil 静默跳过 | agent 多处 `if a.hooks != nil`；`newTestAgent` 默认 nil hooks 正常运行 | ✓ |
| | 注入后 4 点位触发 | `TestRun_UserPromptSubmit_Triggered`、loop hook 测试、TodoGuard/Stop 测试 | ✓ |
| **R8** Spawner 可注入 | mock Spawner 测试 | 全部 `run_test.go`/`runner_test.go` 使用 mock Spawner | ✓ |
| **R9** DefaultSpawner 跨平台 | Unix `sh -c` | `TestShellCommand`（非 Windows 分支） | ✓ |
| | Windows `cmd /c` 或 Git Bash | `TestShellCommand`（Windows 分支）；有 sh 时优先 `sh -c`（Design Doc D3） | ✓ |

\* PostToolUse 多 hook 场景由代码审查确认逻辑正确，建议补测试（见 SUGGESTION）。

---

### 2. Correctness（正确性）

#### R1: JSON 配置加载 — PASS

```20:38:internal/hooks/load.go
func Load(opts LoadOptions) []ResolvedHook {
	var out []ResolvedHook
	if opts.ProjectRoot != "" {
		p := filepath.Join(opts.ProjectRoot, ".coding-agent", "hooks.json")
		out = append(out, loadFile(p, ScopeProject)...)
	}
	// ... global after project
```

- 两级路径、合并顺序、JSON 错误 log+skip、默认 timeout 10000ms、无效 match 跳过并 log — 均符合 spec。

#### R2: stdin JSON + exit code — PASS（含已知偏差）

- Payload 经 `json.Marshal` 写入 Spawner stdin ✓
- exit code 决策表在 `decideOutcome()` 实现 ✓
- **偏差**：`UserPromptSubmit` 不在 `isBlockingEvent()` 中，exit 2/超时均返回 warn 而非 block：

```67:69:internal/hooks/run.go
func isBlockingEvent(e Event) bool {
	return e == EventPreToolUse
}
```

此为 code review 修复 #3 的有意决策（恢复旧 Registry 非阻塞语义），见 `.superpowers/sdd/fix-report.md`。`agent.go` 亦用 `_ = a.hooks.UserPromptSubmit(...)` 忽略 error。

#### R3: match 正则 — PASS

- 仅 Pre/PostToolUse 过滤；Load 时预编译 `compiledMatch`；无效正则 log+skip。

#### R4: timeout — PASS

- `DefaultSpawner` 使用 `context.WithTimeout`；超时设置 `TimedOut=true`，阻塞型 → block。

#### R5: 首 block 短路 — PASS

- 阻塞型事件 `decision == DecisionBlock` 时 `break`；PostToolUse exit 2 为 `DecisionWarn`，不短路。

#### R6: Stop force — PASS

- exit 2 + 非空 stdout → `Report.Force`；首个 force hook 后立即 `break`（review fix #2）。

#### R7: ToolHooks 解耦 — PASS

- interface 定义在 `internal/agent/hooks.go`，agent 不 import hooks 实现
- subagent 通过 `NewSubsetHooks` 仅继承 Pre/PostToolUse
- `runner_iface_test.go` 编译期断言 `*Runner` 实现 `agent.ToolHooks`

#### R8: Spawner 可注入 — PASS

- `NewRunner(..., spawner)` 接受自定义 Spawner；nil 时默认 `DefaultSpawner`

#### R9: DefaultSpawner 跨平台 — PASS

- Unix: `sh -c`；Windows: 检测 PATH 中 `sh`，有则 `sh -c`，否则 `cmd /c`

#### Agent 集成关键点 — PASS

| 触发点 | 位置 | 实现 |
|--------|------|------|
| UserPromptSubmit | `agent.go:250-251` | `a.hooks.UserPromptSubmit(ctx, userInput)` |
| PreToolUse | `loop.go:162-166` | block 时返回 `"Blocked by hook: ..."` |
| PostToolUse | `loop.go:190-198` | 成功/失败均触发 |
| TodoGuard | `loop.go:93-98` | **先于**外部 Stop hook |
| Stop | `loop.go:101-109` | force 注入 user 消息并续跑 |
| Subagent hooks | `agent.go:167-168` | `NewSubsetHooks(a.hooks)` |

TodoGuard 逻辑已从 builtin 迁入 `todo_guard.go`，测试覆盖完整（单元 + `TestLoopStep_TodoGuard_*`）。

---

### 3. Coherence（一致性）

#### 3.1 Design Doc 决策 D1–D7

| 决策 | 描述 | 状态 |
|------|------|------|
| D1 | ToolHooks interface 解耦 | ✓ `internal/agent/hooks.go` |
| D2 | 两级 JSON 配置，项目优先 | ✓ `load.go` |
| D3 | Spawner + exit code 决策表 | ✓ `run.go` + `spawner.go`；UserPromptSubmit 非阻塞为有意偏差 |
| D4 | Runner 门面 | ✓ `runner.go` |
| D5 | TodoGuard 内联到 loop | ✓ `loop.go` + `todo_guard.go` |
| D6 | 移除 builtin hooks | ✓ 目录已删，CLI 改用 Load+NewRunner |
| D7 | JSON Payload 格式 | ✓ 四事件字段正确；Stop **不含 messages**（见下方 WARNING） |

#### 3.2 代码模式一致性 — PASS

- Option 注入模式与现有 `WithRegistry`/`WithChecker` 一致
- 测试使用 stub/mock 与 agent 包现有 `fakeLLM` 模式一致
- 日志前缀 `[hooks]`、`[agent]` 与项目风格一致
- `DecisionError` 用于 spawn 失败（阻塞型），不设置 `Blocked` — 与 fix-report 一致

#### 3.3 delta spec ↔ design doc 一致性 — 2 处需同步

| 议题 | delta spec / openspec design.md | 实现 / technical design | 判定 |
|------|--------------------------------|------------------------|------|
| UserPromptSubmit 阻塞 | R2/D3：exit 2 在 UserPromptSubmit 为 block | 非阻塞 warn；agent 忽略 error | ⚠ 有意偏差，spec 未更新 |
| Stop payload messages | R2/D7：含 `messages` 字段 | `Payload` 无 messages；`Runner.Stop` 忽略 messages 参数 | ⚠ technical design 已省略，delta spec 未同步 |

openspec `design.md` D7 写 Stop 含 messages；`docs/superpowers/specs/2026-06-23-refactor-hooks-to-shell-design.md` §4 Payload 明确 Stop 仅 event+cwd。实现遵循 technical design。

---

### Issues by Priority

#### 1. CRITICAL (Must fix)

*无。* 构建与全量测试通过；核心 hook 引擎、Agent 集成、CLI 装配、TodoGuard 迁移均功能正确。

#### 2. WARNING (Should fix)

1. **UserPromptSubmit 非阻塞与 delta spec 冲突**
   - **Spec**：R2「exit 2 在 PreToolUse **或 UserPromptSubmit** 中表示 block」；D3 决策表将 UserPromptSubmit 列为阻塞型。
   - **实现**：`isBlockingEvent` 仅含 PreToolUse；`TestRunner_UserPromptSubmit_NonBlocking` 断言 exit 2 不返回 error。
   - **影响**：用户配置的 UserPromptSubmit shell hook 无法阻断 prompt 提交。
   - **建议**：在 verify/archive 阶段更新 delta spec 记录「Implementation Divergence」，或恢复阻塞语义并修改 agent.go 处理 error。

2. **Stop payload 缺少 messages 字段**
   - **Spec**：R2 列出 payload 含 `messages`；openspec design.md D7 写 Stop 传 messages。
   - **实现**：`Payload` 无 Messages 字段；`Runner.Stop` 注释 `_ = messages`。
   - **影响**：外部 Stop hook 脚本无法访问对话历史做决策。
   - **建议**：同步 spec 说明有意省略，或实现 messages 摘要序列化。

3. **agent.go 忽略 UserPromptSubmit 返回值**
   - 即使 Runner 将来返回 blocked error，`_ = a.hooks.UserPromptSubmit(...)` 也不会阻断。
   - 与 WARNING #1 同源；若保留非阻塞语义则应在 spec 明确「通知型，不阻断」。

#### 3. SUGGESTION (Nice to fix)

1. **补 PostToolUse 多 hook 测试** — R5「第 1 个 exit 2 后第 2、3 个仍执行」无显式多 hook 测试（代码路径已正确）。
2. **补 nil ToolHooks agent 集成测试** — R7 场景由 e2e 空 Runner 间接覆盖，agent 层无专门 `WithHooks(nil)` 测试。
3. **UserPromptSubmit timeout 场景测试** — R4 对 UserPromptSubmit 超时行为（当前为 warn）无专项测试。
4. **文档同步** — `docs/hook-system-design.md` 仍描述旧 Registry 架构，建议更新或标注 deprecated。

---

### Final Assessment

**总体评估：PASS（含 3 项 WARNING，建议 archive 前同步 spec）**

实现完成了 proposal 的全部目标：Registry/builtin 移除、shell hook 引擎、ToolHooks 解耦、JSON 配置驱动、TodoGuard 内联。26 个 task 均有对应代码与测试。Fresh verification 确认 `go build ./...` 与 `go test ./... -count=1` 全部通过。

两处 spec 偏差（UserPromptSubmit 非阻塞、Stop 无 messages）已在 code review fix-report 中有意接受，但 **delta spec 与 openspec design.md 尚未同步**。建议在 `/comet-archive` 前更新 spec 或明确记录 Implementation Divergence，避免后续读者按 spec 实现 hook 脚本时产生错误预期。

| 指标 | 数量 |
|------|------|
| CRITICAL | 0 |
| WARNING | 3 |
| SUGGESTION | 4 |
