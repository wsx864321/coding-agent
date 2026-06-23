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
