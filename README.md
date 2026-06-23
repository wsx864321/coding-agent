<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/images/logo.svg">
    <img alt="coding-agent" src="docs/images/logo.svg" width="140">
  </picture>
</p>

<h1 align="center">coding-agent</h1>

<p align="center">
  <strong>CLI AI 编码助手 — 连接 OpenAI / Anthropic 兼容 API，在 Agent Loop 中驱动 LLM 操作文件系统</strong>
</p>

<p align="center">
  <a href="#安装"><img src="https://img.shields.io/badge/Go-1.26+-00ADD8?logo=go" alt="Go 1.26+"></a>
  <a href="./LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue" alt="MIT"></a>
</p>

---

## 安装

```bash
go install github.com/wsx864321/coding-agent/cmd@latest
```

或从源码构建：

```bash
git clone https://github.com/wsx864321/coding-agent.git
cd coding-agent
go build -o coding-agent ./cmd
```

### 环境变量（必需）

```bash
# OpenAI 兼容
export OPENAI_API_KEY=sk-xxx

# 如需切换 Base URL（DeepSeek 等）
export OPEN_BASE_URL=https://api.deepseek.com/v1

# 如需使用 Anthropic
export PROVIDER_KIND=anthropic
export ANTHROPIC_API_KEY=sk-ant-xxx
```

项目根目录支持 `.env` 文件自动加载，通过 `--env` 指定路径或 `--env -` 禁用。

---

## 快速开始

### One-Shot 模式

```bash
coding-agent once -m "阅读 main.go 并总结核心逻辑"
coding-agent once -m "重构 utils 包" -q    # --quiet 仅输出最终回答
```

### 交互式 REPL

```bash
coding-agent chat
```

支持 Slash 命令：

| 命令 | 说明 |
|------|------|
| `/help` | 帮助 |
| `/reset` | 清空对话历史 |
| `/compact [focus]` | 手动压缩上下文 |
| `/tools` | 列出所有工具 |
| `/hooks` | 查看 hook 统计 |
| `/skills` | 列出已加载 Skill |
| `/<skill_name>` | 触发指定 Skill |
| `/history` | 查看消息数量 |
| `/jobs` | 查看运行中的后台任务 |
| `/exit`（`/quit`） | 退出 |

### TUI 全屏界面

```bash
coding-agent tui
```

基于 Bubble Tea 的全屏终端界面，支持流式输出、快捷键操作。详见 [TUI 文档](docs/tui.md)。

### 会话恢复

```bash
coding-agent chat --list              # 列出当前项目所有会话
coding-agent chat --resume latest     # 恢复最近会话
coding-agent chat --resume abc123     # 按 ID 前缀恢复
```

---

## 可用工具

| 工具 | 说明 |
|------|------|
| `bash` | 执行 shell 命令，支持 `run_in_background` 后台执行 |
| `bash_output` | 读取后台任务的增量输出 |
| `kill_shell` | 终止后台任务 |
| `wait` | 阻塞等待后台任务完成 |
| `read_file` | 读取文件，支持行范围 |
| `write_file` | 写入/覆盖文件，自动创建目录 |
| `edit_file` | 精确查找替换 |
| `glob_file` | Glob 模式文件发现 |
| `todo_write` | 结构化任务列表管理 |
| `complete_step` | 签署完成步骤（附验证证据） |
| `task` | 派生子代理执行隔离子任务，支持 `run_in_background` |
| `compact` | 模型主动请求上下文压缩 |
| `remember` | 保存事实到长期记忆 |
| `forget` | 删除记忆 |
| `recall` | 搜索/读取/列出记忆 |
| `run_skill` | 触发已加载的 Skill |
| `install_skill` | 安装新 Skill |

> 后台任务工具（`bash_output`、`kill_shell`、`wait`）和记忆工具仅在 `chat` / `tui` 模式下可用。

---

## 配置

### CLI 参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-P, --provider` | `openai` | Provider 类型（`openai` / `anthropic`） |
| `-M, --model` | 按 Provider 自动选择 | 模型名称 |
| `-u, --base-url` | env 自动推导 | API 地址 |
| `-t, --max-turns` | `100` | Agent 循环最大轮数 |
| `-s, --system` | 自动生成 | 自定义 System Prompt |
| `-w, --workdir` | 当前目录 | 文件操作允许的根目录 |
| `-e, --env` | `.env` | 环境变量文件路径，`-` 禁用 |
| `-q, --quiet` | `false` | （`once` 专用）仅输出最终回答 |
| `--context-window` | `0`（关闭） | 上下文窗口 token 数，>0 开启自动压缩 |
| `--soft-compact-ratio` | `0.50` | 软阈值，仅提示不压缩 |
| `--compact-ratio` | `0.80` | 摘要压缩触发阈值 |
| `--compact-force-ratio` | `0.90` | 强制压缩阈值 |
| `--recent-keep` | `3` | 压缩时保留的最近消息下限 |
| `--max-messages-snip` | `80` | 消息裁剪上限，<=0 关闭 |
| `--archive-dir` | `~/.coding-agent/archives` | 压缩归档根目录 |

