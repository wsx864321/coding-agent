package tools

// DefaultRegistry 构造预装工具的注册表
//
// 工具清单：
//   - bash        : 跨平台 shell 执行（支持 run_in_background）
//   - read_file   : 读文件
//   - write_file  : 写文件
//   - edit_file   : 替换文件文本
//   - glob_file   : glob 匹配
//   - grep        : 正则搜索文件内容
//   - web_fetch   : HTTP/HTTPS URL 抓取
//   - worktree    : git worktree 管理（create/list/remove）
//   - todo_write  : 多步骤任务跟踪
//   - complete_step: 步骤完成凭证
//   - task        : 委派子 agent（支持 run_in_background）
//   - compact     : 手动压缩上下文
//   - remember    : 保存长期记忆
//   - forget      : 删除长期记忆
//   - recall      : 搜索/读取/列出长期记忆
//   - bash_output : 读取后台任务输出（非阻塞）
//   - kill_shell  : 终止后台任务
//   - wait        : 等待后台任务完成
//
// 参数 workdir：file 系列工具的白名单基准目录；空字符串不限制。
//
// 这是"默认装配"——调用方可在 DefaultRegistry 基础上追加 / 替换 / 删除工具。
func DefaultRegistry(workdir string) *Registry {
	r := NewRegistry()

	r.Register(NewBashTool(workdir))
	r.Register(NewReadFileTool(workdir))
	r.Register(NewWriteFileTool(workdir))
	r.Register(NewEditFileTool(workdir))
	r.Register(NewGlobFileTool(workdir))
	r.Register(NewGrepTool(workdir))
	r.Register(NewWebFetchTool())
	r.Register(NewWorktreeTool(workdir))
	r.Register(NewTodoWriteTool())
	r.Register(NewCompleteStepTool())
	r.Register(NewTaskTool(nil)) // runner 由 agent.WireTaskTool() 延迟注入
	r.Register(NewCompactTool())

	// 记忆工具——先以 nil store/queue 注册占位，由 agent.WireMemoryTools() 延迟注入真实实例
	r.Register(NewRememberTool(nil, nil))
	r.Register(NewForgetTool(nil, nil))
	r.Register(NewRecallTool(nil))

	// 后台任务配套工具——通过 jobs.FromContext 访问 Manager，由 agent 注入
	r.Register(NewBashOutputTool())
	r.Register(NewKillShellTool())
	r.Register(NewWaitTool())

	return r
}
