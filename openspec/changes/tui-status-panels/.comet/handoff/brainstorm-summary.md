# Brainstorm Summary

- Change: tui-status-panels
- Date: 2026-06-24

## 确认的技术方案

### 三行状态栏布局
- 工作行（仅 busy 时）：spinner + elapsed + token↓
- 模式行（始终）：Plan/YOLO/Shell 标签 + model + effort + git
- 数据行（始终）：ctx 仪表 + cache 率 + jobs 数 + balance
- 每行超宽时自然换行（lipgloss MaxWidth wrap）
- bottomHeight() 动态计算三行 + Todo 面板高度

### 上下文仪表
- 格式：`ctx N/M (P%)`
- 着色：<50% 绿、50-80% 黄、>80% 红
- 从 Agent 获取 ContextSnapshot(used, window int)

### Git 状态
- 异步 Cmd：git rev-parse + git status --porcelain
- 结果通过 gitStatusMsg 返回
- 非 Git 仓库静默降级

### Todo 面板
- 解析 todo_write JSON 参数
- 渲染在输入区上方
- 空列表不占空间

### 余额
- 异步 Cmd 调用 provider 余额接口
- 结果通过 balanceMsg 返回
- 失败静默降级

### 自定义状态行
- 用户配置 command，stdout 第一行替换数据行
- 超时 2s，失败回退内置数据行

## 关键取舍与风险

| 取舍/风险 | 缓解 |
|----------|------|
| Git 命令耗时 | 异步执行 + 超时 |
| 小终端拥挤 | 自然换行不截断 |
| 余额接口不可用 | 静默降级 |

## 测试策略

- 单元测试：三行布局渲染、上下文仪表着色、Todo 面板解析
- 集成测试：Git 状态刷新、余额刷新、自定义状态行
- 回归测试：全量 TUI 测试套件

## Spec Patch

无
