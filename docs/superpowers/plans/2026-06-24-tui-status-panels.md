---
change: tui-status-panels
design-doc: docs/superpowers/specs/2026-06-24-tui-status-panels-design.md
base-ref: 121a388ed1881e20ae38b704b14556a333cec720
---

# TUI 状态栏与信息面板重构 — 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将单行状态栏重构为三行信息面板（工作行/模式行/数据行），并集成上下文仪表、Git 状态、缓存命中率、余额、Todo 任务面板和自定义状态行。

**Architecture:** 重写 `statusbar.go` 提供三个纯渲染函数，扩展 `Model` 结构体容纳新增状态字段，通过异步 `tea.Cmd` 获取 Git/余额等外部数据，在 `View()` 中按动态高度组装底部面板。

**Tech Stack:** Go 1.21+, Bubble Tea v2, Lipgloss v2, 现有 `internal/tui` 包

## Global Constraints

- Go 版本 >= 1.21
- 依赖不新增（使用现有 Bubble Tea + Lipgloss）
- 渲染函数为纯函数（接收 Model 或基本类型，返回 string）
- 异步命令通过 `tea.Cmd` 模式实现，不阻塞 UI 线程
- 所有新增公开函数/类型需有单元测试覆盖
- 回归测试必须通过：`go test ./internal/tui/...`

---

### Task 1: Model 结构体扩展 — 新增字段与类型

**Files:**
- Modify: `internal/tui/model.go:35-73`

**Interfaces:**
- Consumes: 现有 `Model` 结构体
- Produces:
  - `gitStatus` struct (`branch string; ahead int; behind int; dirty bool`)
  - `todoItem` struct (`Content string; Status string; ActiveForm string`)
  - 新增 Model 字段：`gitStatus`, `contextUsed int`, `contextWindow int`, `cacheHitRate int`, `balance string`, `todoArgs string`, `todoItems []todoItem`, `statuslineCmd string`, `statuslineOut string`
  - 新增消息类型：`gitStatusMsg`, `balanceMsg`, `statuslineMsg`

- [ ] **Step 1: 在 `model.go` 顶部新增类型定义**

在 `import` 块之后、`const interruptedStatusMsg` 之前插入：

```go
// gitStatus 保存最近一次 git 状态快照。
type gitStatus struct {
	branch string
	ahead  int
	behind int
	dirty  bool
}

// todoItem 表示 todo_write 工具中的单个任务项。
type todoItem struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"activeForm"`
}

// gitStatusMsg 携带异步 git 查询结果。
type gitStatusMsg struct {
	status gitStatus
}

// balanceMsg 携带异步余额查询结果。
type balanceMsg struct {
	text string
}

// statuslineMsg 携带自定义状态行命令的输出。
type statuslineMsg struct {
	out string
}
```

- [ ] **Step 2: 在 `Model` 结构体中添加新字段**

在 `diffMaxLines` 字段之后追加：

```go
	// --- 状态面板字段 ---
	gitStatus     gitStatus
	contextUsed   int
	contextWindow int
	cacheHitRate  int    // 0-100 百分比
	balance       string // 格式化后的余额文本，如 "¥110.00"
	todoArgs      string // 最近一次 todo_write 的原始 JSON 参数
	todoItems     []todoItem
	statuslineCmd  string
	statuslineOut  string
```

- [ ] **Step 3: 编译验证**

```powershell
cd D:\project\coding-agent; go build ./internal/tui/...
```
Expected: 编译成功，新增字段未使用但无报错。

- [ ] **Step 4: Commit**

```powershell
git add internal/tui/model.go
git commit -m "feat(tui): add model fields for status panels (git, balance, todo, statusline)"
```

---

### Task 2: 重写 statusbar.go — 三行布局渲染

**Files:**
- Rewrite: `internal/tui/statusbar.go`
- Modify: `internal/tui/statusbar_test.go`

**Interfaces:**
- Consumes: `Model` 结构体（含 Task 1 新增字段）
- Produces:
  - `renderWorkingLine(m Model) string` — busy 时返回 spinner 行，idle 时返回 ""
  - `renderModeLine(m Model) string` — 始终返回模式行
  - `renderDataLine(m Model) string` — 始终返回数据行
  - `renderContextGauge(used, window int) string` — 上下文仪表
  - `gaugeColor(pct int) lipgloss.Style` — 阈值着色
  - `shortTokens(n int) string` — 人类可读 token 数
  - `renderGitStatusStr(gs gitStatus) string` — Git 状态字符串

- [ ] **Step 1: 编写新 statusbar 的单元测试**

替换 `internal/tui/statusbar_test.go` 全部内容：

```go
package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestRenderWorkingLineIdle(t *testing.T) {
	m := New()
	got := renderWorkingLine(m)
	if got != "" {
		t.Fatalf("renderWorkingLine(idle) = %q, want empty", got)
	}
}

func TestRenderWorkingLineBusy(t *testing.T) {
	m := New()
	m.busy = true
	m.runStart = time.Now().Add(-3 * time.Second)
	m.statusLabel = "running bash..."

	got := renderWorkingLine(m)
	for _, want := range []string{"running bash...", "3s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderWorkingLine(busy) = %q, missing %q", got, want)
		}
	}
}

func TestRenderWorkingLineDefaultLabel(t *testing.T) {
	m := New()
	m.busy = true
	m.runStart = time.Now()

	got := renderWorkingLine(m)
	if !strings.Contains(got, "thinking") {
		t.Fatalf("renderWorkingLine(busy) = %q, missing thinking label", got)
	}
}

func TestRenderModeLine(t *testing.T) {
	m := New()
	m.modelName = "deepseek-flash"
	m.gitStatus = gitStatus{branch: "main", ahead: 3}

	got := renderModeLine(m)
	for _, want := range []string{"Plan", "deepseek-flash", "main ↑3"} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderModeLine = %q, missing %q", got, want)
		}
	}
}