### 环境变量

| 变量 | 说明 |
|------|------|
| `OPENAI_API_KEY` | OpenAI API Key |
| `OPEN_BASE_URL` | OpenAI 兼容 API 地址 |
| `OPENAI_MODEL` | OpenAI 模型名覆盖 |
| `PROVIDER_KIND` | Provider 类型（`openai` / `anthropic`） |
| `ANTHROPIC_API_KEY` | Anthropic API Key |
| `ANTHROPIC_BASE_URL` | Anthropic API 地址 |
| `ANTHROPIC_MODEL` | Anthropic 模型名覆盖 |
| `CODING_AGENT_MAX_TURNS` | 最大轮数 |
| `CODING_AGENT_CONTEXT_WINDOW` | 上下文窗口大小 |
| `CODING_AGENT_TEMPERATURE` | 温度参数 |
| `CODING_AGENT_MAX_TOKENS` | 最大输出 token 数 |
| `CODING_AGENT_SYSTEM_PROMPT` | 自定义 System Prompt |
| `CODING_AGENT_COMPACT_RATIO` | 压缩阈值 |
| `CODING_AGENT_SOFT_COMPACT_RATIO` | 软压缩阈值 |
| `CODING_AGENT_COMPACT_FORCE_RATIO` | 强制压缩阈值 |
| `CODING_AGENT_RECENT_KEEP` | 保留最近消息数 |
| `CODING_AGENT_MAX_MESSAGES_SNIP` | 消息裁剪上限 |
| `CODING_AGENT_ARCHIVE_DIR` | 压缩归档目录 |
| `CODING_AGENT_SESSION_DIR` | 会话存储目录 |

---

## 核心特性

### 上下文压缩

三层递进策略，仅在 PromptTokens 接近窗口上限时才触发，维持 append-only 高 Cache 命中率。LLM 返回 `prompt_too_long` 时自动响应式压缩恢复。压缩前自动提取关键信息写入长期记忆。[详细设计 →](docs/compaction-design.md)

### 子代理

`task` 工具派生隔离子代理，独立对话上下文和证据账本，共享文件系统和 LLM 客户端。支持后台执行。[详细设计 →](docs/subagent.md)

### 后台任务

`bash` 和 `task` 支持 `run_in_background` 非阻塞执行，通过 `bash_output` / `kill_shell` / `wait` 管理生命周期。[详细设计 →](docs/background-jobs.md)

### 长期记忆

双层记忆：层级化 `AGENTS.md` 文档 + 持久化存储（BM25 检索）。四种类型：`user`、`feedback`、`project`、`reference`。[详细设计 →](docs/memory-design.md)

### Skill 系统

Markdown 驱动的可复用技能。两种模式：`inline`（融入对话）和 `subagent`（隔离执行）。三级发现：项目级 → 全局级 → 内置。[详细设计 →](docs/skill-system.md)

### Hook 系统

外部 shell 命令驱动的扩展机制，通过 `.coding-agent/hooks.json` 声明，stdin JSON payload + exit code 通信。四个事件：`UserPromptSubmit`、`PreToolUse`（可阻断）、`PostToolUse`、`Stop`（可强制续跑）。[详细设计 →](docs/hook-system-design.md)

### 权限管控

串行 Checker 管线，首个 Deny 即短路。内置 `deny-list`（黑名单）、`bash-ask`（交互审批）、`workdir-boundary`（文件系统沙箱）。`chat` 模式交互式询问；`once` 模式仅 deny-list 硬拒绝生效，其余默认放行；`tui` 模式高风险操作默认拒绝。

### 会话持久化

自动保存到 `~/.coding-agent/sessions/<project-bucket>/`，JSONL 格式。支持按 `latest` 或 ID 前缀恢复。[详细设计 →](docs/session-design.md)

### 多 Provider 支持

内置 OpenAI 和 Anthropic 两个 Provider，通过 `-P` 或 `PROVIDER_KIND` 切换。OpenAI 默认模型 `gpt-4o-mini`，Anthropic 默认模型 `claude-sonnet-4-20250514`。

---

## 架构

```
cmd/
  cli/           ← Cobra CLI（root / chat / once / tui）
internal/
  agent/         ← Agent 循环、压缩、子代理、System Prompt
  provider/      ← LLM Provider 抽象层（OpenAI / Anthropic）
  tools/         ← 工具接口 + 全部工具实现
  permission/    ← Allow/Deny 管线 + Asker 接口
  hooks/         ← 外部 Shell Hook 引擎（JSON 配置 + 进程 Spawn）
  jobs/          ← 后台任务管理器（JobManager）
  memory/        ← 长期记忆（Store、Queue、Docs、BM25）
  retrieval/     ← 纯 Go BM25 搜索引擎
  skill/         ← Skill 发现、解析、目录
  tui/           ← Bubble Tea 全屏终端界面
  evidence/      ← todo_write / complete_step 证据账本
docs/            ← 设计文档
```

---

## 许可证

MIT
