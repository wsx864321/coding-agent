## Why

当前 `coding-agent` 以传统 CLI 交互为主，面对多轮会话时的信息密度和操作效率有限。引入基于 Bubble Tea 的 TUI 模式可以提升可读性、输入反馈和交互效率，并在保持现有命令兼容的前提下提供更友好的终端体验。

## What Changes

- 新增一个并行的 TUI 入口命令（例如 `coding-agent tui`），不替换现有 `chat` 和 `once`。
- 在首个迭代中实现聊天主界面能力：消息流展示、输入框、基础快捷键、退出控制与错误可视化。
- 复用现有会话与 agent loop 能力，使 TUI 仅作为交互层而非业务重写。
- 明确跨平台终端兼容目标，优先保证 Windows/macOS/Linux 的基础一致行为。

## Capabilities

### New Capabilities

- `tui-chat-interface`: 提供基于 Bubble Tea 的聊天型终端界面，包括消息渲染、输入、快捷键与生命周期控制。

### Modified Capabilities

- 无。

## Impact

- 受影响代码：`cmd/cli` 命令注册与启动逻辑、交互会话相关模块、终端输出适配层。
- 新增依赖：`github.com/charmbracelet/bubbletea`（及必要的 TUI 生态依赖）。
- 运行影响：新增 TUI 运行路径，但不改变现有 CLI 命令默认行为。
