## MODIFIED Requirements

### Requirement: 外部 hook 通过 stdin JSON payload 和 exit code 通信
系统 MUST 通过 stdin 向外部命令传入 JSON payload，并根据 exit code 决定行为。payload 包含 `event`、`cwd` 及事件相关字段（`toolName`、`toolArgs`、`toolResult`、`prompt`、`messages`）。Hook 执行的 warn/error/block 结果 MUST 通过注入的 `notify` 回调传递给调用方，而非直接写 stderr 日志。配置加载错误（文件读取失败、JSON 解析失败、正则非法）MUST 静默降级，不写日志。

#### Scenario: PreToolUse hook 接收 JSON payload
- **WHEN** PreToolUse 事件触发，匹配到已注册的外部 hook
- **THEN** 系统 spawn 外部命令，通过 stdin 传入 `{"event":"PreToolUse","cwd":"...","toolName":"bash","toolArgs":{...}}` 格式的 JSON

#### Scenario: exit 0 表示 pass
- **WHEN** 外部命令返回 exit code 0
- **THEN** 系统视为 pass，继续执行后续 hook 和操作

#### Scenario: exit 2 在阻塞型事件中表示 block
- **WHEN** PreToolUse 事件中外部命令返回 exit code 2
- **THEN** 系统阻止该操作，将 stderr 或 stdout 作为阻止原因传回 agent，并通过 notify 回调通知

#### Scenario: exit 2 在非阻塞型事件中表示 warn
- **WHEN** PostToolUse 或 Stop 事件中外部命令返回 exit code 2
- **THEN** 系统通过 notify 回调发送警告消息，不阻止操作

#### Scenario: hook 执行 spawn 失败
- **WHEN** hook 外部命令 spawn 失败
- **THEN** 系统通过 notify 回调发送错误消息，不写 log.Printf

#### Scenario: hook 配置加载失败静默降级
- **WHEN** hooks.json 文件不存在、JSON 解析失败或 match 正则非法
- **THEN** 系统静默跳过该配置，不写 log.Printf，不影响其他 hook 加载

### Requirement: Spawner 可注入以支持测试
系统 MUST 支持通过注入自定义 Spawner 函数替代默认的 shell 进程 spawn，以便单元测试无需启动真实子进程。Runner 构造 MUST 额外接受 `notify func(string)` 参数，nil 时丢弃所有通知。

#### Scenario: 使用自定义 Spawner 进行测试
- **WHEN** 创建 Runner 时传入 mock Spawner
- **THEN** hook 执行时调用 mock Spawner 而非 DefaultSpawner

#### Scenario: notify 为 nil 时静默
- **WHEN** 创建 Runner 时 notify 参数为 nil
- **THEN** 所有 warn/error 消息被丢弃
