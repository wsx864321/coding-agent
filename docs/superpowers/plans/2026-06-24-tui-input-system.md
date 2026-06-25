# TUI 输入系统与交互增强 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**base-ref:** `75c1200bfe5c83356dac253597117a7f434c12e4`

**Goal:** 为 Bubble Tea TUI 实现斜杠命令自动补全、输入历史回溯、运行中排队输入、剪贴板粘贴、@文件引用、#快速记忆、!shell 执行和 Plan/YOLO 模式切换八个交互增强子系统。

**Architecture:** 各子系统独立设计，通过 Model 的 Update 函数路由按键事件。补全和历史直接操作 textarea；粘贴和文件引用通过 textarea.InsertString 插入；#和!在 submit 中检测并代理给 controller；模式切换通过快捷键修改 Model 状态字段。

**Tech Stack:** Go 1.22+, charm.land/bubbles/v2 (textarea, key), charm.land/lipgloss/v2, github.com/atotto/clipboard

## 全局约束

- 所有新增文件使用 package `tui`
- 测试文件使用 `_test.go` 后缀，与源文件同目录
- TDD 流程：先写失败测试 → 验证 RED → 写实现 → 验证 GREEN → 提交
- 每个 task 独立可提交，commit message 格式：`feat(tui): <简短描述>`
- 使用 `go build ./internal/tui/...` 和 `go test ./internal/tui/...` 验证
- 斜杠命令列表：`/diff-fold`（已有）、`/help`、`/skills`、`/model`、`/clear`、`/reset`、`/exit`、`/quit`、`/history`、`/tools`、`/hooks`、`/compact`、`/jobs`

---

### Task 1: 斜杠命令自动补全

**Files:**
- Create: `internal/tui/completion.go`
- Create: `internal/tui/completion_test.go`
- Modify: `internal/tui/model.go`
- Modify: `cmd/cli/tui.go`

**Interfaces:**
- Consumes: 无（独立子系统）
- Produces: `type completion struct { items []string; selected int; active bool }` + `func renderCompletion(c completion, width int) string` + `func filterCommands(cmds []string, prefix string) []string`

- [ ] **Step 1: 写 completion 结构体和 renderCompletion 的失败测试**

```go
// completion_test.go
package tui

import (
	"strings"
	"testing"
)

func TestCompletionStructEmpty(t *testing.T) {
	c := completion{}
	if c.active {
		t.Fatal("new completion should not be active")
	}
	if c.selected != 0 {
		t.Fatalf("selected = %d, want 0", c.selected)
	}
	if len(c.items) != 0 {
		t.Fatalf("items = %v, want empty", c.items)
	}
}

func TestFilterCommandsEmpty(t *testing.T) {
	cmds := []string{"/help", "/skills", "/model", "/clear", "/diff-fold"}
	result := filterCommands(cmds, "")
	if len(result) != len(cmds) {
		t.Fatalf("filter with empty prefix: got %d items, want %d", len(result), len(cmds))
	}
}

func TestFilterCommandsPrefix(t *testing.T) {
	cmds := []string{"/help", "/skills", "/model", "/clear", "/diff-fold"}
	result := filterCommands(cmds, "/s")
	if len(result) != 1 {
		t.Fatalf("filter /s: got %d items, want 1", len(result))
	}
	if result[0] != "/skills" {
		t.Fatalf("filter /s: got %q, want /skills", result[0])
	}
}

func TestFilterCommandsNoMatch(t *testing.T) {
	cmds := []string{"/help", "/skills"}
	result := filterCommands(cmds, "/x")
	if len(result) != 0 {
		t.Fatalf("filter /x: got %d items, want 0", len(result))
	}
}

func TestRenderCompletionActive(t *testing.T) {
	c := completion{
		active:   true,
		items:    []string{"/help", "/skills", "/model"},
		selected: 1,
	}
	out := renderCompletion(c, 40)
	if out == "" {
		t.Fatal("renderCompletion with active=true should produce output")
	}
	if !strings.Contains(out, "/help") {
		t.Fatal("renderCompletion should contain /help")
	}
	if !strings.Contains(out, "/skills") {
		t.Fatal("renderCompletion should contain /skills")
	}
	if !strings.Contains(out, "/model") {
		t.Fatal("renderCompletion should contain /model")
	}
}

func TestRenderCompletionInactive(t *testing.T) {
	c := completion{
		active: false,
		items:  []string{"/help", "/skills"},
	}
	out := renderCompletion(c, 40)
	if out != "" {
		t.Fatalf("renderCompletion with active=false: got %q, want empty", out)
	}
}

func TestRenderCompletionEmptyItems(t *testing.T) {
	c := completion{
		active: true,
		items:  nil,
	}
	out := renderCompletion(c, 40)
	if out != "" {
		t.Fatalf("renderCompletion with no items: got %q, want empty", out)
	}
}
```

