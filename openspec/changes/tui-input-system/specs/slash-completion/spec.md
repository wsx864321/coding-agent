## ADDED Requirements

### Requirement: 斜杠命令自动补全
系统 SHALL 在用户输入 `/` 后显示可用命令的补全菜单，支持键盘导航和选择。

#### Scenario: 触发补全菜单
- **WHEN** 用户输入以 `/` 开头的文本
- **THEN** 补全菜单显示匹配的命令列表（如 /help、/skills、/model 等）

#### Scenario: 键盘导航
- **WHEN** 补全菜单显示且用户按 ↑↓
- **THEN** 高亮项在菜单中上下移动

#### Scenario: 接受补全
- **WHEN** 用户按 Tab 或 Enter 选择补全项
- **THEN** 输入区替换为完整命令，补全菜单关闭

#### Scenario: 关闭补全
- **WHEN** 补全菜单显示且用户按 Esc
- **THEN** 补全菜单关闭，输入区保持当前文本
