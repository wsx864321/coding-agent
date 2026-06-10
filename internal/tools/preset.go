package tools

// DefaultRegistry 构造一个预装 5 个工具的注册表
//
// 工具清单：
//   - bash        : 跨平台 shell 执行（按设计不限制 workdir）
//   - read_file   : 读 workdir 内文件
//   - write_file  : 写 workdir 内文件
//   - edit_file   : 替换 workdir 内文件文本
//   - glob_file   : 在 workdir 内做 glob 匹配
//
// 参数 workdir：file 系列工具的白名单基准目录；空字符串不限制（部分工具会拒绝运行）
//
// 这是"默认装配"——调用方可在 DefaultRegistry 基础上追加 / 替换 / 删除工具
func DefaultRegistry(workdir string) *Registry {
	r := NewRegistry()

	r.Register(NewBashTool(workdir))
	r.Register(NewReadFileTool(workdir))
	r.Register(NewWriteFileTool(workdir))
	r.Register(NewEditFileTool(workdir))
	r.Register(NewGlobFileTool(workdir))

	return r
}
