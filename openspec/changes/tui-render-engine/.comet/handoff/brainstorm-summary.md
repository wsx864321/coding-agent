# Brainstorm Summary

- Change: tui-render-engine
- Date: 2026-06-24

## 确认的技术方案

### 事件系统扩展（扁平扩展）
- 在 `event.Event` 上新增 `ReasoningChunk`、`ToolCallID`、`ToolChunk` 三个字段
- 新增 `ReasoningText`、`ToolProgress` 两个 Kind 常量
- 现有事件类型零破坏，字段向后兼容

### 推理文本渲染（新增 EntryReasoning）
- 新增 `EntryReasoning` transcript entry 类型
- Model 新增 `reasoning` builder、`reasoningLineIdx`、`showReasoning`、`thinkStart`
- 推理期间不累积 pending builder，推理结束后才开始回答渲染
- Ctrl+O 切换折叠/展开，原地重写 transcript 块

### 工具流式输出（固定 20 行截断 + drain loop）
- 保留最后 20 行 + 当前未完成行
- ToolProgress 事件驱动，drain loop 合并高频事件（maxEventDrain=512）
- ToolResult 到达时转为折叠摘要

### Shell 输出折叠（反向扫描 transcript）
- Ctrl+B 从 transcript 末尾反向扫描找最近 Shell 输出块
- `shellOutputs` map 存储完整输出，`shellExpanded` map 记录展开状态
- 1MB 内存上限，仅 bash 工具存储完整输出

### Markdown 渲染（glamour 内置 chroma + lang=diff 检测）
- `glamour.WithChromaStyle()` 启用语法高亮
- 仅当代码块语言标记为 `diff` 时触发 +/- 着色
- Diff 着色在 glamour 渲染后叠加 lipgloss 样式

### 文本选择（wrappedLines 坐标映射）
- selection 结构体基于 wrappedLines 索引
- 鼠标左键拖拽选择，Ctrl+C 复制到剪贴板
- 剪贴板不可用时降级提示

## 关键取舍与风险

| 取舍/风险 | 缓解 |
|----------|------|
| 事件结构体膨胀 | 零值字段无开销 |
| 流式输出高频重渲染 | drain loop + maxEventDrain=512 |
| Shell 输出内存 | 1MB 截断 + 仅 bash 工具 |
| 剪贴板跨平台 | atotto/clipboard 三平台测试 |

## 测试策略

- 单元测试：event 新字段、flushableMarkdownPrefix、toolTail 截断、selection 映射、diff 检测
- 集成测试：ReasoningText→EntryReasoning、ToolProgress→流式块、Ctrl+B/Ctrl+O/Ctrl+C
- 回归测试：全量 TUI 测试套件

## Spec Patch

无