func TestRenderModeLineDirty(t *testing.T) {
	m := New()
	m.modelName = "deepseek-flash"
	m.gitStatus = gitStatus{branch: "main", dirty: true}

	got := renderModeLine(m)
	if !strings.Contains(got, "main *") {
		t.Fatalf("renderModeLine(dirty) = %q, missing dirty marker", got)
	}
}

func TestRenderDataLine(t *testing.T) {
	m := New()
	m.contextUsed = 45000
	m.contextWindow = 128000
	m.cacheHitRate = 85
	m.balance = "¥110.00"

	got := renderDataLine(m)
	for _, want := range []string{"ctx", "45K", "128K", "35%", "cache 85%", "¥110.00"} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderDataLine = %q, missing %q", got, want)
		}
	}
}

func TestRenderContextGauge(t *testing.T) {
	tests := []struct {
		used, window int
		want         string
	}{
		{0, 0, ""},
		{45000, 128000, "ctx 45K/128K (35%)"},
		{0, 128000, "ctx 0/128K (0%)"},
		{128000, 128000, "ctx 128K/128K (100%)"},
	}
	for _, tc := range tests {
		got := renderContextGauge(tc.used, tc.window)
		if got != tc.want {
			t.Fatalf("renderContextGauge(%d, %d) = %q, want %q", tc.used, tc.window, got, tc.want)
		}
	}
}

func TestGaugeColorThresholds(t *testing.T) {
	green := gaugeColor(30)
	yellow := gaugeColor(60)
	red := gaugeColor(85)

	// 验证三者返回不同样式
	gs := green.Render("x")
	ys := yellow.Render("x")
	rs := red.Render("x")

	if gs == ys || ys == rs || gs == rs {
		t.Fatalf("gaugeColor thresholds should produce distinct styles: green=%q yellow=%q red=%q", gs, ys, rs)
	}
}