- [ ] **Step 2: 运行测试确认失败 (RED)**

Run: `go test ./internal/tui/ -run "TestCompletion|TestFilter|TestRender" -count=1`
Expected: FAIL — "undefined: completion" / "undefined: filterCommands" / "undefined: renderCompletion"

- [ ] **Step 3: 实现 completion 结构体和函数 — completion.go**

```go
// completion.go
package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// completion 表示斜杠命令自动补全菜单的状态。
type completion struct {
	items    []string // 匹配的命令列表
	selected int      // 当前选中索引
	active   bool     // 菜单是否可见
}

var (
	completionMenuStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("8")).
				Padding(0, 1)

	completionItemStyle       = lipgloss.NewStyle()
	completionSelectedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true)
)

// renderCompletion 渲染补全菜单。如果补全不活跃或 items 为空，返回空字符串。
func renderCompletion(c completion, width int) string {
	if !c.active || len(c.items) == 0 {
		return ""
	}
	var lines []string
	for i, item := range c.items {
		if i == c.selected {
			lines = append(lines, completionSelectedStyle.Render("▶ "+item))
		} else {
			lines = append(lines, completionItemStyle.Render("  "+item))
		}
	}
	content := strings.Join(lines, "\n")
	return completionMenuStyle.Width(width - 2).Render(content)
}

// filterCommands 根据前缀过滤命令列表，返回匹配的子集。
// prefix 包含开头的 "/"，如 "/s"。
func filterCommands(cmds []string, prefix string) []string {
	if prefix == "" {
		return append([]string(nil), cmds...)
	}
	var result []string
	for _, c := range cmds {
		if strings.HasPrefix(c, prefix) {
			result = append(result, c)
		}
	}
	return result
}
```

- [ ] **Step 4: 在 model.go 中新增 completion 字段和 slashCommands**

在 Model struct 中添加（约 line 118 后）：

```go
	// --- 斜杠命令补全 ---
	completion    completion
	slashCommands []string
```

在 New() 函数中添加默认 slashCommands：

```go
func New() Model {
	return Model{
		// ... 已有字段 ...
		slashCommands: []string{
			"/help", "/skills", "/model", "/clear", "/reset",
			"/exit", "/quit", "/history", "/tools", "/hooks",
			"/compact", "/jobs", "/diff-fold",
		},
	}
}
```

添加 setter 方法：

```go
// SetSlashCommands 设置可补全的斜杠命令列表。
func (m *Model) SetSlashCommands(cmds []string) {
	m.slashCommands = cmds
}
```

- [ ] **Step 5: 在 Update 中处理补全逻辑**

在 `case tea.KeyPressMsg:` 分支的 `default` 之前添加补全键处理（约 line 493 前）：

