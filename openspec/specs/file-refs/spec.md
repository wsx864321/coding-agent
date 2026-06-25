# file-refs Specification

## Purpose
TBD - created by archiving change tui-input-system. Update Purpose after archive.
## Requirements
### Requirement: @文件引用解析
系统 SHALL 在用户输入 `@` 后触发文件路径补全，支持从工作目录搜索匹配文件。

#### Scenario: 触发文件补全
- **WHEN** 用户输入 `@` 后跟部分文件名
- **THEN** 补全菜单显示匹配的文件路径

#### Scenario: 接受文件引用
- **WHEN** 用户选择文件补全项
- **THEN** 输入区插入 `@path/to/file` 引用，发送时解析为文件内容

