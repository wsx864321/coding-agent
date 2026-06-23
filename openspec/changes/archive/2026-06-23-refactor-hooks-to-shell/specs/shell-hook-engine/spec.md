## ADDED Requirements

### Requirement: 支持通过 JSON 配置文件声明外部 shell hook
系统 MUST 支持从 JSON 配置文件加载 hook 声明。配置文件路径为全局 `~/.coding-agent/hooks.json` 和项目级 `.coding-agent/hooks.json`。加载顺序为项目级优先，然后全局级。同 scope 内按 JSON 数组顺序执行。

#### Scenario: 加载项目级和全局级 hooks
- **WHEN** agent 启动且项目目录存在 `.coding-agent/hooks.json`，用户主目录存在 `~/.coding-agent/hooks.json`
- **THEN** 系统先加载项目 hooks 再加载全局 hooks，合并为有序列表

#### Scenario: 仅存在全局配置
- **WHEN** 项目目录无 `.coding-agent/hooks.json` 但全局配置存在
- **THEN** 系统仅加载全局 hooks

#### Scenario: 无任何配置文件
- **WHEN** 两个配置路径均不存在
- **THEN** 系统正常运行，无外部 hook 被触发

#### Scenario: 配置文件 JSON 格式错误
- **WHEN** 配置文件存在但 JSON 解析失败
- **THEN** 系统记录警告日志，跳过该文件，继续加载其他配置

### Requirement: 外部 hook 通过 stdin JSON payload 和 exit code 通信
系统 MUST 通过 stdin 向外部命令传入 JSON payload，并根据 exit code 决定行为。payload 包含 `event`、`cwd` 及事件相关字段（`toolName`、`toolArgs`、`toolResult`、`prompt`、`messages`）。

#### Scenario: PreToolUse hook 接收 JSON payload
- **WHEN** PreToolUse 事件触发，匹配到已注册的外部 hook
- **THEN** 系统 spawn 外部命令，通过 stdin 传入 `{"event":"PreToolUse","cwd":"...","toolName":"bash","toolArgs":{...}}` 格式的 JSON

#### Scenario: exit 0 表示 pass
- **WHEN** 外部命令返回 exit code 0
- **THEN** 系统视为 pass，继续执行后续 hook 和操作

#### Scenario: exit 2 在阻塞型事件中表示 block
- **WHEN** PreToolUse 或 UserPromptSubmit 事件中外部命令返回 exit code 2
- **THEN** 系统阻止该操作，将 stderr 或 stdout 作为阻止原因传回 agent

#### Scenario: exit 2 在非阻塞型事件中表示 warn
- **WHEN** PostToolUse 或 Stop 事件中外部命令返回 exit code 2
- **THEN** 系统记录警告日志，不阻止操作

### Requirement: 支持 PreToolUse hook 按工具名正则匹配
系统 MUST 支持 hook 配置中的 `match` 字段，使用正则表达式匹配工具名。仅 PreToolUse 和 PostToolUse 事件支持 `match` 过滤。

#### Scenario: match 字段匹配工具名
- **WHEN** PreToolUse 触发工具 `bash`，hook 配置 `match: "bash|shell"`
- **THEN** 该 hook 被执行

#### Scenario: match 字段不匹配工具名
- **WHEN** PreToolUse 触发工具 `read_file`，hook 配置 `match: "bash"`
- **THEN** 该 hook 被跳过

#### Scenario: 无 match 字段
- **WHEN** hook 配置未设置 `match` 字段
- **THEN** 该 hook 对该事件的所有工具调用均触发

### Requirement: 外部 hook 支持超时控制
系统 MUST 支持 hook 配置中的 `timeout` 字段（毫秒），默认 10000ms。超时时在阻塞型事件中视为 block，非阻塞型事件中视为 warn。

#### Scenario: hook 在超时前完成
- **WHEN** 外部命令在 timeout 内完成
- **THEN** 按 exit code 正常决策

#### Scenario: hook 超时（阻塞型事件）
- **WHEN** PreToolUse hook 超过配置的 timeout 仍未完成
- **THEN** 系统终止进程，视为 block

#### Scenario: hook 超时（非阻塞型事件）
- **WHEN** PostToolUse hook 超过 timeout 仍未完成
- **THEN** 系统终止进程，记录警告日志

### Requirement: 首个 block 结果短路后续 hook 执行
系统 MUST 在阻塞型事件中，首个返回 block 的 hook 立即停止后续 hook 执行。非阻塞型事件中所有 hook 均顺序执行。

#### Scenario: PreToolUse 首个 hook block
- **WHEN** PreToolUse 事件注册了 3 个 hook，第 1 个返回 exit 2
- **THEN** 第 2、3 个 hook 不被执行，工具调用被阻止

#### Scenario: PostToolUse 全部执行
- **WHEN** PostToolUse 事件注册了 3 个 hook，第 1 个返回 exit 2
- **THEN** 第 2、3 个 hook 仍然执行

### Requirement: Stop 事件支持 force 续跑语义
系统 MUST 在 Stop 事件中，当外部命令返回 exit 2 且 stdout 非空时，将 stdout 内容作为 force 消息注入 agent 对话，强制 agent 继续运行。

#### Scenario: Stop hook 返回 force 消息
- **WHEN** Stop 事件外部命令返回 exit 2，stdout 输出 "Please complete remaining tasks"
- **THEN** 系统将 "Please complete remaining tasks" 作为 user 消息注入，agent 继续运行

#### Scenario: Stop hook pass
- **WHEN** Stop 事件所有外部命令返回 exit 0
- **THEN** agent 正常结束本轮

### Requirement: Agent 通过 ToolHooks interface 解耦
Agent 核心循环 MUST 通过 `ToolHooks` interface 调用 hook，不得直接 import hook 实现包。ToolHooks 为 nil 时所有 hook 点位静默跳过。

#### Scenario: ToolHooks 为 nil
- **WHEN** Agent 创建时未注入 ToolHooks
- **THEN** 所有 hook 触发点静默跳过，agent 正常运行

#### Scenario: ToolHooks 注入后触发
- **WHEN** Agent 创建时注入了 ToolHooks 实现
- **THEN** 在 4 个 hook 点位正确触发对应的 interface 方法

### Requirement: Spawner 可注入以支持测试
系统 MUST 支持通过注入自定义 Spawner 函数替代默认的 shell 进程 spawn，以便单元测试无需启动真实子进程。

#### Scenario: 使用自定义 Spawner 进行测试
- **WHEN** 创建 Runner 时传入 mock Spawner
- **THEN** hook 执行时调用 mock Spawner 而非 DefaultSpawner

### Requirement: DefaultSpawner 跨平台兼容
DefaultSpawner MUST 在 Unix 系统上使用 `sh -c`，在 Windows 上使用 `cmd /c` 执行外部命令。

#### Scenario: Unix 平台执行
- **WHEN** 在 Linux/macOS 上 DefaultSpawner 执行命令
- **THEN** 使用 `sh -c "<command>"` 执行

#### Scenario: Windows 平台执行
- **WHEN** 在 Windows 上 DefaultSpawner 执行命令
- **THEN** 使用 `cmd /c "<command>"` 执行
