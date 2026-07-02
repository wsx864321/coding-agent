# Session 持久化设计

## 目标

让进程退出后再启动能恢复对话历史，解决"关掉终端就丢失"的问题。

---

## 设计原则

1. **极简 v1**：只做 Save / Load / List 三个操作，不加状态机、不加清理策略
2. **与 archive 对称**：session 和 archive 共用分桶策略，目录结构一致
3. **向前兼容**：文件命名对齐后续 GUI 需求，扩展不需要迁移
4. **原子写入**：temp file + rename，崩溃不残留半截文件

---

## 目录结构

```
<UserHome>/.coding-agent/
├── archives/                         # 已有：compact 归档（审计用）
│   └── D-project-coding-agent-a1b2c3d4e5f6/
│       └── 20260614-170500.000.jsonl
│
└── sessions/                         # 新增：session 持久化（可恢复）
    └── D-project-coding-agent-a1b2c3d4e5f6/
        └── 20260614-171500.000-deepseek-v3.jsonl
```

项目分桶使用与 archive 完全相同的 `archiveProjectBucket()`：
- 对工作目录绝对路径做 SHA1，取前 12 位 hex
- 格式：`<路径段编码>-<12位哈希>`
- 路径段编码：完整绝对路径按分隔符切分为段（最多 5 段），去除盘符冒号、替换非法文件名字符，用 `-` 拼接
- 例如：`D:\project\coding-agent` → `D-project-coding-agent-a1b2c3d4e5f6`

---

## 文件格式

`.jsonl`，每行一个 `openai.ChatCompletionMessage` JSON 对象：

```jsonl
{"role":"system","content":"You are a coding agent..."}
{"role":"user","content":"帮我写一个 HTTP 服务"}
{"role":"assistant","content":"好的，我来创建..."}
{"role":"user","content":"\u003ccompaction-summary\u003e\n...\n\u003c/compaction-summary\u003e"}
{"role":"user","content":"加一个日志中间件"}
```

- 与 archive 共用 JSONL 编解码
- compact 摘要原样存储，恢复时 `pinnedPrefixLen` 自动识别
- 使用 `json.Decoder` 流式解析，支持 MiB 级 tool 输出

---

## 命名规则

```
20260614-171500.000-deepseek-v3.jsonl
^^^^^^^^^^^^^^^^^^^ ^^^^^^^^^^^
   UTC 纳秒时间戳      模型名（斜杠替换为短横）
```

- 时间戳天然可排序
- 模型名提供可读信息
- 后续 GUI 列表按文件名排序即可

---

## API

### SaveSession(path, messages)
- 整文件覆盖写入（非增量追加）
- 通过 temp file + rename 保证原子性
- compact 改写了消息中间部分，增量追加需要复杂的 reconciliation

### LoadSession(path)
- 返回完整 `[]openai.ChatCompletionMessage`
- 文件不存在返回 `os.IsNotExist`
- 流式 `json.Decoder` 解析，无行缓冲限制

### ListSessions(dir)
- 返回 `[]SessionInfo`，按更新时间倒序
- `SessionInfo`：Path, Preview（首条消息前 80 字符）, Turns, UpdatedAt, ID
- 自动跳过空的 session（无 user 消息）
- 缺失目录不报错，返回空列表

### SessionBucket(baseDir, workdir)
- 返回 session 根目录下的项目分桶子目录完整路径

---

## Agent 集成

```
NewAgent → SetSessionPath (bind) → Run → auto SaveCurrentSession
                ↑                                    ↓
           --resume 恢复                        每轮 turn 完成后
```

- `SessionDir`：Config 字段，默认 `~/.coding-agent/sessions`
- `SessionPath`：Agent 字段，空表示不持久化
- `SaveCurrentSession()`：Run 每轮完成后自动调用
- `AppendMessage()`：session 恢复时逐条追加消息

---

## CLI 用法

```bash
# 新会话（自动保存到 sessions/<bucket>/<timestamp>-<model>.jsonl）
coding-agent chat

# 列出当前项目所有会话
coding-agent chat --list

# 恢复最近会话
coding-agent chat --resume latest

# 按 ID 前缀恢复（例如 202506）
coding-agent chat --resume 202506

# 自定义 session 目录
coding-agent chat --session-dir /path/to/sessions
```

CLI 参数：

| 参数 | 类型 | 说明 |
|------|------|------|
| `--resume` | string | `latest` 恢复最近，或 ID 前缀匹配 |
| `--list` | bool | 列出当前项目所有会话 |
| `--session-dir` | string | 覆盖默认 session 目录 |

---

## 配置项

| 配置 | 默认值 | 说明 |
|------|--------|------|
| `--session-dir` | `~/.coding-agent/sessions` | session 持久化根目录 |
| `CODING_AGENT_SESSION_DIR` | — | 环境变量覆盖 |

---

## 后续扩展预留

所有 v1 设计选择都为后续 GUI 扩展预留了空间：

| 当前 v1 | 后续 GUI |
|---------|---------|
| `SessionInfo` 4 字段 | 加 `Title`, `Model`, `Workspace` 等 |
| 文件名为 `时间戳-模型.jsonl` | GUI 列表直接解析文件名展示 |
| `ListSessions` 返回切片 | GUI 面板直接消费 |
| `SaveSession` 整文件覆盖 | 加 rewrite version 后改为增量追加 |
| 无清理策略 | 加 `.trash/` 软删除 + TTL 淘汰 |
| 无标题管理 | 加 `.titles.json` sidecar |
