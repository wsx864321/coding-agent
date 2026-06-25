# Brainstorm Summary

- Change: tui-input-system
- Date: 2026-06-24

## 确认的技术方案

### 斜杠命令补全
- completion 结构体（items、selected、active）
- 输入 `/` 触发，↑↓ 导航，Tab/Enter 接受，Esc 关闭
- 命令列表从 slash commands 注册表获取

### 输入历史回溯
- submittedInputs []string + submittedInputCursor int
- 空闲时 ↑↓ 回溯，编辑后 Enter 作为新消息发送

### 排队输入
- pendingInterject []string 队列
- 运行中 Enter 追加到队列，TurnDone 自动发送队首
- 显示 "N queued" 提示

### 剪贴板粘贴
- Ctrl+V 读取 clipboard，大文本折叠显示
- 图片粘贴保存临时文件 + @引用

### @文件引用
- 输入 `@` 触发文件路径补全
- 基于工作目录 glob/walk 搜索

### #快速记忆
- `# note` 格式检测，调用 memory.QuickAdd
- 显示确认提示

### !shell 直接执行
- `!cmd` 格式检测，绕过模型执行
- 输入区边框变色指示 Shell 模式

### Plan/YOLO 模式切换
- Shift+Tab 切换 Plan，Ctrl+Y 切换 YOLO
- 状态栏显示对应标签（Plan=蓝，YOLO=红）

## 关键取舍与风险

| 取舍/风险 | 缓解 |
|----------|------|
| YOLO 模式安全 | 红色醒目标签 + 仅显式 Ctrl+Y 激活 |
| 文件补全性能 | 限制搜索深度和结果数 |
| 剪贴板跨平台 | atotto/clipboard 三平台测试 |

## 测试策略

- 单元测试：补全过滤、历史回溯、排队队列、模式切换
- 集成测试：斜杠命令流程、@引用解析、#记忆写入、!shell 执行
- 回归测试：全量 TUI 测试套件

## Spec Patch

无
