## 1. event 包核心定义

- [x] 1.1 创建 `internal/event/event.go`：定义 Kind 枚举（6 种）、Level 枚举、Event struct
- [x] 1.2 创建 `internal/event/sink.go`：定义 Sink 接口、FuncSink 适配器、Discard 实例
- [x] 1.3 创建 `internal/event/textsink.go`：实现 TextSink（Out/Err io.Writer），渲染 6 种事件到 ANSI 终端
- [x] 1.4 编写 event 包单元测试：TextSink 各 Kind 输出格式、FuncSink、Discard

## 2. Agent 层迁移（StreamEmitter → Sink）

- [x] 2.1 修改 `internal/agent/option.go`：新增 `WithSink(event.Sink)` Option，移除 `WithEmitter` 相关
- [x] 2.2 修改 `internal/agent/agent.go`：Agent struct 新增 `sink event.Sink` 字段（nil 时用 Discard）
- [x] 2.3 修改 `internal/agent/loop.go`：所有 emitter.OnXxx 调用替换为 sink.Emit
- [x] 2.4 修改 `internal/agent/subagent.go`：subagent 的 Sink 传递
- [x] 2.5 删除 `internal/agent/emitter.go`：移除 StreamEmitter 接口及 context 注入
- [x] 2.6 更新 `internal/agent/agent_test.go`：适配 Sink，移除 StreamEmitter 依赖

## 3. Hook 层迁移（log.Printf → notify）

- [x] 3.1 修改 `internal/hooks/runner.go`：NewRunner 增加 notify 参数，Runner 持有 notify 字段
- [x] 3.2 修改 `internal/hooks/run.go`：移除 3 处 log.Printf，非 pass outcome 通过 notify 输出
- [x] 3.3 修改 `internal/hooks/load.go`：移除 4 处 log.Printf，错误静默降级
- [x] 3.4 修改 `internal/agent/todo_guard.go`：移除 2 处 log.Printf，改为通过 Agent 的 sink 发 Notice
- [x] 3.5 更新 hook 相关测试：验证 notify 回调被正确调用，验证零 log.Printf

## 4. TUI 迁移（chanEmitter → Sink）

- [x] 4.1 修改 `internal/tui/runner.go`：chanEmitter 改为实现 event.Sink
- [x] 4.2 修改 TUI model Update：根据 Event.Kind 分发渲染，替代原有多 message type
- [x] 4.3 更新 `cmd/cli/tui.go` / `cmd/cli/tui_runner.go`：装配 channel Sink

## 5. CLI 层装配

- [ ] 5.1 修改 `cmd/cli/chat_setup.go`：创建 TextSink 传入 Agent，notify 桥接到 Sink
- [ ] 5.2 修改 `cmd/cli/chat.go`：agent 运行时输出迁移到 TextSink（REPL 启动信息和 slash 命令保持 fmt）
- [ ] 5.3 修改 `cmd/cli/once.go`：创建 TextSink（quiet 模式用精简 Sink），notify 桥接

## 6. 清理与验证

- [ ] 6.1 全量编译 `go build ./...`
- [ ] 6.2 全量测试 `go test ./...`
- [ ] 6.3 验证零 log.Printf：grep 确认 internal/hooks/ 和 internal/agent/ 无 log.Printf 调用