func TestShortTokens(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{500, "500"},
		{1500, "1.5K"},
		{45000, "45K"},
		{128000, "128K"},
		{1500000, "1.5M"},
	}
	for _, tc := range tests {
		got := shortTokens(tc.n)
		if got != tc.want {
			t.Fatalf("shortTokens(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestRenderGitStatusStr(t *testing.T) {
	tests := []struct {
		gs   gitStatus
		want string
	}{
		{gitStatus{branch: "main"}, "main"},
		{gitStatus{branch: "main", dirty: true}, "main *"},
		{gitStatus{branch: "main", ahead: 3}, "main ↑3"},
		{gitStatus{branch: "feat/x", ahead: 2, behind: 1}, "feat/x ↑2 ↓1"},
	}
	for _, tc := range tests {
		got := renderGitStatusStr(tc.gs)
		if got != tc.want {
			t.Fatalf("renderGitStatusStr(%+v) = %q, want %q", tc.gs, got, tc.want)
		}
	}
}

func TestBottomHeightBase(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m = m.syncLayout()

	// 2 (mode + data) + 1 (help) + textarea
	want := 2 + 1 + m.textarea.Height()
	if got := m.bottomHeight(); got != want {
		t.Fatalf("bottomHeight() = %d, want %d", got, want)
	}
}

func TestBottomHeightBusy(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.busy = true
	m = m.syncLayout()

	// 1 (working) + 2 (mode + data) + 1 (help) + textarea
	want := 1 + 2 + 1 + m.textarea.Height()
	if got := m.bottomHeight(); got != want {
		t.Fatalf("bottomHeight(busy) = %d, want %d", got, want)
	}
}

func TestBottomHeightWithTodo(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.todoArgs = `[{"content":"task1","status":"pending"}]`
	m = m.syncLayout()

	// 2 (mode + data) + 1 (todo) + 1 (help) + textarea
	want := 2 + 1 + 1 + m.textarea.Height()
	if got := m.bottomHeight(); got != want {
		t.Fatalf("bottomHeight(todo) = %d, want %d", got, want)
	}
}

func TestBottomHeightWithApprovalAndError(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.approval = &pendingApproval{toolName: "write_file", respond: func(bool) {}}
	m.lastError = "network down"
	m = m.syncLayout()

	want := 2 + 1 + m.textarea.Height() + 2 + 1
	if got := m.bottomHeight(); got != want {
		t.Fatalf("bottomHeight() = %d, want %d", got, want)
	}
}

func TestWindowSizeSetsViewportHeightFromBottom(t *testing.T) {
	m := New()
	m.textarea.SetValue("line1\nline2")
	m = m.syncLayout()

	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	updated := next.(Model)

	bottom := updated.bottomHeight()
	wantVP := 30 - bottom
	if wantVP < 1 {
		wantVP = 1
	}
	if updated.viewport.Height() != wantVP {
		t.Fatalf("viewport height = %d, want %d (term=%d bottom=%d)", updated.viewport.Height(), wantVP, 30, bottom)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```powershell
cd D:\project\coding-agent; go test ./internal/tui/ -run "TestRender|TestGaugeColor|TestShortTokens|TestBottomHeight|TestWindowSize" -v -count=1
```
Expected: 编译失败 — `renderWorkingLine`、`renderModeLine`、`renderDataLine` 等函数未定义。

- [ ] **Step 3: 重写 `internal/tui/statusbar.go`**

替换 `statusbar.go` 全部内容：

```go
package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// ---- 三行状态栏渲染 ----

// renderWorkingLine 在 busy 时渲染 spinner 工作行，idle 时返回空字符串。
func renderWorkingLine(m Model) string {
	if !m.busy {
		return ""
	}
	spin := m.spinner.View()
	elapsed := time.Since(m.runStart).Truncate(time.Second)
	label := m.statusLabel
	if label == "" {
		label = "thinking"
	}
	return fmt.Sprintf("%s %s %s", spin, label, elapsed)
}

// renderModeLine 始终渲染模式信息行。
func renderModeLine(m Model) string {
	model := m.modelName
	if model == "" {
		model = "coding-agent"
	}

	gitPart := renderGitStatusStr(m.gitStatus)

	return fmt.Sprintf("Plan · %s · %s", model, gitPart)
}

// renderDataLine 始终渲染数据信息行。
func renderDataLine(m Model) string {
	var parts []string

	// 上下文仪表
	if gauge := renderContextGauge(m.contextUsed, m.contextWindow); gauge != "" {
		parts = append(parts, gauge)
	}

	// 缓存命中率
	if m.cacheHitRate > 0 {
		parts = append(parts, fmt.Sprintf("cache %d%%", m.cacheHitRate))
	}

	// 余额
	if m.balance != "" {
		parts = append(parts, m.balance)
	}

	return joinWithSep(parts, " · ")
}

// ---- 上下文仪表 ----

// renderContextGauge 渲染上下文窗口用量仪表。window <= 0 时返回空。
func renderContextGauge(used, window int) string {
	if window <= 0 {
		return ""
	}
	pct := used * 100 / window
	color := gaugeColor(pct)
	return color.Render(fmt.Sprintf("ctx %s/%s (%d%%)", shortTokens(used), shortTokens(window), pct))
}

// gaugeColor 根据百分比返回对应颜色样式。
// <50: 绿色, 50-80: 黄色, >80: 红色
func gaugeColor(pct int) lipgloss.Style {
	switch {
	case pct > 80:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // 红
	case pct >= 50:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // 黄
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // 绿
	}
}

// shortTokens 将 token 数转为人类可读格式（K/M）。
func shortTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.0fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// ---- Git 状态 ----

// renderGitStatusStr 格式化 git 状态字符串。
func renderGitStatusStr(gs gitStatus) string {
	if gs.branch == "" {
		return "—"
	}
	s := gs.branch
	if gs.dirty {
		s += " *"
	} else {
		if gs.ahead > 0 {
			s += fmt.Sprintf(" ↑%d", gs.ahead)
		}
		if gs.behind > 0 {
			s += fmt.Sprintf(" ↓%d", gs.behind)
		}
	}
	return s
}

// ---- 辅助函数 ----

// joinWithSep 用分隔符连接非空字符串。
func joinWithSep(parts []string, sep string) string {
	var filtered []string
	for _, p := range parts {
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	result := filtered[0]
	for i := 1; i < len(filtered); i++ {
		result += sep + filtered[i]
	}
	return result
}

// ---- bottomHeight 动态计算 ----

func (m Model) bottomHeight() int {
	h := 0
	if m.busy {
		h++ // 工作行
	}
	h += 2 // 模式行 + 数据行
	if m.todoArgs != "" {
		h++ // Todo 面板
	}
	h += m.textarea.Height() // 输入区
	h += 1                    // 帮助行
	if m.approval != nil {
		h += 2 // 审批横幅
	}
	if m.lastError != "" {
		h += 1
	}
	return h
}
```

- [ ] **Step 4: 运行测试确认通过**

```powershell
cd D:\project\coding-agent; go test ./internal/tui/ -run "TestRender|TestGaugeColor|TestShortTokens|TestBottomHeight|TestWindowSize" -v -count=1
```
Expected: 所有测试 PASS。

- [ ] **Step 5: Commit**

```powershell
git add internal/tui/statusbar.go internal/tui/statusbar_test.go
git commit -m "feat(tui): rewrite statusbar with three-line layout and context gauge"
```

---

### Task 3: 更新 View() 组装新的底部布局

**Files:**
- Modify: `internal/tui/view.go:19-49`

**Interfaces:**
- Consumes: `renderWorkingLine`, `renderModeLine`, `renderDataLine` (Task 2), `renderTodoPanel` (将在 Task 6 中定义，此处先以空实现占位)
- Produces: 更新后的 `View()` 方法（三行状态栏 + Todo 面板 + 输入区 + 帮助行）

- [ ] **Step 1: 编写 View 布局测试**

在 `internal/tui/statusbar_test.go` 末尾追加：

```go
func TestViewThreePanelLayout(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.modelName = "coding-agent"
	m = m.syncLayout()
	m = m.syncViewportContent()

	view := viewContent(m)
	for _, want := range []string{
		"> ",
		"Plan",
		"coding-agent",
		"Shift+Enter",
		"Enter 发送",
		"Ctrl+C",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("View missing %q:\n%s", want, view)
		}
	}
}

func TestViewShowsApprovalBannerAboveStatusBar(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.approval = &pendingApproval{
		toolName: "write_file",
		args:     map[string]any{"path": "config.yaml"},
		respond:  func(bool) {},
	}
	m = m.syncLayout()

	view := viewContent(m)
	statusIdx := strings.Index(view, "Plan")
	bannerIdx := strings.Index(view, "Allow")
	if bannerIdx < 0 {
		t.Fatalf("View missing approval banner:\n%s", view)
	}
	if statusIdx < 0 {
		t.Fatalf("View missing status bar:\n%s", view)
	}
	if bannerIdx > statusIdx {
		t.Fatalf("approval banner should appear above status bar:\n%s", view)
	}
}

func TestViewBusyShowsWorkingLine(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.busy = true
	m.runStart = time.Now()
	m = m.syncLayout()

	view := viewContent(m)
	if !strings.Contains(view, "thinking") {
		t.Fatalf("View(busy) missing thinking:\n%s", view)
	}
}

func TestViewWithTodoPanel(t *testing.T) {
	m := New()
	m.width = 80
	m.height = 24
	m.todoArgs = `[{"content":"task1","status":"in_progress","activeForm":"testing"}]`
	m.todoItems = []todoItem{
		{Content: "task1", Status: "in_progress", ActiveForm: "testing"},
	}
	m = m.syncLayout()

	view := viewContent(m)
	if !strings.Contains(view, "task1") {
		t.Fatalf("View(todo) missing task:\n%s", view)
	}
}
```

注意：需要添加 `import "time"` 到测试文件（如果尚未导入）。

- [ ] **Step 2: 运行测试确认失败**

```powershell
cd D:\project\coding-agent; go test ./internal/tui/ -run "TestViewThreePanelLayout|TestViewShowsApprovalBanner|TestViewBusyShowsWorkingLine|TestViewWithTodoPanel" -v -count=1
```
Expected: 部分测试失败（View 仍使用旧布局）。

- [ ] **Step 3: 更新 `internal/tui/view.go` 的 `View()` 方法**

将 `View()` 方法体替换为按新布局组装：

```go
// View 渲染对话区、审批横幅、三行状态栏、Todo 面板、输入区与快捷键帮助。
func (m Model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}

	var parts []string
	vpContent := m.viewport.View()
	if !m.sel.empty() {
		lines := strings.Split(vpContent, "\n")
		lines = m.sel.highlightRange(lines)
		vpContent = strings.Join(lines, "\n")
	}
	parts = append(parts, messageStyle.Render(vpContent))

	if m.approval != nil {
		banner := renderApprovalBanner(*m.approval, m.contentWidth())
		parts = append(parts, statusStyle.Render(banner))
	}
	if m.lastError != "" {
		parts = append(parts, errorStyle.Render("错误: "+m.lastError))
	}

	// 三行状态栏
	if wl := renderWorkingLine(m); wl != "" {
		parts = append(parts, statusStyle.Render(wl))
	}
	parts = append(parts, statusStyle.Render(renderModeLine(m)))
	parts = append(parts, statusStyle.Render(renderDataLine(m)))

	// Todo 面板
	if m.todoArgs != "" {
		parts = append(parts, renderTodoPanel(m.todoItems))
	}

	parts = append(parts, m.textarea.View())
	parts = append(parts, helpStyle.Render(helpText))

	v := tea.NewView(joinLines(parts))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}
```

- [ ] **Step 4: 由于 `renderTodoPanel` 尚未定义，添加临时占位函数**

在 `internal/tui/view.go` 文件末尾添加：

```go
// renderTodoPanel 渲染 Todo 任务面板（临时占位，Task 6 中移至 todopanel.go）。
func renderTodoPanel(items []todoItem) string {
	if len(items) == 0 {
		return ""
	}
	var parts []string
	for _, it := range items {
		icon := todoStatusIcon(it.Status)
		parts = append(parts, fmt.Sprintf("%s %s", icon, it.Content))
	}
	return statusStyle.Render(strings.Join(parts, " · "))
}

// todoStatusIcon 返回任务状态对应的图标。
func todoStatusIcon(status string) string {
	switch status {
	case "completed":
		return "✓"
	case "in_progress":
		return "⟳"
	default:
		return "⏳"
	}
}
```

需要在 `view.go` 头部 import 中添加 `"fmt"`：

```go
import (
	"fmt"
	"strings"
	// ... 其余不变
)
```

- [ ] **Step 5: 运行测试确认通过**

```powershell
cd D:\project\coding-agent; go test ./internal/tui/ -run "TestView|TestBottomHeight" -v -count=1
```
Expected: 所有测试 PASS。

- [ ] **Step 6: Commit**

```powershell
git add internal/tui/view.go internal/tui/statusbar_test.go
git commit -m "feat(tui): update View() with three-line status bar and todo panel"
```

---

### Task 4: Git 状态异步刷新

**Files:**
- Modify: `internal/tui/model.go`（`submit()` 方法 + `event.TurnDone` 分支）
- Modify: `internal/tui/statusbar.go`（`fetchGitStatus` 命令）

**Interfaces:**
- Consumes: `gitStatus` 类型 (Task 1), `gitStatusMsg` (Task 1)
- Produces: `fetchGitStatus() tea.Cmd`

- [ ] **Step 1: 在 `statusbar.go` 末尾添加 `fetchGitStatus`**

```go
// fetchGitStatus 异步执行 git 命令获取分支和状态。
func fetchGitStatus() tea.Cmd {
	return func() tea.Msg {
		branch := runGitCmd("rev-parse", "--abbrev-ref", "HEAD")
		porcelain := runGitCmd("status", "--porcelain")
		ahead := runGitCmd("rev-list", "--count", "HEAD..@{upstream}")
		behind := runGitCmd("rev-list", "--count", "@{upstream}..HEAD")

		gs := gitStatus{
			branch: strings.TrimSpace(branch),
			dirty:  strings.TrimSpace(porcelain) != "",
		}
		// 解析 ahead/behind 计数（忽略解析错误）
		if n, err := strconv.Atoi(strings.TrimSpace(ahead)); err == nil {
			gs.ahead = n
		}
		if n, err := strconv.Atoi(strings.TrimSpace(behind)); err == nil {
			gs.behind = n
		}
		return gitStatusMsg{status: gs}
	}
}

// runGitCmd 执行 git 命令，忽略错误并返回 stdout 字符串。
func runGitCmd(args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}
```

需要在 `statusbar.go` 头部 import 中添加：
```go
import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
)
```

- [ ] **Step 2: 在 `model.go` 的 `Update()` 中处理 `gitStatusMsg`**

在 `Update` 方法 switch 的最外层 `msg.(type)` 分支中添加（放在 `balanceMsg` 和 `statuslineMsg` 之前，`event.Event` 分支之后）：

在 `case streamClosedMsg:` 之前插入：

```go
	case gitStatusMsg:
		m.gitStatus = msg.status
		return m, nil
```

- [ ] **Step 3: 在 `submit()` 和 `TurnDone` 处理中触发刷新**

修改 `submit()` 方法末尾的 `return`：
```go
	// 原: return m, tea.Batch(waitStreamEvent(ch), m.spinner.Tick)
	return m, tea.Batch(waitStreamEvent(ch), m.spinner.Tick, fetchGitStatus())
```

在 `event.TurnDone` 处理的 idle path（`m.busy = false` 之后）追加 `fetchGitStatus()`：

在 `TurnDone` 的完整路径末尾（`return m, nil` 之前约 5 行位置），将：
```go
			return m, nil
		}
```
改为返回带命令的 batch（仅应用于非 interrupt 路径）。具体修改：

找到 `case event.TurnDone:` 块末尾：
```go
			return m, nil
		}
		if m.streamCh != nil {
			return m, waitStreamEvent(m.streamCh)
		}
		return m, nil
```

修改为：
```go
			return m, tea.Batch(fetchGitStatus())
		}
		if m.streamCh != nil {
			return m, waitStreamEvent(m.streamCh)
		}
		return m, tea.Batch(fetchGitStatus())
```

- [ ] **Step 4: 编译验证**

```powershell
cd D:\project\coding-agent; go build ./internal/tui/...
```
Expected: 编译成功。

- [ ] **Step 5: Commit**

```powershell
git add internal/tui/statusbar.go internal/tui/model.go
git commit -m "feat(tui): add async git status fetch and display"
```

---

### Task 5: 余额异步刷新

**Files:**
- Modify: `internal/tui/statusbar.go`（`fetchBalance` 命令）
- Modify: `internal/tui/model.go`（`Update()` 中的 `balanceMsg` 处理 + `submit()`/`TurnDone` 触发）
- Add/Modify: `internal/agent/agent.go`（暴露 `Balance()` 方法）

**Interfaces:**
- Consumes: `balanceMsg` (Task 1), Agent 的 `Balance()` 方法
- Produces: `fetchBalance(runner Runner) tea.Cmd`

- [ ] **Step 1: 在 Agent 上添加 `Balance` 接口方法**

首先在 `internal/tui/runner.go` 中扩展 `Runner` 接口（或新建 `BalanceProvider` 可选接口）：

在 `runner.go` 文件末尾追加：

```go
// BalanceProvider 是提供余额查询的可选接口。
// Runner 可选择实现此接口；TUI 通过类型断言使用。
type BalanceProvider interface {
	Balance(ctx context.Context) (string, error)
}
```

- [ ] **Step 2: 在 `internal/agent/agent.go` 上实现 `Balance` 方法**

在 `agent.go` 中 `Agent` 的方法区域（如 `SessionDir()` 之后）添加：

```go
// Balance 查询当前 provider 的余额信息。
// 返回格式化后的余额字符串（如 "¥110.00"），错误时返回空串。
func (a *Agent) Balance(ctx context.Context) (string, error) {
	// 尝试将 provider 断言为支持余额查询的扩展接口。
	type balanceQuerier interface {
		QueryBalance(ctx context.Context) (string, error)
	}
	if bq, ok := a.prov.(balanceQuerier); ok {
		return bq.QueryBalance(ctx)
	}
	return "", nil
}
```

- [ ] **Step 3: 在 `statusbar.go` 中添加 `fetchBalance`**

在 `statusbar.go` 末尾追加：

```go
// fetchBalance 异步查询余额。
func fetchBalance(runner Runner) tea.Cmd {
	return func() tea.Msg {
		bp, ok := runner.(BalanceProvider)
		if !ok {
			return balanceMsg{}
		}
		text, err := bp.Balance(context.Background())
		if err != nil || text == "" {
			return balanceMsg{}
		}
		return balanceMsg{text: text}
	}
}
```

需要在 `statusbar.go` import 中添加 `"context"`。

- [ ] **Step 4: 在 `model.go` 中处理 `balanceMsg` + 触发刷新**

在 `Update()` 的 switch 中添加（`gitStatusMsg` 旁边）：

```go
	case balanceMsg:
		m.balance = msg.text
		return m, nil
```

在 `submit()` 的 batch 中添加（与 `fetchGitStatus()` 并列）：
```go
	return m, tea.Batch(waitStreamEvent(ch), m.spinner.Tick, fetchGitStatus(), fetchBalance(m.runner))
```

在 `TurnDone` 非中断路径的返回中添加 `fetchBalance`（与 `fetchGitStatus()` 并列）：
```go
	return m, tea.Batch(fetchGitStatus(), fetchBalance(m.runner))
```

- [ ] **Step 5: 编译验证**

```powershell
cd D:\project\coding-agent; go build ./...
```
Expected: 编译成功。

- [ ] **Step 6: Commit**

```powershell
git add internal/tui/statusbar.go internal/tui/model.go internal/tui/runner.go internal/agent/agent.go
git commit -m "feat(tui): add async balance fetch and display"
```

---

### Task 6: Todo 任务面板 — 解析与渲染

**Files:**
- Create: `internal/tui/todopanel.go`
- Create: `internal/tui/todopanel_test.go`
- Modify: `internal/tui/model.go`（`event.ToolDispatch` 处理中识别 `todo_write`）
- Remove from `internal/tui/view.go`: `renderTodoPanel` 和 `todoStatusIcon`（移入 `todopanel.go`）

**Interfaces:**
- Consumes: `todoItem`, `todoItems` (Task 1), `event.ToolDispatch`
- Produces: `renderTodoPanel(items []todoItem) string`, `parseTodoItems(rawJSON string) []todoItem`

- [ ] **Step 1: 编写 `todopanel_test.go`**

```go
package tui

import (
	"strings"
	"testing"
)

func TestParseTodoItemsEmpty(t *testing.T) {
	items := parseTodoItems("")
	if len(items) != 0 {
		t.Fatalf("parseTodoItems(\"\") = %d items, want 0", len(items))
	}
}

func TestParseTodoItemsValid(t *testing.T) {
	raw := `[{"content":"task1","status":"pending","activeForm":"task1"},{"content":"task2","status":"in_progress","activeForm":"task2"},{"content":"task3","status":"completed","activeForm":"task3"}]`
	items := parseTodoItems(raw)
	if len(items) != 3 {
		t.Fatalf("parseTodoItems = %d items, want 3", len(items))
	}
	if items[0].Content != "task1" || items[0].Status != "pending" {
		t.Fatalf("item[0] = %+v", items[0])
	}
	if items[1].Status != "in_progress" {
		t.Fatalf("item[1].Status = %q, want in_progress", items[1].Status)
	}
	if items[2].Status != "completed" {
		t.Fatalf("item[2].Status = %q, want completed", items[2].Status)
	}
}

func TestParseTodoItemsInvalidJSON(t *testing.T) {
	items := parseTodoItems(`not json`)
	if len(items) != 0 {
		t.Fatalf("parseTodoItems(invalid) = %d items, want 0", len(items))
	}
}

func TestRenderTodoPanel(t *testing.T) {
	items := []todoItem{
		{Content: "task1", Status: "pending"},
		{Content: "task2", Status: "in_progress", ActiveForm: "testing"},
		{Content: "task3", Status: "completed"},
	}
	out := renderTodoPanel(items)
	if out == "" {
		t.Fatal("renderTodoPanel returned empty")
	}
	if !strings.Contains(out, "task1") || !strings.Contains(out, "task2") || !strings.Contains(out, "task3") {
		t.Fatalf("renderTodoPanel missing tasks: %s", out)
	}
	if !strings.Contains(out, "⏳") {
		t.Fatalf("renderTodoPanel missing pending icon: %s", out)
	}
	if !strings.Contains(out, "⟳") {
		t.Fatalf("renderTodoPanel missing in_progress icon: %s", out)
	}
	if !strings.Contains(out, "✓") {
		t.Fatalf("renderTodoPanel missing completed icon: %s", out)
	}
}

func TestRenderTodoPanelEmpty(t *testing.T) {
	if out := renderTodoPanel(nil); out != "" {
		t.Fatalf("renderTodoPanel(nil) = %q, want empty", out)
	}
	if out := renderTodoPanel([]todoItem{}); out != "" {
		t.Fatalf("renderTodoPanel([]) = %q, want empty", out)
	}
}

func TestTodoStatusIcon(t *testing.T) {
	tests := []struct {
		status, want string
	}{
		{"pending", "⏳"},
		{"in_progress", "⟳"},
		{"completed", "✓"},
		{"unknown", "⏳"},
		{"", "⏳"},
	}
	for _, tc := range tests {
		got := todoStatusIcon(tc.status)
		if got != tc.want {
			t.Fatalf("todoStatusIcon(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```powershell
cd D:\project\coding-agent; go test ./internal/tui/ -run "TestParseTodoItems|TestRenderTodoPanel|TestTodoStatusIcon" -v -count=1
```
Expected: 编译失败 — `parseTodoItems`、`renderTodoPanel`、`todoStatusIcon` 未定义。

- [ ] **Step 3: 创建 `internal/tui/todopanel.go`**

```go
package tui

import (
	"encoding/json"
	"fmt"
	"strings"
)

// todoStatusIcon 返回任务状态对应的图标。
func todoStatusIcon(status string) string {
	switch status {
	case "completed":
		return "✓"
	case "in_progress":
		return "⟳"
	default:
		return "⏳"
	}
}

// parseTodoItems 解析 todo_write 的 JSON 参数，返回任务列表。
// 解析失败或 JSON 非数组时返回空切片。
func parseTodoItems(rawJSON string) []todoItem {
	rawJSON = strings.TrimSpace(rawJSON)
	if rawJSON == "" {
		return nil
	}
	var items []todoItem
	if err := json.Unmarshal([]byte(rawJSON), &items); err != nil {
		return nil
	}
	return items
}

// renderTodoPanel 渲染 Todo 任务面板。
// 空任务列表返回空字符串。
func renderTodoPanel(items []todoItem) string {
	if len(items) == 0 {
		return ""
	}
	var parts []string
	for _, it := range items {
		icon := todoStatusIcon(it.Status)
		parts = append(parts, fmt.Sprintf("%s %s", icon, it.Content))
	}
	return strings.Join(parts, " · ")
}
```

- [ ] **Step 4: 从 `view.go` 中移除临时占位函数**

删除 `internal/tui/view.go` 中文件末尾的 `renderTodoPanel` 和 `todoStatusIcon` 两个函数。

同时从 `view.go` 的 import 中移除 `"fmt"`（如果不再使用）— 检查 `View()` 方法没有使用 `fmt`。如果 `View()` 未使用 `fmt` 且两个函数已移除，则删除 `"fmt"` import。

- [ ] **Step 5: 在 `model.go` 中处理 `todo_write` 工具**

在 `event.ToolDispatch` 分支中添加 todo_write 识别逻辑。

找到 `case event.ToolDispatch:`（在 `ingestDrainEvent` 中约第 844 行位置），在 `m.pendingToolArgs = msg.ToolArgs` 之后：

```go
			// 识别 todo_write 工具并解析任务列表
			if msg.ToolName == "todo_write" {
				m.todoArgs = msg.ToolArgs
				m.todoItems = parseTodoItems(msg.ToolArgs)
			}
```

同样在 `Update` 方法的 `event.ToolDispatch` 分支（约第 139 行）做相同处理：

```go
		case event.ToolDispatch:
			if !m.busy {
				return m, nil
			}
			m.statusLabel = "running " + msg.ToolName + "..."
			m.pendingToolName = msg.ToolName
			m.pendingToolArgs = msg.ToolArgs
			// 识别 todo_write 工具并解析任务列表
			if msg.ToolName == "todo_write" {
				m.todoArgs = msg.ToolArgs
				m.todoItems = parseTodoItems(msg.ToolArgs)
			}
```

- [ ] **Step 6: 运行测试确认通过**

```powershell
cd D:\project\coding-agent; go test ./internal/tui/ -run "TestParseTodoItems|TestRenderTodoPanel|TestTodoStatusIcon" -v -count=1
```
Expected: 所有测试 PASS。

- [ ] **Step 7: Commit**

```powershell
git add internal/tui/todopanel.go internal/tui/todopanel_test.go internal/tui/view.go internal/tui/model.go
git commit -m "feat(tui): add todo panel parsing and rendering"
```

---

### Task 7: 自定义状态行

**Files:**
- Modify: `internal/tui/statusbar.go`（`runStatusline` 命令）
- Modify: `internal/tui/model.go`（处理 `statuslineMsg` + 触发刷新）
- Modify: `internal/agent/config.go`（新增 `StatuslineCommand` 配置字段）

**Interfaces:**
- Consumes: `statuslineCmd`, `statuslineOut` (Task 1), `statuslineMsg` (Task 1)
- Produces: `runStatusline(cmd string) tea.Cmd`

- [ ] **Step 1: 在 `Config` 中添加 `StatuslineCommand` 字段**

在 `internal/agent/config.go` 中 `SessionDir` 字段之后追加：

```go
	// StatuslineCommand 自定义状态行命令。
	// 非空时在每个 turn 完成后执行该命令，stdout 首行替换内置数据行。
	// 命令接收 JSON 上下文（含 model、branch、context_used 等）作为 stdin。
	StatuslineCommand string
```

在 `model.go` 的 `Model` 结构体中，`statuslineCmd` 字段已存在于 Task 1，无需额外操作。

- [ ] **Step 2: 将配置传递到 TUI Model**

找到 TUI 初始化的调用链。在 `NewWithRunner` 或构造 Model 时设置 `statuslineCmd`。

检查代码库中实例化 `tui.Model` 的位置：

```powershell
cd D:\project\coding-agent; grep -rn "tui\.New" --include="*.go"
```

假设是 `cmd/` 目录下的入口文件。在其中添加：

在构造 Model 后：
```go
m.statuslineCmd = cfg.StatuslineCommand
```

- [ ] **Step 3: 在 `statusbar.go` 中添加 `runStatusline`**

```go
// runStatusline 执行自定义状态行命令，stdin 传入 ctxJSON，捕获 stdout 首行。
func runStatusline(cmd, ctxJSON string) tea.Cmd {
	return func() tea.Msg {
		if cmd == "" {
			return statuslineMsg{}
		}
		c := exec.Command("sh", "-c", cmd)
		c.Stdin = strings.NewReader(ctxJSON)
		c.Stderr = nil
		out, err := c.Output()
		if err != nil {
			return statuslineMsg{}
		}
		firstLine := strings.SplitN(string(out), "\n", 2)[0]
		return statuslineMsg{out: strings.TrimSpace(firstLine)}
	}
}
```

- [ ] **Step 4: 修改 `renderDataLine` 支持自定义状态行覆盖**

在 `renderDataLine` 函数开头添加：

```go
	// 自定义状态行覆盖
	if m.statuslineOut != "" {
		return m.statuslineOut
	}
```

完整函数变为：
```go
func renderDataLine(m Model) string {
	// 自定义状态行覆盖
	if m.statuslineOut != "" {
		return m.statuslineOut
	}

	var parts []string

	if gauge := renderContextGauge(m.contextUsed, m.contextWindow); gauge != "" {
		parts = append(parts, gauge)
	}

	if m.cacheHitRate > 0 {
		parts = append(parts, fmt.Sprintf("cache %d%%", m.cacheHitRate))
	}

	if m.balance != "" {
		parts = append(parts, m.balance)
	}

	return joinWithSep(parts, " · ")
}
```

- [ ] **Step 5: 在 `model.go` 中处理 `statuslineMsg` + 触发刷新**

在 `Update()` 的 switch 中添加：

```go
	case statuslineMsg:
		m.statuslineOut = msg.out
		return m, nil
```

在 `submit()` 和 `TurnDone` 的 batch 中添加 `runStatusline` 调用（仅当 `m.statuslineCmd != ""` 时）。

由于 `submit()` 的 batch 已包含 `fetchGitStatus()` 和 `fetchBalance()`，这里追加一个条件式命令。将 `submit()` 末尾的 return 改为预先构建 batch 数组：

```go
	cmds := []tea.Cmd{waitStreamEvent(ch), m.spinner.Tick, fetchGitStatus(), fetchBalance(m.runner)}
	if m.statuslineCmd != "" {
		cmds = append(cmds, runStatusline(m.statuslineCmd, ""))
	}
	return m, tea.Batch(cmds...)
```

类似地在 `TurnDone` 非中断路径的 batch 中添加。

- [ ] **Step 6: 编译并运行测试**

```powershell
cd D:\project\coding-agent; go build ./...
cd D:\project\coding-agent; go test ./internal/tui/ -v -count=1
```
Expected: 编译成功，现有测试 PASS。

- [ ] **Step 7: Commit**

```powershell
git add internal/tui/statusbar.go internal/tui/model.go internal/agent/config.go
git commit -m "feat(tui): add custom statusline command support"
```

---

### Task 8: 上下文窗口快照刷新

**Files:**
- Modify: `internal/agent/agent.go`（添加 `ContextSnapshot` 方法）
- Modify: `internal/tui/runner.go`（添加 `ContextSnapshotProvider` 接口）
- Modify: `internal/tui/model.go`（`TurnDone` 后刷新 contextUsed）

**Interfaces:**
- Consumes: Agent 的 `lastPromptTokens` + `contextWindow`
- Produces: `ContextSnapshotProvider` 接口 → `fetchContextSnapshot` → 更新 `m.contextUsed`、`m.contextWindow`

- [ ] **Step 1: 在 `agent.go` 上添加 `ContextSnapshot` 方法**

在 `Agent` 上（`ContextStats` 方法附近）：

```go
// ContextSnapshot 返回当前上下文用量快照（已用 tokens，窗口上限）。
func (a *Agent) ContextSnapshot() (used int, window int) {
	return a.lastPromptTokens, a.contextWindow
}
```

- [ ] **Step 2: 在 `runner.go` 中添加 `ContextSnapshotProvider` 接口**

```go
// ContextSnapshotProvider 提供上下文窗口用量查询。
type ContextSnapshotProvider interface {
	ContextSnapshot() (used int, window int)
}
```

- [ ] **Step 3: 在 `model.go` 的 `TurnDone` 中刷新上下文快照**

在 `TurnDone` 处理中，`m.busy = false` 之后、return 之前添加：

```go
	// 刷新上下文快照
	if csp, ok := m.runner.(ContextSnapshotProvider); ok {
		m.contextUsed, m.contextWindow = csp.ContextSnapshot()
	}
```

- [ ] **Step 4: 编译验证并运行全量测试**

```powershell
cd D:\project\coding-agent; go build ./...
cd D:\project\coding-agent; go test ./internal/tui/... -v -count=1
```
Expected: 编译成功，所有测试 PASS。

- [ ] **Step 5: Commit**

```powershell
git add internal/agent/agent.go internal/tui/runner.go internal/tui/model.go
git commit -m "feat(tui): add context window snapshot refresh after each turn"
```

---

### Task 9: 缓存命中率显示

**Files:**
- Modify: `internal/tui/model.go`（`TurnDone` 后刷新 `cacheHitRate`）

**Interfaces:**
- Consumes: 现有的 usage 统计
- Produces: `cacheHitRate` 字段的计算与更新

> **注意:** 由于当前 provider 接口没有显式的缓存命中率 API，此任务的实现方案是：在每个 turn 完成后从 provider 的 usage 统计中推导缓存命中率（若 provider 支持 `X-Cache-Hit` 等响应头）。如果 provider 不支持，`cacheHitRate` 保持为 0，数据行不显示缓存率信息。后期可扩展 provider 接口暴露独立方法。

- [ ] **Step 1: 在 `runner.go` 中添加 `CacheHitRateProvider` 接口（可选）**

```go
// CacheHitRateProvider 提供缓存命中率查询（0-100）。
type CacheHitRateProvider interface {
	CacheHitRate() int
}
```

- [ ] **Step 2: 在 `model.go` 的 `TurnDone` 中刷新**

在上下文快照刷新代码之后添加：

```go
	// 刷新缓存命中率
	if chp, ok := m.runner.(CacheHitRateProvider); ok {
		m.cacheHitRate = chp.CacheHitRate()
	}
```

- [ ] **Step 3: 编译验证**

```powershell
cd D:\project\coding-agent; go build ./...
go test ./internal/tui/... -count=1
```
Expected: 编译成功，所有测试 PASS。

- [ ] **Step 4: Commit**

```powershell
git add internal/tui/runner.go internal/tui/model.go
git commit -m "feat(tui): add cache hit rate display wiring"
```

---

### Task 10: 集成测试与回归验证

**Files:**
- Modify: `internal/tui/integration_test.go`（若有需要更新）
- 运行: `internal/tui/*_test.go`（全量）

**Interfaces:**
- 不新增，仅验证现有全部测试通过。

- [ ] **Step 1: 更新可能受影响的旧测试**

检查 `integration_test.go` 中是否有引用已删除的 `renderStatusBar` 符号：

```powershell
cd D:\project\coding-agent; grep -rn "renderStatusBar" internal/tui/
```
Expected: 无引用（已在 Task 2 中替换为 `renderWorkingLine`/`renderModeLine`/`renderDataLine`）。

如果 `integration_test.go` 中有 `renderStatusBar` 引用，将其更新为新的渲染函数调用。

- [ ] **Step 2: 运行全量测试套件**

```powershell
cd D:\project\coding-agent; go test ./internal/tui/... -v -count=1
```
Expected: 所有测试 PASS。

- [ ] **Step 3: 运行 race 检测**

```powershell
cd D:\project\coding-agent; go test -race ./internal/tui/... -count=1
```
Expected: 无 race 警告，所有测试 PASS。

- [ ] **Step 4: 构建全项目**

```powershell
cd D:\project\coding-agent; go build ./...
```
Expected: 编译成功，无错误。

- [ ] **Step 5: Commit**

```powershell
git add -A
git commit -m "test(tui): update integration tests for new status panels"
```

---

## 文件变更总览

| 文件 | 变更类型 | 涉及任务 |
|------|---------|---------|
| `internal/tui/model.go` | 修改：新增字段 + 消息类型 + Update 处理 | Task 1, 4, 5, 6, 7, 8, 9 |
| `internal/tui/statusbar.go` | 重写：三行布局 + context gauge + git + balance + statusline | Task 2, 4, 5, 7 |
| `internal/tui/statusbar_test.go` | 重写：覆盖新渲染函数 | Task 2, 3 |
| `internal/tui/view.go` | 修改：bottomHeight + View 组装 | Task 3, 6 |
| `internal/tui/todopanel.go` | 新增：Todo 面板解析与渲染 | Task 6 |
| `internal/tui/todopanel_test.go` | 新增：Todo 面板测试 | Task 6 |
| `internal/tui/runner.go` | 修改：新增 BalanceProvider / ContextSnapshotProvider / CacheHitRateProvider 接口 | Task 5, 8, 9 |
| `internal/agent/agent.go` | 修改：新增 Balance() / ContextSnapshot() 方法 | Task 5, 8 |
| `internal/agent/config.go` | 修改：新增 StatuslineCommand 字段 | Task 7 |

## 执行顺序依赖

```
Task 1 (Model fields)
 └─ Task 2 (Statusbar rewrite)
     └─ Task 3 (View integration)
         ├─ Task 4 (Git status) ─┐
         ├─ Task 5 (Balance)   ─┤
         ├─ Task 6 (Todo panel) ─┤
         ├─ Task 7 (Statusline) ─┤
         ├─ Task 8 (Context)   ─┤
         └─ Task 9 (Cache)     ─┘
              └─ Task 10 (Regression tests)
```

Tasks 4-9 可并行执行（均依赖 Task 3，互不依赖）。
