# Skill 技能系统

## 概述

Skill 系统为 coding-agent 提供可扩展的知识注入能力。技能以 Markdown 文件（`SKILL.md`）的形式存在，通过两级加载策略平衡 token 开销和功能丰富度：

- **第一级（Catalog）**：启动时扫描所有技能，将名称 + 描述注入 system prompt（~100 tokens/skill）
- **第二级（Content）**：LLM 按需调用 `run_skill` 工具加载完整内容（~2000 tokens/skill）

设计参考了 Claude Code 的 Skill 工具、DeepSeek-Reasonix 的 playbook 系统，以及 learn-claude-code 的教学实现。

## 架构

```
┌─────────────────────────────────────────────────────────────┐
│                        Session Boot                         │
│                                                             │
│  skill.NewStore(workdir, homeDir)                           │
│    ├── 扫描 project 级: .coding-agent/skills/               │
│    │                    .agents/skills/                      │
│    │                    .claude/skills/                      │
│    ├── 扫描 global 级:  ~/.coding-agent/skills/             │
│    │                    ~/.agents/skills/                    │
│    │                    ~/.claude/skills/                    │
│    └── 注册 builtin:    skill-creator (Go embed)            │
│                                                             │
│  skill.ApplyIndex(systemPrompt, skills)                     │
│    → system prompt 末尾追加技能目录                          │
│                                                             │
│  registry.Register(run_skill, install_skill)                │
│    → 工具注册到 tool.Registry                               │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│                      Runtime Invocation                      │
│                                                              │
│  路径 A: 用户 /<skill_name> [args]                           │
│    → chat.go handleSlashCommand → 注入 skill body 到 prompt │
│                                                              │
│  路径 B: LLM 调用 run_skill(name, arguments)                 │
│    → inline 模式: 返回 skill body 作为 tool_result            │
│    → subagent 模式: RunSubAgent(skill.Body, task)             │
│                                                              │
│  路径 C: LLM 调用 install_skill(name, content)               │
│    → 写入 .coding-agent/skills/<name>/SKILL.md               │
│    → Store 热更新，立即可通过 run_skill 调用                   │
└─────────────────────────────────────────────────────────────┘
```

## 已实现的功能

### 1. 多层文件系统发现 + 兼容目录

**三层优先级**（高覆盖低）：

| 层级 | 扫描目录 | 说明 |
|------|---------|------|
| Project | `<workdir>/.coding-agent/skills/` | 项目专属技能 |
| Project | `<workdir>/.agents/skills/` | 兼容 Reasonix / Claude Code |
| Project | `<workdir>/.claude/skills/` | 兼容 Claude Code |
| Global | `~/.coding-agent/skills/` | 用户全局技能 |
| Global | `~/.agents/skills/` | 兼容全局 |
| Global | `~/.claude/skills/` | 兼容全局 |
| Builtin | Go embed | 内置技能（skill-creator） |

**文件布局**：
- 目录布局：`<root>/<name>/SKILL.md`（推荐）
- 扁平布局：`<root>/<name>.md`

**优先级规则**：同名 skill 先到先得（project > global > builtin）。

**源码**：`internal/skill/skill.go`

### 2. SKILL.md 格式

```markdown
---
name: my-skill
description: 一句话描述，出现在 system prompt 目录中
runAs: inline
---

# Skill 标题

具体指令内容...
```

**Frontmatter 字段**：

| 字段 | 必填 | 默认值 | 说明 |
|------|------|--------|------|
| `name` | 否 | 目录名/文件名 | 技能唯一标识 |
| `description` | 否 | 空 | 一句话描述 |
| `runAs` | 否 | `inline` | `inline` 或 `subagent` |
| `context` | 否 | — | `fork` 等同于 `runAs: subagent`（CC 兼容） |
| `model` | 否 | — | 预留，模型覆盖（v1 未启用） |

**源码**：`internal/skill/parse.go`

### 3. 双执行模式

- **inline**：skill body 作为 tool_result 注入当前对话，LLM 按指令执行。适合规范类、模板类技能。
- **subagent**：通过 `RunSubAgent` 在隔离 session 中执行。独立 messages 历史，不污染父 agent 上下文。适合探索性、重操作类技能。

**源码**：`internal/skill/tools.go` + `internal/agent/agent.go` (WireSkillTools)

### 4. Slash 命令自动注册

每个已加载的 skill 自动成为 REPL 的 slash 命令：

```
> /skill-creator            # 触发 skill-creator
> /code-review fix bugs     # 触发 code-review，传入参数 "fix bugs"
> /skills                   # 查看所有已加载 skill
```

**内置命令优先**：`/help`、`/reset`、`/exit` 等不会被 skill 覆盖。

