# Memory 系统设计文档

> coding-agent v1 Memory 子系统 | 已实现 + 未来规划

---

## 已实现功能

### 1. 双层记忆架构

| 层级 | 存储位置 | 生命周期 | 用途 |
|------|---------|---------|------|
| **层级文档** | AGENTS.md / AGENTS.local.md | 跨会话 | 项目规范、用户指南 |
| **自动记忆存储** | `~/.coding-agent/memory/` + `projects/<bucket>/memory/` | 跨会话 | 用户偏好、项目知识、外部引用 |

### 2. 四种记忆类型

| 类型 | 存储位置 | 用途 |
|------|---------|------|
| `user` | 全局目录 | 用户偏好、习惯、专长 |
| `feedback` | 全局目录 | 工作方式指导、经验教训 |
| `project` | 项目目录 | 项目事实、目标、约束、架构决策 |
| `reference` | 项目目录 | 外部资源指针：URL、工单号 |

### 3. 存储格式

**记忆文件** (`<name>.md`)：
```markdown
---
name: prefers-tabs
title: Prefers tabs
description: User prefers tabs over spaces
type: user
---

Always indent with tabs in this project.
**Why:** Consistency with existing codebase conventions.
**How to apply:** Always use tabs when writing or editing files.
```

**索引文件** (`MEMORY.md`)：
```markdown
# 记忆索引

- [Prefers tabs](prefers-tabs.md) — User prefers tabs over spaces
- [Build command](build-command.md) — go build ./cmd/coding-agent
```

### 4. 层级文档发现

按优先级从低到高自动发现：
1. **User 级**: `~/.coding-agent/AGENTS.md`
2. **Ancestor 级**: Git 根目录到项目路径上的 AGENTS.md
3. **Project 级**: 项目根目录 `AGENTS.md`（共享，可提交 git）
4. **Local 级**: 项目根目录 `AGENTS.local.md`（个人，git-ignored）

支持 `@path/to/file.md` 导入语法（递归深度 5 层，循环检测）。

### 5. 工具层

#### `remember` 工具
- 模型调用，保存一条长期记忆
- 需要人工审批（Checker 机制）
- 支持覆盖更新（同名记忆）
- 类型变更时自动迁移旧文件

#### `forget` 工具
- 模型调用，删除一条记忆
- **软删除**：移动到 `.archive/<timestamp>-<name>.md`
- 需要人工审批
- 同时更新 MEMORY.md 索引

#### `recall` 工具
- `search`: BM25 搜索记忆（自带纯 Go 实现）
- `read`: 按名称读取完整记忆内容
- `list`: 列出所有已保存记忆索引
- 支持按类型过滤
- ReadOnly 工具，无需审批

### 6. BM25 检索引擎

纯 Go 实现，无外部依赖：
- 参数：k1=1.2, b=0.75
- 拉丁词小写化 + CJK 字符单字拆分
- 相对分数裁剪（threshold=0.15）
- 摘录生成（MakeSnippet）

### 7. 中会话记忆变更通知

新增记忆不会直接修改 system prompt（保护前缀缓存），而是通过 `memory.Queue` 暂存，在下一次用户输入时前置到 user 消息：

```
<memory-update>
新增记忆：prefers-tabs — User prefers tabs
删除记忆：old-preference
</memory-update>

[用户原始消息...]
```

### 8. 自动记忆提取

每轮对话结束后（LLM 返回 final answer 时）尝试自动提取：
- 节流：至少间隔 5 分钟 + 累计 5 轮
- 使用压缩前消息快照（避免压缩丢失细节）
- LLM side-query 提取（同模型，temperature=0，30s 超时）
- 单次最多提取 3 条新记忆

### 9. Agent 集成

```go
// 自动加载
memSet := memory.Load(memory.Options{CWD: workdir})
a, _ := agent.NewAgent(cfg,
    agent.WithRegistry(registry),
    agent.WithMemory(memSet),
)
```

System prompt 结构：
```
[基础 system prompt]             ← 最稳定（缓存前缀）
[层级文档 memory block]          ← 会话间可能变化
[MEMORY.md 索引]                 ← 会话间可能变化
```

### 10. 目录结构

```
~/.coding-agent/
├── memory/
│   └── global/                      ← 跨项目记忆
│       ├── MEMORY.md
│       └── prefers-tabs.md
├── projects/<bucket>/memory/        ← 项目记忆
│   ├── MEMORY.md
│   ├── build-command.md
│   └── .archive/                    ← 软删除归档
│       └── 20260114-120000-old.md
├── sessions/                         ← 会话持久化（已有）
└── archives/                         ← 压缩归档（已有）
```

---

## 未来规划

### Phase 2: 记忆整理 (Consolidation)

- [ ] 文件数阈值触发（≥15 条）
- [ ] LLM 去重合并 + 淘汰过时记忆
- [ ] 保持记忆库 ≤ 30 条
- [ ] 软删除归档保留审计

### Phase 3: 桌面建议系统

- [ ] 扫描最近 session transcript
- [ ] 关键词匹配 + LLM 确认生成候选
- [ ] 通过 CLI 提示用户确认/拒绝

### Phase 4: 会话历史搜索

- [ ] 对 session JSONL 文件做 BM25 索引
- [ ] 支持 `search`（搜索历史对话）和 `around`（查看上下文）

### Phase 5: 高级特性

- [ ] 全局记忆跨项目自动共享
- [ ] 记忆重要性评分与自动过期
- [ ] LLM side-query 替代 BM25（可选）
- [ ] 记忆版本历史
- [ ] 记忆导出/导入

---

## 设计原则

1. **索引常驻，内容按需**：MEMORY.md 始终在 system prompt 中；完整内容通过 recall 按需加载
2. **软删除可追溯**：forget 归档而非销毁
3. **前缀缓存友好**：中会话变更不修改 system prompt
4. **工具需审批**：remember/forget 需要人工确认，recall 放行
5. **提取有节流**：避免每轮都调 LLM 提取

---

## 文件清单

| 文件 | 说明 |
|------|------|
| `internal/memory/types.go` | 核心类型定义 |
| `internal/memory/memory.go` | 加载与合成 |
| `internal/memory/store.go` | 文件 CRUD + MEMORY.md 索引 |
| `internal/memory/doc.go` | 层级文档发现 + @import |
| `internal/memory/queue.go` | 中会话通知队列 |
| `internal/retrieval/bm25.go` | BM25 检索引擎 |
| `internal/tools/remember.go` | remember 工具 |
| `internal/tools/forget.go` | forget 工具 |
| `internal/tools/recall.go` | recall 工具 |
| `internal/agent/memory_extract.go` | 自动记忆提取 |
| `internal/agent/agent.go` | Agent 集成（新增字段和方法） |
| `internal/agent/option.go` | WithMemory option |
| `internal/agent/compact.go` | 压缩前快照保存 |
| `internal/agent/prompt.go` | 索引注入 system prompt |
| `internal/tools/preset.go` | 记忆工具注册 |
