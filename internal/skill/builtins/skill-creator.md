---
name: skill-creator
description: 帮助创建新的 SKILL.md 技能文件。当用户想要创建、编写或保存一个新的 skill 时使用。
runAs: inline
---

# Skill Creator

你正在帮助用户创建一个新的 coding-agent skill。请按以下步骤操作：

## 1. 收集信息

向用户了解：
- **技能名称**：只能包含字母、数字、连字符 `-`、下划线 `_`，以字母开头
- **技能描述**：一句话说明这个 skill 做什么、什么时候使用
- **执行模式**：`inline`（内容注入当前对话）或 `subagent`（在独立 session 中执行）
- **技能内容**：具体的指令、规范、模板等

## 2. 编写 SKILL.md

生成符合以下格式的 SKILL.md 内容：

```markdown
---
name: <skill-name>
description: <一句话描述，说明功能和使用时机>
runAs: <inline 或 subagent>
---

# <Skill 标题>

<详细的指令内容>

## 步骤

1. ...
2. ...

## 注意事项

- ...
```

### Frontmatter 字段说明

| 字段 | 必填 | 说明 |
|------|------|------|
| `name` | 是 | 技能的唯一标识符 |
| `description` | 是 | 一句话描述（会出现在 system prompt 的技能目录中） |
| `runAs` | 否 | `inline`（默认）或 `subagent` |

### 执行模式选择指南

- **inline**：适合规范类、指导类 skill（如编码风格指南、代码审查清单）。内容直接注入当前对话，LLM 按指令执行。
- **subagent**：适合需要独立工作的 skill（如代码探索、大规模分析）。在隔离 session 中运行，只返回最终结果。

## 3. 保存

使用 `install_skill` 工具保存，参数：
- `name`：技能名称
- `content`：完整的 SKILL.md 内容（包含 frontmatter）

保存后技能立即可通过 `run_skill` 或 `/<name>` 调用。

## 4. 验证

告知用户：
- 技能已保存到 `.coding-agent/skills/<name>/SKILL.md`
- 可以通过 `run_skill("<name>")` 或 `/<name>` 调用
- 下次启动 session 时会出现在 system prompt 的技能目录中
