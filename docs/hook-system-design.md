# Hook 系统设计

## 目标

把 agent 主循环里的"扩展点"抽出来，让循环只负责"触发"，业务逻辑（日志、权限、注入、收尾）由注册到 Registry 的回调承担。

## 包结构

```
internal/hooks/
  hooks.go              // 核心：Event 枚举 + 4 类 Hook 类型 + Registry
  hooks_test.go         // Registry 行为测试（短路、计数、nil 安全）
  builtin/
    permission.go       // 4 个内置 hook + Sink
    default.go          // Default() 一次性注册入口
```

## 事件

| 事件             | 触发时机                          | 入参                              | 返回值语义                                                                                                |
|------------------|-----------------------------------|-----------------------------------|-----------------------------------------------------------------------------------------------------------|
| UserPromptSubmit | Run 入口追加 user 消息前          | ctx, content                      | 仅通知；error 不阻断主流程                                                                                 |
| PreToolUse       | 工具执行前                        | ctx, call                         | 首条返回非空 `block` 的 hook 阻断本次调用；其余 hook 不再触发                                             |
| PostToolUse      | 工具执行后（含失败）              | ctx, call, output                 | 纯副作用；无返回值                                                                                       |
| Stop             | 拿到 LLM final answer 后，循环退出前 | ctx, messages                  | 首条返回非空 `force` 的 hook 强制续跑：把 `force` 作为 user 消息注入，下一轮 LLM 必须继续                  |

## 主循环中的 4 个 trigger 点

```text
Run(userInput)
  └─ TriggerUserPromptSubmit(userInput)        // 通知型，不阻断
  └─ for turn in [0, MaxTurns):
       └─ loopStep
            ├─ LLM.CreateChatCompletion
            └─ if no tool_calls:
                 └─ TriggerStop(messages)      // 首个非空 force 强制续跑
            └─ else:
                 └─ for tc in tool_calls:
                      └─ executeToolCall
                           └─ invokeTool
                                ├─ TriggerPreToolUse(call)   // 首个非空 block 短路
                                ├─ permission.Checker        // 系统级硬约束
                                ├─ registry.Get + Execute
                                └─ TriggerPostToolUse(call, output)
```

## 安全不变式

**hook 可以"放水"（允许更多操作），但不能"开闸"（绕过系统硬拒绝）。**

`invokeTool` 内的执行顺序保证了这一点：

1. PreToolUse hook 链依次触发；首个返回非空 `block` 的 hook 立即阻断
2. hook 全部放行后，**仍要**走 `permission.Checker`（系统级 deny / ask）
3. system deny 是不可被 hook 覆盖的安全底线

集成测试 `TestRun_PreToolUse_HookAllowsButCheckerDenies` 显式验证了这一点：hook 放行 `echo`，但 system Checker 拒绝 `echo`，最终回填给 LLM 的仍是 `Permission denied`。

## 装配点（cmd/cli）

`once` 和 `chat` 共用 `buildAgent` 构造 Agent + 工具注册表，差异在 hooks 装配：

| 模式 | asker      | system Pipeline                | 触发顺序                                  |
|------|------------|--------------------------------|-------------------------------------------|
| once | nil        | 仅 Deny（无 TTY 故不装 Ask）   | hook（含 Ask 视作 Allow）→ system Deny    |
| chat | StdinAsker | 仅 Deny（Ask 由 PermissionHook 承担，避免重复询问） | hook（Ask + 警告）→ system Deny           |

`PermissionHook` 与 `system Pipeline` 的职责划分：

- **PermissionHook（业务级、用户友好）**：含 Ask 询问、stderr 警告、彩色提示
- **system Pipeline（系统级、不可绕过）**：仅硬拒绝 deny 列表

两层串联形成一个完整防御：hook 给出友好的"是否继续"询问，system deny 兜底"绝对禁止"。

## 当前已注册的内置 hook

通过 `builtin.Default(r, workdir, asker, out)` 一次性注入 5 个 hook（注册顺序即触发顺序）：

