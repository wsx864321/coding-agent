# Brainstorm Summary

- Change: upgrade-tui-core
- Date: 2026-06-23

## 确认的技术方案

### Bubble Tea v2 迁移
- 原地升级：直接替换 go.mod 中 `bubbletea v1.3.4` → `charm.land/bubbletea/v2`
- 同步升级 lipgloss → `charm.land/lipgloss/v2`，引入 `charm.land/bubbles/v2`
- chat 命令不使用 Bubble Tea，不受影响
- 一次性迁移所有 `internal/tui/` 和 `cmd/cli/tui.go` 的 import path

### Markdown 渲染
- 使用 glamour（Charm 官方 Markdown terminal renderer）作为首版方案
- 通过 Renderer interface 封装，后续可替换为自定义 goldmark walker
- 流式段落边界刷新（flushableMarkdownPrefix）不依赖 renderer 是否增量——每次刷新完整的已确认段落
- glamour 配置：WordWrap 绑定终端宽度，使用 DarkStyle 基础样式

### 事件系统
- 扩展 StreamEmitter 接口，增加 OnToolStart/OnToolEnd/OnApprovalRequest
- 修改 RunStreaming 签名：`onText func(string)` → `emitter StreamEmitter`
- agent 层 loop.go 的 invokeTool 前后调用 emitter
- 非流式 Run() 路径不受影响

### 审批并发模型
- `chan bool` + `sync.Once` 保护 + `ctx.Done` 超时
- agent goroutine 在 `<-ch` 阻塞等待 TUI 用户响应
- TUI 主线程通过 `respondFn(bool)` 解锁
- Esc 中断通过 context cancel 传播，agent 侧 select 走 ctx.Done 分支返回 denied

### 组件选型
- textarea: bubbles/v2 textarea — 多行、光标、IME、Shift+Enter 换行
- viewport: bubbles/v2 viewport — 滚动条、PgUp/PgDn/Home/End、鼠标滚轮、tail-follow
- spinner: bubbles/v2 spinner — 动画进度 + 耗时计数
- CJK: go-runewidth 替代 utf8.RuneLen

## 关键取舍与风险

- glamour 减少实现量（~100 行 vs ~500 行）但牺牲定制灵活性；Renderer interface 保留替换能力
- RunStreaming 签名变更只影响 tui_runner.go 一个调用者，风险可控
- 审批阻塞期间 agent loop 完全暂停，Esc 通过 ctx cancel 安全退出
- v2 API 差异（tea.View struct、tea.KeyMsg 字段、key.Binding）需要逐项适配

## 测试策略

- 单元测试：Markdown renderer、工具卡片、审批横幅、flushablePrefix 独立测试
- 集成测试：mock Runner + 预设事件序列验证 Update-View 循环
- v2 迁移测试：先迁移修复现有测试编译，确认基础功能不退化
- CJK 测试：中文/日文/emoji 混合换行
- 边界测试：空消息、超长输出、半代码块、中断审批

## Spec Patch

无。delta spec 已覆盖所有确认的功能需求。glamour 替代自定义 walker 是实现决策，不影响 spec requirement。
