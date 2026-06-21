package agent

import (
	"fmt"
	"strings"

	"github.com/wsx864321/coding-agent/internal/skill"
	"github.com/wsx864321/coding-agent/internal/tools"
)

// buildSystemPrompt 根据已注册工具和 skill 自动生成 system prompt
//
// 输出格式（示例）：
//
//	你是一个编码助手，可以使用以下工具完成任务：
//
//	1. bash
//	   描述: 在本地终端执行...
//	   参数 schema: {"type":"object","properties":{...}}
//
//	2. read_file
//	   ...
//
//	请按用户意图选择合适的工具，按需连续调用多个工具以完成任务。
//
//	# Skills — 可调用的技能
//	...
func buildSystemPrompt(registry *tools.Registry, skills []skill.Skill) string {
	var b strings.Builder

	toolList := registry.List()
	if len(toolList) == 0 {
		b.WriteString("你是一个编码助手。当前未注册任何工具，请直接回答用户问题。\n")
	} else {
		b.WriteString("你是一个编码助手，可以使用以下工具完成任务。\n\n")

		for i, t := range toolList {
			fmt.Fprintf(&b, "%d. %s\n", i+1, t.Name())
			fmt.Fprintf(&b, "   描述: %s\n", t.Description())
			schema := t.Schema()
			if len(schema) > 0 {
				fmt.Fprintf(&b, "   参数 schema: %s\n", string(schema))
			}
			b.WriteString("\n")
		}

		b.WriteString("请按用户意图选择合适的工具，按需连续调用多个工具以完成任务。\n")

		if hasTodoTools(registry) {
			b.WriteString(todoGuidance)
		}
		if hasTaskTool(registry) {
			b.WriteString(taskGuidance)
		}
		if hasBackgroundJobTools(registry) {
			b.WriteString(backgroundJobGuidance)
		}
	}

	prompt := b.String()

	// 追加 skill catalog（名称 + 描述，不含 body）
	if len(skills) > 0 {
		prompt = skill.ApplyIndex(prompt, skills)
	}

	return prompt
}

// hasTodoTools 检查 registry 中是否注册了 todo_write 和 complete_step
func hasTodoTools(registry *tools.Registry) bool {
	return registry.Get("todo_write") != nil && registry.Get("complete_step") != nil
}

func hasTaskTool(registry *tools.Registry) bool {
	return registry.Get("task") != nil
}

// hasBackgroundJobTools 检查 registry 中是否注册了后台任务相关工具
func hasBackgroundJobTools(registry *tools.Registry) bool {
	return registry.Get("bash_output") != nil && registry.Get("wait") != nil
}

const todoGuidance = `
对于多步骤任务（3 步以上），使用 todo_write 和 complete_step 工具跟踪进度：
- 动手之前先调 todo_write 列出所有步骤，第一步设为 in_progress，其余 pending
- 每次只保持一个 in_progress
- 完成一个步骤的实际工作后，先调 complete_step 提供完成证据，再调 todo_write 更新状态
- 不要跳过 complete_step 直接将步骤标记为 completed，否则 todo_write 会拒绝
- 简单任务（少于 3 步）无需使用 todo_write
`

const taskGuidance = `
使用 task 工具委派子任务给独立的子 agent：
- 子 agent 在独立的 session 中运行（全新对话），只有最终回答返回
- 适合探索性工作（如搜索代码模式、分析调用链）或自包含的修改任务
- prompt 必须自包含——子 agent 看不到你的对话历史
- 不要用 task 做简单的单步操作（如读一个文件），直接调对应工具更快
- 子 agent 不能再派生子 agent（只支持一层嵌套）
`

const backgroundJobGuidance = `
对于长命令（install/build/test/deploy）或耗时子任务，使用后台执行避免阻塞：
- bash 的 run_in_background=true：立即返回 job_id，跨 turn 持续运行，不受 timeout 限制
- task 的 run_in_background=true：子 agent 在后台运行，最终回答通过 bash_output 读取
- 用 bash_output(job_id=...) 增量读取输出（非阻塞，可带 filter 正则过滤行）
- 用 wait(job_ids=[...]) 阻塞等待完成（可设 timeout_seconds）
- 用 kill_shell(job_id=...) 终止运行中的后台任务
- 后台任务完成后，下一轮会自动收到完成通知
- 不要对短命令（< 5 秒）使用后台执行，直接同步跑更快
`