1. **PermissionHook**（PreToolUse）— bash 硬拒绝关键字 / 破坏性关键字 / 写入工作区越界
2. **LogHook**（PreToolUse）— 打印每次工具调用摘要，便于 debug / 审计
3. **LargeOutputHook**（PostToolUse）— 输出超过 50KB 时打告警
4. **ContextInjectHook**（UserPromptSubmit）— 打印 workdir 等上下文
5. **SummaryHook**（Stop）— 退出前打印工具调用总次数

## 未来演进路线

> 本节记录的是**生产级目标**，与当前实现保持一定的距离；
> 后续 PR 会按下面的顺序逐步落地。

### 阶段 1：观察 / 配置化（短期）

- [ ] `internal/hooks/config.go`：从 `settings.json`（或环境变量）读取 hook 开关
  - 例：`HOOKS_LOG_ENABLED=false` 可关闭 LogHook 而不影响其他
  - 例：`HOOKS_LARGE_OUTPUT_THRESHOLD=200000` 调大阈值
- [ ] 写日志到文件：把 `Sink.W` 默认从 `os.Stderr` 改成可注入的 `*log.Logger`
- [ ] `/hooks` 命令：增加详细列表（按事件分组、显示 hook 来源 + 描述）

### 阶段 2：用户自定义 hook（中期）

- [ ] **从 `settings.json` 加载外部脚本 hook**：
  - 例：用户在项目根放 `.coding-agent/hooks.json`：
    ```json
    {
      "PreToolUse": [
        {
          "name": "block-secrets",
          "type": "command",
          "command": "grep -E '(AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9]{32})' || exit 0",
          "block-on-exit-code": 1
        }
      ]
    }
    ```
  - hook 引擎读配置 → spawn 子进程 → 通过 stdin 传 JSON → 通过 exit code / stdout 拿阻断信号
- [ ] **错误恢复**：单个 hook panic 不影响后续；引入 `defer recover` 包裹每个触发
- [ ] **超时控制**：每个 hook 调用加 `context.WithTimeout`，避免慢 hook 拖垮主循环

### 阶段 3：扩展事件 / 异步（长期）

- [ ] **新事件**：
  - `PreCompact`（消息历史压缩前）
  - `PostCompact`
  - `SubAgentStart` / `SubAgentStop`（子 Agent 边界）
  - `PermissionRequest`（user 实际看到 Ask 弹窗前 / 后）
- [ ] **异步执行**：PostToolUse 改为 fire-and-forget，避免日志 hook 阻塞主循环
- [ ] **可观测性**：每次 hook 触发记录耗时，导出 Prometheus metric

### 阶段 4：与外部系统的安全集成（远期）

- [ ] **签名 / 校验**：外部脚本 hook 必须带 SHA256 摘要，未在 `settings.json.trustedScripts` 白名单中则拒绝执行
- [ ] **隔离执行**：脚本 hook 跑在 OS 沙箱（macOS Seatbelt / Linux bwrap）里，与 agent 主进程隔离
- [ ] **审计日志**：所有 hook 触发 + 阻断决策落 `~/.coding-agent/audit.log`，方便合规审查

## 设计原则

1. **最小核心 + 可组合**：`internal/hooks` 只提供 Registry + 4 个事件类型，**不**含任何业务规则
2. **分层防御**：hooks（业务级 / 用户友好）+ permission.Checker（系统级 / 不可绕过）→ 任意一层失守另一层兜底
3. **短路语义**：PreToolUse / Stop 首个非空返回即短路；PostToolUse 全部执行
4. **可观察**：每个 hook 的执行结果（阻断 / 放行 / force）都应被测试覆盖，避免静默回归

## 已知局限 / 注意点

- **重复询问的隐患已规避**：chat 模式下 system Pipeline 不装 Ask 列表，避免对同一命令问两次
- **hook error 容忍**：UserPromptSubmit 的 error 当前被静默忽略；生产级应至少打印到 stderr
- **PostToolUse 输出包含 Error**：当 `tool.Execute` 返回 error 时，仍会触发 PostToolUse（让日志 hook 看到"失败"）；hook 需自行判断 `output` 字符串前缀
- **once 模式无 TTY**：PermissionHook 内的 Ask 直接视为 Allow（与 system Pipeline 行为对齐）；如果有强需求，可加 `--interactive` flag 强制走 StdinAsker
