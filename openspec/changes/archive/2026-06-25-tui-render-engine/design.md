## Context

当前 TUI 渲染引擎基于 Bubble Tea v2 + glamour (chroma) 实现基础的消息流、工具卡片和 Markdown 渲染。事件系统定义了 6 种事件类型（Text、ToolDispatch、ToolResult、ApprovalRequest、TurnDone、Notice），TUI Model 通过 channel 接收事件并驱动渲染。

Reasonix 的 TUI 渲染引擎在此基础上增加了：思考/推理块（reasoning block）、工具实时流式输出（ToolProgress）、Shell 输出折叠/展开（Ctrl+B）、文本选择与剪贴板复制、Diff 视图渲染。这些能力需要在当前架构中增量添加。

## Goals / Non-Goals

**Goals:**
- 实现 LLM 思考/推理文本的折叠展示（默认折叠，Ctrl+O 展开）
- 实现工具执行过程中的实时流式输出（ToolProgress 事件 → 尾部截断渲染）
- 实现 Shell 命令输出的交互式折叠/展开（Ctrl+B 切换）
- 升级 Markdown 渲染：chroma 语法高亮、diff 视图、表格完善
- 实现 viewport 文本的鼠标选择与剪贴板复制
- 实现 Diff 格式文本的 +/- 着色渲染

**Non-Goals:**
- 不涉及状态栏重构（Change B）
- 不涉及输入系统改造（Change C）
- 不涉及覆盖层/模态系统（Change D）
- 不改变现有事件系统的核心架构（仅新增事件类型）
- 不引入新的外部依赖（优先使用已有依赖 glamour/chroma + lipgloss）

## Decisions

### D1: 推理文本使用独立事件类型 `ReasoningText`

**选择**: 新增 `event.ReasoningText` 事件类型，携带增量文本 chunk。

**替代方案**:
- 复用 `event.Text` + flag 区分：语义不清晰，增加判断分支
- 在 TurnDone 中携带完整推理文本：失去实时流式体验

**理由**: 推理文本与回答文本在渲染上完全不同（折叠 vs 直接展示），独立事件类型使 Model.Update 的分发逻辑清晰，且不影响现有 Text 事件的处理。

### D2: 工具流式输出使用 `ToolProgress` 事件

**选择**: 新增 `event.ToolProgress` 事件类型，携带 `ToolCallID string` + `Chunk string`。

**替代方案**:
- 复用 `ToolResult` 多次发送：破坏 ToolResult 的"最终结果"语义
- 在 ToolDispatch 后持续更新同一块：需要额外的状态管理，不如事件驱动清晰

**理由**: Reasonix 使用相同模式。ToolProgress 在 ToolDispatch 和 ToolResult 之间发送，Model 在 transcript 中维护一个"流式输出块"，每次收到 ToolProgress 时原地更新该块（尾部截断 + 行数计数）。

### D3: Shell 折叠使用内存 map 存储完整输出

**选择**: 在 Model 中维护 `shellOutputs map[string]string`（toolCallID → 完整输出）和 `shellExpanded map[string]bool`（展开状态），Ctrl+B 切换时原地重写 transcript 块。

**替代方案**:
- 写入临时文件：增加 I/O 开销，且需要管理文件生命周期
- 使用 transcript entry 的 Raw 字段存储完整输出：Raw 字段当前用于工具卡片参数，语义冲突

**理由**: Shell 输出通常在 KB 级别，内存存储足够。map 结构支持 O(1) 查找和更新。

### D4: Markdown 渲染保持 glamour 但启用 chroma 语法高亮

**选择**: 继续使用 glamour 渲染器，配置 `glamour.WithChromaStyle()` 启用 chroma 语法高亮，同时自定义 diff 视图的渲染逻辑（在 glamour 输出之上叠加 lipgloss 着色）。

**替代方案**:
- 替换为 goldmark 直接渲染：增加新依赖，glamour 已满足需求
- 仅使用 lipgloss 手动渲染：代码块语法高亮需自行实现，工作量大

**理由**: glamour 内置 chroma 支持，只需正确配置即可启用语法高亮。Diff 视图在 glamour 渲染后对 `+`/`-` 行进行二次着色。

### D5: 文本选择使用 Bubble Tea 的 MouseMode + 自定义 selection 状态

**选择**: 在 Model 中维护 `selection` 结构体（startLine, startCol, endLine, endCol, active），MouseMode 设为 CellMotion，在 Update 中处理 MouseMsg（左键按下开始选择、拖动扩展、释放结束）。Ctrl+C 在选中时复制到剪贴板（使用 `github.com/atotto/clipboard`）。

**替代方案**:
- 使用终端原生选择（Shift+鼠标）：依赖终端模拟器支持，不可控
- 使用 viewport 内置选择：Bubble Tea viewport 不支持文本选择

**理由**: Reasonix 使用相同方案。Bubble Tea 的 MouseModeCellMotion 提供逐 cell 的鼠标事件，足以实现精确的文本选择。

### D6: Diff 视图在 Markdown 渲染后叠加着色

**选择**: 在 `renderEntry` 中检测内容是否为 diff 格式（连续 `+`/`-`/`@@` 行），若是则对 `+` 行应用绿色、`-` 行应用红色、`@@` 行应用青色。

**替代方案**:
- 使用专门的 diff 渲染库：增加依赖
- 在 Markdown 渲染前预处理：glamour 可能破坏 diff 格式

**理由**: Diff 格式简单（行首字符决定颜色），在 glamour 渲染后叠加着色是最小侵入方案。

## Risks / Trade-offs

- **[性能] 流式输出高频更新**: ToolProgress 事件可能高频触发（每行或每 N 字节），每次触发需要重写 transcript 块并重新渲染 viewport。→ 使用事件合并（drain loop，已在 Reasonix 中验证），限制每帧最多重渲染一次。
- **[内存] Shell 输出存储**: 长时间运行的命令可能产生大量输出（MB 级）。→ 仅对 `shell-` 前缀的 tool ID 存储完整输出；其他工具仅保留尾部截断。
- **[兼容性] 剪贴板库跨平台**: `atotto/clipboard` 在 Windows/Linux/macOS 上行为可能不同。→ 在三个平台上测试，失败时降级为仅显示"已复制 N 字符"提示。
- **[依赖] chroma 语法高亮**: glamour 已依赖 chroma，但可能需要显式 `import _ "github.com/alecthomas/chroma/v2/formatters"` 触发注册。→ 在 markdown.go 中验证 chroma 初始化。