```go
		case m.completion.active:
			// 补全菜单激活时的按键处理
			switch msg.String() {
			case "up":
				if m.completion.selected > 0 {
					m.completion.selected--
				}
				return m, nil
			case "down":
				if m.completion.selected < len(m.completion.items)-1 {
					m.completion.selected++
				}
				return m, nil
			case "tab", "enter":
				if len(m.completion.items) > 0 {
					sel := m.completion.items[m.completion.selected]
					m.textarea.SetValue(sel + " ")
					m.textarea.SetCursor(len(sel) + 1)
				}
				m.completion = completion{}
				return m, nil
			case "esc":
				m.completion = completion{}
				return m, nil
			default:
				m.completion = completion{}
				// fall through to default
			}
```

在 `default` 分支中（约 line 493），在交给 textarea 之前检测 `/`：

```go
		default:
			if m.busy {
				return m, nil
			}
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(msg)
			// 检测斜杠命令补全
			m = m.checkSlashCompletion()
			m = m.syncLayout()
			return m, cmd
```

添加 `checkSlashCompletion` 方法：

```go
// checkSlashCompletion 检测当前输入是否触发斜杠命令补全。
func (m Model) checkSlashCompletion() Model {
	val := m.textarea.Value()
	// 仅在输入以单独 "/" 开头且未包含空格时激活补全
	if strings.HasPrefix(val, "/") && !strings.Contains(val, " ") {
		items := filterCommands(m.slashCommands, val)
		if len(items) > 0 {
			m.completion = completion{
				items:    items,
				selected: 0,
				active:   true,
			}
		} else {
			m.completion = completion{}
		}
	} else {
		m.completion = completion{}
	}
	return m
}
```

- [ ] **Step 6: 在 View 中渲染补全菜单**

在 view.go 的 `View()` 方法中，在输入区（`m.textarea.View()`）之前插入：

```go
	if m.completion.active {
		parts = append(parts, renderCompletion(m.completion, m.contentWidth()))
	}
```

- [ ] **Step 7: 在 cmd/cli/tui.go 中传递 slashCommands**

修改 `runTui` 函数：

```go
func runTui(cmd *cobra.Command, args []string) error {
	setup, err := setupTuiAgent(cmd)
	if err != nil {
		return err
	}
	defer setup.cleanup()

	cfg := buildConfig(cmd)
	workdir := resolveWorkdir(cmd)
	sessionBucket := agent.SessionBucket(agent.ResolveSessionDir(cfg.SessionDir), workdir)
	setup.Agent.SetSessionPath(agent.NewSessionPath(sessionBucket, cfg.Model))

	m := tui.NewWithRunner(newAgentRunner(setup.Agent), setup.TuiSink)
	m.SetSlashCommands(defaultSlashCommands())
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		return err
	}
	return nil
}
```

新增 `defaultSlashCommands` 函数（在 tui.go 末尾）：

```go
// defaultSlashCommands 返回 TUI 中可用的斜杠命令列表。
func defaultSlashCommands() []string {
	return []string{
		"/help", "/skills", "/model", "/clear", "/reset",
		"/exit", "/quit", "/history", "/tools", "/hooks",
		"/compact", "/jobs", "/diff-fold",
	}
}
```

- [ ] **Step 8: 运行测试确认通过 (GREEN)**

Run: `go test ./internal/tui/ -run "TestCompletion|TestFilter|TestRender" -count=1 -v`
Expected: PASS

- [ ] **Step 9: 运行构建确认编译通过**

Run: `go build ./internal/tui/...`
Expected: 无错误

- [ ] **Step 10: 提交**

```bash
git add internal/tui/completion.go internal/tui/completion_test.go internal/tui/model.go internal/tui/view.go cmd/cli/tui.go
git commit -m "feat(tui): slash command autocomplete with completion menu"
```

---

### Task 2: 输入历史回溯

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`

**Interfaces:**
- Consumes: Model.submit() 机制
- Produces: `submittedInputs []string`, `submittedInputCursor int`, `rememberSubmittedInput()`, `recallSubmittedInput(dir int)`

（Task 2-8 的详细步骤将在后续 plan 迭代中补充，聚焦 Task 1 先行实现。）