**源码**：`cmd/cli/chat.go` (handleSlashCommand)

### 5. install_skill 运行时创建

LLM 可以通过 `install_skill` 工具在运行时创建新技能：

```json
{
  "name": "sql-style",
  "content": "---\nname: sql-style\ndescription: SQL 编码规范\n---\n\n# SQL Style Guide\n..."
}
```

保存到 `<workdir>/.coding-agent/skills/<name>/SKILL.md`，Store 立即热更新。

**源码**：`internal/skill/tools.go` (InstallSkillTool)

### 6. 内置 skill-creator

开箱即用的 meta-skill，帮助用户创建新的 SKILL.md 文件。通过 `/skill-creator` 或 `run_skill("skill-creator")` 触发。

**源码**：`internal/skill/builtins/skill-creator.md` + `internal/skill/builtins.go`

### 7. System Prompt Catalog 注入

启动时将技能目录（名称 + 描述 + 执行模式标记）追加到 system prompt 末尾，上限 4000 字符。LLM 每轮都能看到可用技能，但不消耗大量 token。

**源码**：`internal/skill/index.go`

### 8. Subagent Meta Tools 排除

`run_skill` 和 `install_skill` 已加入 `SubagentMetaTools()` 排除列表，子 agent 不会继承这些工具。

**源码**：`internal/agent/subagent.go`

## 测试覆盖

| 测试文件 | 覆盖内容 |
|---------|---------|
| `internal/skill/parse_test.go` | frontmatter 解析（正常/引号/无 frontmatter/空内容/仅 frontmatter） |
| `internal/skill/skill_test.go` | 名称校验、Store 扫描、Get、Install、优先级覆盖、兼容目录、扁平布局 |
| `internal/skill/index_test.go` | ApplyIndex 空/有技能、Catalog 格式 |
| `internal/skill/tools_test.go` | run_skill inline/not found/empty name/subagent 无 runner/subagent 有 runner、install_skill 成功/空名 |

## 文件结构

```
internal/skill/
├── skill.go          # Skill 结构体、Store、多层发现、Install
├── parse.go          # SKILL.md frontmatter 解析器
├── index.go          # ApplyIndex（catalog 注入 system prompt）+ Catalog（文本格式）
├── tools.go          # run_skill 工具 + install_skill 工具 + SkillRunner 接口
├── builtins.go       # 内置 skill 加载（Go embed）
├── builtins/
│   └── skill-creator.md  # 内置 skill-creator 技能
├── parse_test.go
├── skill_test.go
├── index_test.go
└── tools_test.go
```

## 后续规划

### Phase 2: 工具白名单

通过 `allowed-tools` frontmatter 字段精确控制 subagent skill 可用的工具集：

```markdown
---
name: explore
runAs: subagent
allowed-tools: [read_file, glob_file, bash]
---
```

**实现路径**：
1. `ParseFrontmatter` 支持列表解析（或引入 `gopkg.in/yaml.v3`）
2. `FilterRegistry` 升级：支持白名单模式（不只是排除列表）
3. `WireSkillTools` 中根据 `AllowedTools` 构建 subagent registry

### Phase 3: 更多内置 Skill

| 技能 | 模式 | 工具集 | 用途 |
|------|------|--------|------|
| `explore` | subagent | read_file, glob_file, bash (只读) | 代码搜索、架构分析 |
| `review` | subagent | read_file, glob_file | PR review、质量检查 |
| `test` | subagent | bash, read_file, write_file | 自动生成测试用例 |
| `research` | subagent | bash, read_file | Web + 代码研究 |

### Phase 4: 条件激活

CC 风格的 `paths` frontmatter，按文件路径 glob 条件激活 skill：

```markdown
---
name: react-style
paths: ["src/components/**/*.tsx", "src/hooks/**/*.ts"]
---
```

只有当用户正在操作匹配文件时，该 skill 才出现在 catalog 中。

### Phase 5: Skill 级别 Hooks

允许 skill 定义自己的 hook 配置：

```markdown
---
name: secure-deploy
hooks:
  pre_tool_use:
    bash: require_approval
---
```

### Phase 6: 配置文件支持

通过 `.coding-agent/config.toml` 管理 skill：

```toml
[skills]
paths = ["~/my-team-skills", "../shared/skills"]
disabled_skills = ["review"]
```

### Phase 7: 模型覆盖

`model` frontmatter 字段生效，允许不同 skill 使用不同模型：

```markdown
---
name: review
runAs: subagent
model: gpt-4o
---
```

### Phase 8: Prompt 缓存优化

- 固定 skill catalog 前缀，确保 prompt cache 命中
- skill body 通过 `<skill-pin>` 标签保护，context compaction 时不被压缩
