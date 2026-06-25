# clipboard-paste Specification

## Purpose
TBD - created by archiving change tui-input-system. Update Purpose after archive.
## Requirements
### Requirement: 剪贴板粘贴
系统 SHALL 支持用户通过 Ctrl+V 将系统剪贴板内容粘贴到输入区。

#### Scenario: 粘贴文本
- **WHEN** 用户按 Ctrl+V 且剪贴板包含文本
- **THEN** 文本插入到输入区光标位置

#### Scenario: 粘贴大文本
- **WHEN** 剪贴板文本超过 500 字符
- **THEN** 文本折叠显示为 "pasted N chars" 标签，发送时展开

#### Scenario: 粘贴图片
- **WHEN** 用户按 Ctrl+V 且剪贴板包含图片
- **THEN** 图片保存为临时文件，输入区插入 @image-ref 引用

