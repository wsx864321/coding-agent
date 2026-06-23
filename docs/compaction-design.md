# 上下文压缩设计与路线图

## 目标

在长会话里控制上下文体积，避免 `prompt_too_long` 与成本失控，同时尽量保持任务连续性：

- 先做**低成本压缩**（不额外调用 LLM）
- 再做**摘要压缩**（必要时才调用 LLM）
- 最后用**防循环 + 失败兜底**保证可用性

---

## 当前已经做了什么（已落地）

> 以下内容对应当前代码实现，已经接入到主循环。

### 1) M1：低成本压缩层（0 额外 API）

在每轮 LLM 调用前执行：

- `pruneStaleToolResults`
  - 对过旧且较大的 `tool` 输出做占位替换
  - 释放上下文空间，减少历史大输出长期驻留
  - 原始消息可归档到 `archive_dir`

- `snipCompact`
  - 当消息条数超阈值时裁掉中间消息，保留首尾
  - 避免切断 `assistant(tool_calls) -> tool(result)` 消息对

### 2) M2：摘要压缩层（按阈值触发）

- `maybeCompact` 按 `context_window` 与 ratio 阈值判断是否触发摘要压缩
- `compactHistory` 执行实际折叠：
  - 保留 pinned 前缀（system + 小体量关键 user turn + 历史摘要）
  - 保留最近 tail（保证当前任务连续性）
  - 中间区域折叠为一条带标签摘要消息
- `summarize` 复用当前模型进行摘要（同 provider）
- `archiveMessages` 将被折叠区域落盘到 JSONL
  - 默认落在仓库外：`~/.coding-agent/archives`
  - Windows 等价：`%USERPROFILE%\.coding-agent\archives`
  - 自动按项目分桶：`<projectName>-<pathHash>/`
  - 自动清理：14 天 TTL + 每项目 1GB 上限

### 3) M3：稳定性与容错

- 三阈值策略：
  - `soft_compact_ratio`：只提示临近，不压缩
  - `compact_ratio`：正常触发压缩
  - `compact_force_ratio`：高水位强制压缩
- 防循环保护：
  - 连续压缩仍无法降到安全区时，进入 `compactStuck` 暂停自动压缩
- 失败兜底：
  - 摘要失败时使用 `mechanicalFoldDigest`，不会阻塞会话

### 4) 响应式压缩（prompt_too_long 恢复）

- `reactiveCompact`：当 LLM 返回 `prompt_too_long` 错误时自动触发
- 强制执行 `compactHistory("reactive", force=true)`，跳过经济性判断
- 压缩成功后自动重试本轮 LLM 调用

### 5) 记忆联动（压缩前快照）

- `preCompactSnapshot`：在压缩前对即将折叠的消息区域做快照
- `memory_extract`：从快照中提取有价值的信息，写入长期记忆系统
- 降低压缩后的"失忆"影响，关键事实可持久化

### 6) 手动触发能力

- 工具层：`compact` tool（模型可主动触发）
- CLI 层：`/compact [focus]` 可人工触发

---

## 当前配置项（CLI）

以下参数已支持：

- `--context-window`
- `--soft-compact-ratio`
- `--compact-ratio`
- `--compact-force-ratio`
- `--recent-keep`
- `--max-messages-snip`
- `--archive-dir`

推荐起步值（可按模型窗口调优）：

- `context-window`: 按模型能力设置（例如 200k）
- `soft-compact-ratio`: `0.50`
- `compact-ratio`: `0.80`
- `compact-force-ratio`: `0.90`
- `recent-keep`: `3`
- `max-messages-snip`: `80`
- `archive-dir`: 默认自动推导到用户目录，不污染项目工作区

---

## 未来规划

### 阶段 1：可观测性增强（高优先级）

- 增加压缩事件输出（开始 / 完成 / 跳过原因）
- 输出统计：压缩消息数、估算节省 token、归档路径
- 在 chat status 中展示当前上下文占用与下一阈值距离
- soft 阈值触达时给出用户可见提示

### 阶段 2：压缩质量增强（高优先级）

- 引入更稳的折叠经济性判断，避免低价值摘要调用
- 增强摘要 prompt 的结构化要求（目标、决策、改动、待办）
- 压缩后恢复最近高价值上下文（最近读过文件、关键计划、任务约束），避免"压完就失忆"的二次成本

### 阶段 3：跨会话与恢复工具链（中长期）

- 归档检索工具化（按文件、命令、时间窗口检索旧 transcript）
- 支持"从某段历史重新摘要"与"局部恢复到当前会话"
- 做到可追溯、可恢复、可审计

### 阶段 4：策略自适应（远期）

- 基于模型窗口、任务类型、工具输出分布，动态调整阈值
- 在不同任务形态下自动选择更优压缩组合（成本 / 质量 / 连续性）

---

## 设计原则

1. **便宜优先**：先 prune/snip，再摘要
2. **连续性优先**：保留任务前缀与近期尾部，不破坏 tool 对齐
3. **可恢复优先**：折叠前归档，失败可退化，不让会话卡死
4. **渐进演进**：先可用，再可观测，再高质量，再智能化
