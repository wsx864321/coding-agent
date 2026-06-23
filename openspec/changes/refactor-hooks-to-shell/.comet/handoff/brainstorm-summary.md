# Brainstorm Summary

- Change: refactor-hooks-to-shell
- Date: 2026-06-23

## 确认的技术方案

将 hook 系统从进程内 Go 回调改造为外部 shell 命令模式（参照 DeepSeek-Reasonix）：

1. **ToolHooks interface** — 4 个方法（UserPromptSubmit、PreToolUse、PostToolUse、Stop），定义在 `internal/agent/hooks.go`，Agent 不直接 import hook 包
2. **Runner 门面** — `internal/hooks/runner.go` 实现 `ToolHooks` interface，封装配置加载 + hook 执行
3. **Spawner 机制** — 函数类型注入，DefaultSpawner 在 Unix 用 `sh -c`，Windows 优先检测 Git Bash `sh`、回退 `cmd /c`
4. **JSON 配置** — `~/.coding-agent/hooks.json`（全局）+ `.coding-agent/hooks.json`（项目级），项目级优先
5. **通信协议** — stdin JSON payload + exit code（0=pass, 2=block）
6. **SubsetHooks 包装器** — subagent 只继承 Pre/PostToolUse，UserPromptSubmit/Stop 返回空值
7. **TodoGuard 内联** — 从 hook 体系移入 agent 主循环，优先于外部 Stop hooks 执行
8. **Stop force 语义** — exit 2 + stdout 非空 → stdout 作为 force 消息注入

## 关键取舍与风险

- 每次 hook 有 ~10-50ms spawn 开销，不在热路径上可接受
- 移除内置 hook 是 BREAKING 变更，但无外部用户依赖
- Windows `cmd /c` 回退对复杂脚本兼容性差，优先用 Git Bash 规避
- 不实现信任机制和热加载（后续迭代）

## 测试策略

- Spawner 注入 mock 覆盖所有 exit code 路径（pass/block/warn/timeout）
- 临时 hooks.json + shell 脚本做端到端测试
- TodoGuard 内联用 evidence.Ledger mock 验证
- 零 hook 降级：无配置时 agent 正常运行

## Spec Patch

无
