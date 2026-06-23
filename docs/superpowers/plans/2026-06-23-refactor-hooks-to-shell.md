---
change: refactor-hooks-to-shell
design-doc: docs/superpowers/specs/2026-06-23-refactor-hooks-to-shell-design.md
base-ref: d53a6aba309b446bb7a13b0671399a1ee71b8619
---

# Shell Hook Engine 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: 使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 按任务逐步实施。步骤使用 checkbox（`- [ ]`）语法跟踪进度。

**Goal:** 将 hook 系统从进程内 Go 回调（`hooks.Registry`）改造为外部 shell 命令模式，Agent 通过 `ToolHooks` interface 解耦，配置文件声明 hook，运行时 spawn 子进程并通过 stdin JSON + exit code 通信。

**Architecture:** **D1** 在 `internal/agent/hooks.go` 定义 `ToolHooks` interface 与 `SubsetHooks` 包装器；**D2** 两级 JSON 配置（项目 `.coding-agent/hooks.json` 优先于全局 `~/.coding-agent/hooks.json`）由 `hooks.Load()` 解析为 `[]ResolvedHook`；**D3** `DefaultSpawner` + `Run()` 执行引擎按 exit code 决策；**D4** `Runner` 实现 `ToolHooks` 作为 CLI 装配门面；**D5** TodoGuard 逻辑内联到 agent 主循环 Stop 判断（先于外部 hook）；**D6** 移除全部 builtin hook；**D7** 各事件通过统一 `Payload` JSON 经 stdin 传给外部命令。

**Tech Stack:** Go 1.26、`os/exec`、`encoding/json`、`regexp`、平台检测（Unix `sh -c` / Windows Git Bash 或 `cmd /c`）

**参考实现:** DeepSeek-Reasonix `D:\project\DeepSeek-Reasonix`

## Global Constraints

- Agent 包 **不得** import `internal/hooks`（**D1**）；`ToolHooks` 定义在 `internal/agent/hooks.go`。
- 保留 4 个 hook 点位：UserPromptSubmit、PreToolUse、PostToolUse、Stop；不新增 Reasonix 额外事件。
- 阻塞型事件（PreToolUse、UserPromptSubmit）：exit 2 或超时 → block；非阻塞型（PostToolUse、Stop）：exit 2 / 其它非零 / 超时 → warn（**D3**）。
- Stop 特殊语义：exit 2 + stdout 非空 → `Report.Force = stdout`（**D3**）。
- 默认 hook 超时 10000ms；JSON 解析失败 log warning 并跳过该文件（**D2**）。
- PreToolUse 流程不变式：hook 不阻断时仍走 `permission.Checker`（设计 doc §1 安全不变式）。
- **BREAKING（D6）**：LogHook / LargeOutputHook / ContextInjectHook / SummaryHook 移除；用户需迁移为外部脚本。
- TodoGuard 内联后不受外部 hook 失败影响，且 **先于** 外部 Stop hook 执行（**D5**）。
- Subagent 仅继承 PreToolUse + PostToolUse，通过 `NewSubsetHooks`（**D1**）。
- 每个 task 完成后：`tasks.md` 打勾 → `git commit`（不得积攒）。
- 验证命令：`go build ./...`、`go test ./... -count=1`。

## 设计决策索引

| 决策 | 内容 | 本计划对应 Task |
|------|------|----------------|
| **D1** | `ToolHooks` interface + `SubsetHooks` 包装器，Agent 不 import hooks 包 | Task 1.1, 4.1, 4.6 |
| **D2** | 两级 JSON 配置，项目级优先，Load 合并 | Task 1.2, 1.3, 1.4 |
| **D3** | Spawner + exit code 决策 + 超时 kill | Task 2.1, 2.2, 2.3, 2.4 |
| **D4** | `Runner` 实现 `ToolHooks`，CLI 装配 | Task 3.1, 3.2, 5.1, 5.2 |
| **D5** | TodoGuard 内联到 `loopStepWithText` Stop 判断 | Task 4.4 |
| **D6** | 移除全部 builtin hook | Task 6.1 |
| **D7** | 统一 JSON Payload 经 stdin 传递 | Task 1.2, 2.2, 3.2 |

## 目标文件结构

```
internal/agent/
├── hooks.go           # ToolHooks interface + SubsetHooks（D1，新建）
├── todo_guard.go      # checkTodoGuard 内联逻辑（D5，新建）
├── option.go          # WithHooks(ToolHooks)（修改）
├── agent.go           # hooks 字段类型 + 触发点 + WireTaskTool（修改）
├── loop.go            # Pre/Post/Stop 触发点 + TodoGuard 顺序（修改）
└── subagent.go        # SubagentOptions.Hooks ToolHooks（修改）

internal/hooks/
├── hook.go            # Event/HookConfig/Payload/Decision 等核心类型（D2/D7，新建）
├── load.go            # Load() 配置加载（D2，新建）
├── load_test.go
├── spawner.go         # DefaultSpawner（D3，新建）
├── spawner_test.go
├── run.go             # Run() + decideOutcome()（D3，新建）
├── run_test.go
├── runner.go          # Runner 门面（D4，新建）
├── runner_test.go
├── context.go         # WithSubagentFlag / IsSubagent（从 hooks.go 迁出，新建）
└── hooks_test.go      # 重写（删除 Registry 测试）

internal/hooks/builtin/   # 整目录删除（D6）

cmd/cli/
├── once.go            # hooks.Load + NewRunner（D4）
├── chat_setup.go      # 同上
└── chat.go            # /hooks 命令适配（修改）
```

---

## 第 1 组：核心类型与接口定义

### Task 1.1: 定义 ToolHooks interface 与 SubsetHooks（D1）

**依赖:** 无

**Files:**
- Create: `internal/agent/hooks.go`

**Interfaces:**
- Produces: `ToolHooks` interface；`SubsetHooks` struct；`NewSubsetHooks(h ToolHooks) ToolHooks`

**变更描述:** 在 agent 包内定义 hook 调用契约，Subagent 通过 `SubsetHooks` 屏蔽 UserPromptSubmit / Stop。

- [ ] **Step 1: 创建 `internal/agent/hooks.go`**

```go
package agent

import (
	"context"

	"github.com/wsx864321/coding-agent/internal/provider"
)

// ToolHooks 是 Agent 与 hook 实现之间的解耦边界（D1）。
type ToolHooks interface {
	UserPromptSubmit(ctx context.Context, content string) error
	PreToolUse(ctx context.Context, name string, args map[string]any) (block bool, message string)
	PostToolUse(ctx context.Context, name string, args map[string]any, result string)
	Stop(ctx context.Context, messages []provider.Message) (force string, ok bool)
}

// SubsetHooks 仅转发 PreToolUse / PostToolUse，供 subagent 使用（D1）。
type SubsetHooks struct {
	inner ToolHooks
}

func (s *SubsetHooks) UserPromptSubmit(_ context.Context, _ string) error { return nil }

func (s *SubsetHooks) PreToolUse(ctx context.Context, name string, args map[string]any) (bool, string) {
	return s.inner.PreToolUse(ctx, name, args)
}

func (s *SubsetHooks) PostToolUse(ctx context.Context, name string, args map[string]any, result string) {
	s.inner.PostToolUse(ctx, name, args, result)
}

func (s *SubsetHooks) Stop(_ context.Context, _ []provider.Message) (string, bool) { return "", false }

// NewSubsetHooks 构造 subagent 专用 hook 视图。
func NewSubsetHooks(h ToolHooks) ToolHooks {
	if h == nil {
		return nil
	}
	return &SubsetHooks{inner: h}
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./internal/agent/...`
Expected: PASS（此时 agent 仍使用旧 `*hooks.Registry`，本文件独立可编译）

- [ ] **Step 3: Commit**

```bash
git add internal/agent/hooks.go
git commit -m "feat(agent): add ToolHooks interface and SubsetHooks wrapper"
```

---

### Task 1.2: 定义 hooks 包核心类型（D2, D7）

**依赖:** 无（可与 Task 1.1 并行）

**Files:**
- Create: `internal/hooks/hook.go`
- Create: `internal/hooks/context.go`（从现有 `hooks.go` 迁出 subagent 辅助函数）

**Interfaces:**
- Produces: `Event`, `HookConfig`, `Settings`, `Scope`, `ResolvedHook`, `Payload`, `SpawnInput`, `SpawnResult`, `Spawner`, `Decision`, `Outcome`, `Report`

**变更描述:** 新建类型文件，暂不删除旧 `hooks.go`（Registry 在 Task 6.2 移除，保证中间态可编译）。

- [ ] **Step 1: 创建 `internal/hooks/context.go`**

```go
package hooks

import "context"

type subagentCtxKey struct{}

func WithSubagentFlag(ctx context.Context) context.Context {
	return context.WithValue(ctx, subagentCtxKey{}, true)
}

func IsSubagent(ctx context.Context) bool {
	v, _ := ctx.Value(subagentCtxKey{}).(bool)
	return v
}
```

- [ ] **Step 2: 创建 `internal/hooks/hook.go`**

```go
package hooks

import "time"

type Event string

const (
	EventPreToolUse       Event = "PreToolUse"
	EventPostToolUse      Event = "PostToolUse"
	EventUserPromptSubmit Event = "UserPromptSubmit"
	EventStop             Event = "Stop"
)

type HookConfig struct {
	Match       string `json:"match,omitempty"`
	Command     string `json:"command"`
	Description string `json:"description,omitempty"`
	Timeout     int    `json:"timeout,omitempty"` // ms, default 10000
	Cwd         string `json:"cwd,omitempty"`
}

type Settings struct {
	Hooks map[Event][]HookConfig `json:"hooks"`
}

type Scope string

const (
	ScopeProject Scope = "project"
	ScopeGlobal  Scope = "global"
)

type ResolvedHook struct {
	HookConfig
	Event  Event
	Scope  Scope
	Source string // settings file absolute path
}

// Payload 是 stdin 传给外部 hook 命令的 JSON（D7）。
type Payload struct {
	Event      Event          `json:"event"`
	Cwd        string         `json:"cwd"`
	ToolName   string         `json:"toolName,omitempty"`
	ToolArgs   map[string]any `json:"toolArgs,omitempty"`
	ToolResult string         `json:"toolResult,omitempty"`
	Prompt     string         `json:"prompt,omitempty"`
}

type SpawnInput struct {
	Command string
	Cwd     string
	Stdin   string
	Timeout time.Duration
}

type SpawnResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
	TimedOut bool
	Err      error
}

type Spawner func(ctx context.Context, in SpawnInput) SpawnResult

type Decision string

const (
	DecisionPass  Decision = "pass"
	DecisionBlock Decision = "block"
	DecisionWarn  Decision = "warn"
	DecisionError Decision = "error"
)

type Outcome struct {
	Hook     ResolvedHook
	Decision Decision
	ExitCode int
	Stdout   string
	Stderr   string
	TimedOut bool
	Duration time.Duration
}

type Report struct {
	Event    Event
	Outcomes []Outcome
	Blocked  bool
	Force    string // Stop 事件专用
}
```

- [ ] **Step 3: 验证编译**

Run: `go build ./internal/hooks/...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/hooks/hook.go internal/hooks/context.go
git commit -m "feat(hooks): add shell hook core types and subagent context helpers"
```

---

### Task 1.3: 实现 Load() 配置加载（D2）

**依赖:** Task 1.2

**Files:**
- Create: `internal/hooks/load.go`

**Interfaces:**
- Consumes: `HookConfig`, `Settings`, `Scope`, `ResolvedHook`, `Event`（Task 1.2）
- Produces: `LoadOptions`, `Load(opts LoadOptions) []ResolvedHook`

**变更描述:** 按项目 → 全局顺序加载 JSON；解析失败 log warning 跳过；合并为 `[]ResolvedHook`（项目级在前）。

- [ ] **Step 1: 实现 `internal/hooks/load.go`**

```go
package hooks

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

const defaultHookTimeoutMs = 10000

type LoadOptions struct {
	ProjectRoot string
	HomeDir     string // defaults to os.UserHomeDir()
}

func Load(opts LoadOptions) []ResolvedHook {
	home := opts.HomeDir
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			log.Printf("[hooks] 无法获取用户目录: %v", err)
			return nil
		}
	}

	var out []ResolvedHook
	if opts.ProjectRoot != "" {
		p := filepath.Join(opts.ProjectRoot, ".coding-agent", "hooks.json")
		out = append(out, loadFile(p, ScopeProject)...)
	}
	globalPath := filepath.Join(home, ".coding-agent", "hooks.json")
	out = append(out, loadFile(globalPath, ScopeGlobal)...)
	return out
}

func loadFile(path string, scope Scope) []ResolvedHook {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[hooks] 读取 %s 失败: %v", path, err)
		}
		return nil
	}
	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		log.Printf("[hooks] 解析 %s 失败: %v", path, err)
		return nil
	}
	abs, _ := filepath.Abs(path)
	var resolved []ResolvedHook
	for event, configs := range settings.Hooks {
		for _, cfg := range configs {
			if cfg.Command == "" {
				continue
			}
			if cfg.Timeout <= 0 {
				cfg.Timeout = defaultHookTimeoutMs
			}
			resolved = append(resolved, ResolvedHook{
				HookConfig: cfg,
				Event:      event,
				Scope:      scope,
				Source:     abs,
			})
		}
	}
	return resolved
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./internal/hooks/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/hooks/load.go
git commit -m "feat(hooks): implement JSON config Load for project and global scopes"
```

---

### Task 1.4: Load 单元测试（D2）

**依赖:** Task 1.3

**Files:**
- Create: `internal/hooks/load_test.go`

**Interfaces:**
- Consumes: `Load()`, `LoadOptions`（Task 1.3）

- [ ] **Step 1: 编写失败测试**

```go
package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

func writeHooksJSON(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_ProjectAndGlobalMerge(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()

	writeHooksJSON(t, root, ".coding-agent/hooks.json", `{
	  "hooks": {
	    "PreToolUse": [{"command": "echo project", "match": "bash"}]
	  }
	}`)
	writeHooksJSON(t, home, ".coding-agent/hooks.json", `{
	  "hooks": {
	    "Stop": [{"command": "echo global"}]
	  }
	}`)

	got := Load(LoadOptions{ProjectRoot: root, HomeDir: home})
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got[0].Scope != ScopeProject || got[0].Command != "echo project" {
		t.Errorf("project hook: %+v", got[0])
	}
	if got[1].Scope != ScopeGlobal || got[1].Event != EventStop {
		t.Errorf("global hook: %+v", got[1])
	}
}

func TestLoad_GlobalOnly(t *testing.T) {
	home := t.TempDir()
	writeHooksJSON(t, home, ".coding-agent/hooks.json", `{
	  "hooks": {"PostToolUse": [{"command": "cat"}]}
	}`)
	got := Load(LoadOptions{ProjectRoot: t.TempDir(), HomeDir: home})
	if len(got) != 1 || got[0].Scope != ScopeGlobal {
		t.Fatalf("got=%+v", got)
	}
}

func TestLoad_NoConfig(t *testing.T) {
	got := Load(LoadOptions{ProjectRoot: t.TempDir(), HomeDir: t.TempDir()})
	if len(got) != 0 {
		t.Fatalf("expected empty, got %+v", got)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	root := t.TempDir()
	writeHooksJSON(t, root, ".coding-agent/hooks.json", `{invalid`)
	got := Load(LoadOptions{ProjectRoot: root, HomeDir: t.TempDir()})
	if len(got) != 0 {
		t.Fatalf("expected skip on bad JSON, got %+v", got)
	}
}

func TestLoad_DefaultTimeout(t *testing.T) {
	root := t.TempDir()
	writeHooksJSON(t, root, ".coding-agent/hooks.json", `{
	  "hooks": {"PreToolUse": [{"command": "true"}]}
	}`)
	got := Load(LoadOptions{ProjectRoot: root, HomeDir: t.TempDir()})
	if len(got) != 1 || got[0].Timeout != defaultHookTimeoutMs {
		t.Fatalf("timeout=%d, want %d", got[0].Timeout, defaultHookTimeoutMs)
	}
}
```

- [ ] **Step 2: 运行测试**

Run: `go test ./internal/hooks/ -run TestLoad -count=1 -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/hooks/load_test.go
git commit -m "test(hooks): add Load config merge and error handling tests"
```

---

## 第 2 组：Spawner 与执行引擎

### Task 2.1: 实现 DefaultSpawner（D3）

**依赖:** Task 1.2

**Files:**
- Create: `internal/hooks/spawner.go`
- Create: `internal/hooks/spawner_test.go`

**Interfaces:**
- Consumes: `SpawnInput`, `SpawnResult`, `Spawner`（Task 1.2）
- Produces: `DefaultSpawner(ctx, in) SpawnResult`；`shellCommand(command string) (name string, args []string)`

- [ ] **Step 1: 编写 spawner 测试**

```go
func TestDefaultSpawner_ExitCodeAndStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("platform-specific shell quoting covered in integration tests")
	}
	res := DefaultSpawner(context.Background(), SpawnInput{
		Command: `printf '%s' ok`,
		Timeout: 5 * time.Second,
	})
	if res.ExitCode != 0 || res.Stdout != "ok" {
		t.Fatalf("res=%+v", res)
	}
}

func TestDefaultSpawner_Stdin(t *testing.T) {
	if runtime.GOS == "windows" {
		t.Skip("platform-specific")
	}
	payload := `{"event":"PreToolUse"}`
	res := DefaultSpawner(context.Background(), SpawnInput{
		Command: `cat`,
		Stdin:   payload,
		Timeout: 5 * time.Second,
	})
	if res.Stdout != payload {
		t.Fatalf("stdout=%q", res.Stdout)
	}
}

func TestDefaultSpawner_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("platform-specific")
	}
	res := DefaultSpawner(context.Background(), SpawnInput{
		Command: `sleep 2`,
		Timeout: 50 * time.Millisecond,
	})
	if !res.TimedOut {
		t.Fatalf("expected timeout, got %+v", res)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/hooks/ -run TestDefaultSpawner -count=1 -v`
Expected: FAIL（`DefaultSpawner` 未定义）

- [ ] **Step 3: 实现 `internal/hooks/spawner.go`**

```go
package hooks

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func DefaultSpawner(ctx context.Context, in SpawnInput) SpawnResult {
	if in.Timeout <= 0 {
		in.Timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, in.Timeout)
	defer cancel()

	name, args := shellCommand(in.Command)
	cmd := exec.CommandContext(ctx, name, args...)
	if in.Cwd != "" {
		cmd.Dir = in.Cwd
	}
	cmd.Stdin = strings.NewReader(in.Stdin)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := SpawnResult{
		Stdout: strings.TrimSpace(stdout.String()),
		Stderr: strings.TrimSpace(stderr.String()),
	}
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			res.TimedOut = true
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		} else if res.TimedOut {
			res.ExitCode = -1
		} else {
			res.Err = err
		}
	}
	return res
}

func shellCommand(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("sh"); err == nil {
			return "sh", []string{"-c", command}
		}
		return "cmd", []string{"/c", command}
	}
	return "sh", []string{"-c", command}
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/hooks/ -run TestDefaultSpawner -count=1 -v`
Expected: PASS（Unix）；Windows 上 skip 项不影响

- [ ] **Step 5: Commit**

```bash
git add internal/hooks/spawner.go internal/hooks/spawner_test.go
git commit -m "feat(hooks): add DefaultSpawner with timeout and platform shell detection"
```

---

### Task 2.2: 实现 Run() 与 decideOutcome()（D3, D7）

**依赖:** Task 1.2, Task 2.1

**Files:**
- Create: `internal/hooks/run.go`

**Interfaces:**
- Consumes: `Payload`, `ResolvedHook`, `Spawner`, `Report`, `Outcome`, `Decision`, `Event`（Task 1.2）；`DefaultSpawner`（Task 2.1）
- Produces: `Run(ctx, payload, hooks, spawner) Report`；`decideOutcome(event, res) Decision`；`isBlockingEvent(event) bool`；`matchesHook(h ResolvedHook, payload Payload) bool`

**变更描述:** 按 event 过滤 hook；Pre/PostToolUse 额外按 `match` 正则过滤（空 match = 全匹配）；阻塞型事件首个 block 短路；Stop exit 2 + stdout → Force。

- [ ] **Step 1: 实现 `internal/hooks/run.go`**

```go
package hooks

import (
	"context"
	"encoding/json"
	"regexp"
	"time"
)

func Run(ctx context.Context, payload Payload, hooks []ResolvedHook, spawner Spawner) Report {
	rep := Report{Event: payload.Event}
	blocking := isBlockingEvent(payload.Event)

	for _, h := range hooks {
		if h.Event != payload.Event {
			continue
		}
		if !matchesHook(h, payload) {
			continue
		}

		body, _ := json.Marshal(payload)
		cwd := h.Cwd
		if cwd == "" {
			cwd = payload.Cwd
		}
		start := time.Now()
		res := spawner(ctx, SpawnInput{
			Command: h.Command,
			Cwd:     cwd,
			Stdin:   string(body),
			Timeout: time.Duration(h.Timeout) * time.Millisecond,
		})
		decision := decideOutcome(payload.Event, res)
		out := Outcome{
			Hook:     h,
			Decision: decision,
			ExitCode: res.ExitCode,
			Stdout:   res.Stdout,
			Stderr:   res.Stderr,
			TimedOut: res.TimedOut,
			Duration: time.Since(start),
		}
		rep.Outcomes = append(rep.Outcomes, out)

		if payload.Event == EventStop && res.ExitCode == 2 && res.Stdout != "" {
			rep.Force = res.Stdout
		}
		if decision == DecisionBlock {
			rep.Blocked = true
			if blocking {
				break
			}
		}
	}
	return rep
}

func isBlockingEvent(e Event) bool {
	return e == EventPreToolUse || e == EventUserPromptSubmit
}

func matchesHook(h ResolvedHook, p Payload) bool {
	if h.Match == "" {
		return true
	}
	if p.Event != EventPreToolUse && p.Event != EventPostToolUse {
		return true
	}
	re, err := regexp.Compile(h.Match)
	if err != nil {
		return false
	}
	return re.MatchString(p.ToolName)
}

func decideOutcome(event Event, res SpawnResult) Decision {
	blocking := isBlockingEvent(event)
	if res.TimedOut {
		if blocking {
			return DecisionBlock
		}
		return DecisionWarn
	}
	if res.ExitCode == 0 {
		return DecisionPass
	}
	if res.ExitCode == 2 {
		if blocking {
			return DecisionBlock
		}
		return DecisionWarn
	}
	return DecisionWarn
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./internal/hooks/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/hooks/run.go
git commit -m "feat(hooks): implement Run engine with exit-code decisions and short-circuit"
```

---

### Task 2.3: Run 单元测试（D3）

**依赖:** Task 2.2

**Files:**
- Create: `internal/hooks/run_test.go`

- [ ] **Step 1: 编写 mock spawner 与测试用例**

```go
package hooks

import (
	"context"
	"strings"
	"testing"
	"time"
)

func mockSpawner(responses map[string]SpawnResult) Spawner {
	return func(_ context.Context, in SpawnInput) SpawnResult {
		for key, res := range responses {
			if strings.Contains(in.Command, key) {
				return res
			}
		}
		return SpawnResult{ExitCode: 0}
	}
}

func TestRun_PreToolUse_BlockShortCircuit(t *testing.T) {
	hooks := []ResolvedHook{{
		Event: EventPreToolUse, HookConfig: HookConfig{Command: "block-hook", Match: "bash"},
	}, {
		Event: EventPreToolUse, HookConfig: HookConfig{Command: "second-hook"},
	}}
	sp := mockSpawner(map[string]SpawnResult{
		"block-hook": {ExitCode: 2, Stderr: "denied"},
		"second-hook": {ExitCode: 0},
	})
	rep := Run(context.Background(), Payload{
		Event: EventPreToolUse, Cwd: "/tmp", ToolName: "bash", ToolArgs: map[string]any{"command": "rm"},
	}, hooks, sp)
	if !rep.Blocked || len(rep.Outcomes) != 1 {
		t.Fatalf("rep=%+v", rep)
	}
}

func TestRun_PreToolUse_MatchFilter(t *testing.T) {
	hooks := []ResolvedHook{{
		Event: EventPreToolUse, HookConfig: HookConfig{Command: "only-bash", Match: "^bash$"},
	}}
	called := false
	sp := Spawner(func(_ context.Context, in SpawnInput) SpawnResult {
		called = true
		return SpawnResult{ExitCode: 0}
	})
	Run(context.Background(), Payload{Event: EventPreToolUse, ToolName: "read_file"}, hooks, sp)
	if called {
		t.Fatal("match should filter out non-bash tool")
	}
}

func TestRun_Stop_ForceFromStdout(t *testing.T) {
	hooks := []ResolvedHook{{
		Event: EventStop, HookConfig: HookConfig{Command: "stop-hook"},
	}}
	sp := mockSpawner(map[string]SpawnResult{
		"stop-hook": {ExitCode: 2, Stdout: "请继续完成待办"},
	})
	rep := Run(context.Background(), Payload{Event: EventStop, Cwd: "/tmp"}, hooks, sp)
	if rep.Force != "请继续完成待办" {
		t.Fatalf("force=%q", rep.Force)
	}
}

func TestRun_PostToolUse_NonBlockingWarn(t *testing.T) {
	hooks := []ResolvedHook{{
		Event: EventPostToolUse, HookConfig: HookConfig{Command: "warn-hook"},
	}}
	sp := mockSpawner(map[string]SpawnResult{"warn-hook": {ExitCode: 2}})
	rep := Run(context.Background(), Payload{Event: EventPostToolUse, ToolName: "bash"}, hooks, sp)
	if rep.Blocked {
		t.Fatal("PostToolUse exit 2 should warn, not block")
	}
}

func TestRun_Timeout_BlockOnPreToolUse(t *testing.T) {
	hooks := []ResolvedHook{{
		Event: EventPreToolUse, HookConfig: HookConfig{Command: "slow", Timeout: 1},
	}}
	sp := Spawner(func(context.Context, SpawnInput) SpawnResult {
		return SpawnResult{TimedOut: true, ExitCode: -1}
	})
	rep := Run(context.Background(), Payload{Event: EventPreToolUse, ToolName: "bash"}, hooks, sp)
	if !rep.Blocked {
		t.Fatal("timeout on blocking event should block")
	}
}
```

- [ ] **Step 2: 运行测试**

Run: `go test ./internal/hooks/ -run TestRun -count=1 -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/hooks/run_test.go
git commit -m "test(hooks): add Run engine decision and short-circuit tests"
```

---

## 第 3 组：Runner 门面

### Task 3.1: 实现 Runner struct（D4）

**依赖:** Task 2.2

**Files:**
- Create: `internal/hooks/runner.go`

**Interfaces:**
- Consumes: `Run()`, `Payload`, `ResolvedHook`, `Spawner`（Task 2.2）
- Produces: `Runner` struct；`NewRunner(hooks, cwd, spawner) *Runner`；`func (r *Runner) Count() map[Event]int`

- [ ] **Step 1: 实现 `internal/hooks/runner.go`**

```go
package hooks

import (
	"context"
	"fmt"

	"github.com/wsx864321/coding-agent/internal/agent"
	"github.com/wsx864321/coding-agent/internal/provider"
)

var _ agent.ToolHooks = (*Runner)(nil)

type Runner struct {
	hooks   []ResolvedHook
	cwd     string
	spawner Spawner
}

func NewRunner(hooks []ResolvedHook, cwd string, spawner Spawner) *Runner {
	if spawner == nil {
		spawner = DefaultSpawner
	}
	return &Runner{hooks: hooks, cwd: cwd, spawner: spawner}
}

func (r *Runner) Count() map[Event]int {
	m := make(map[Event]int)
	for _, h := range r.hooks {
		m[h.Event]++
	}
	return m
}

func (r *Runner) UserPromptSubmit(ctx context.Context, content string) error {
	payload := Payload{Event: EventUserPromptSubmit, Cwd: r.cwd, Prompt: content}
	rep := Run(ctx, payload, r.hooks, r.spawner)
	if rep.Blocked {
		last := rep.Outcomes[len(rep.Outcomes)-1]
		msg := last.Stderr
		if msg == "" {
			msg = last.Stdout
		}
		return fmt.Errorf("blocked: %s", msg)
	}
	return nil
}

func (r *Runner) PreToolUse(ctx context.Context, name string, args map[string]any) (bool, string) {
	payload := Payload{Event: EventPreToolUse, Cwd: r.cwd, ToolName: name, ToolArgs: args}
	rep := Run(ctx, payload, r.hooks, r.spawner)
	if rep.Blocked {
		last := rep.Outcomes[len(rep.Outcomes)-1]
		msg := last.Stderr
		if msg == "" {
			msg = last.Stdout
		}
		return true, msg
	}
	return false, ""
}

func (r *Runner) PostToolUse(ctx context.Context, name string, args map[string]any, result string) {
	payload := Payload{Event: EventPostToolUse, Cwd: r.cwd, ToolName: name, ToolArgs: args, ToolResult: result}
	_ = Run(ctx, payload, r.hooks, r.spawner)
}

func (r *Runner) Stop(ctx context.Context, messages []provider.Message) (string, bool) {
	_ = messages // D7: 当前设计 doc Payload 不含 messages；Stop 仅传 event+cwd
	payload := Payload{Event: EventStop, Cwd: r.cwd}
	rep := Run(ctx, payload, r.hooks, r.spawner)
	if rep.Force != "" {
		return rep.Force, true
	}
	return "", false
}
```

- [ ] **Step 2: 验证编译**

Run: `go build ./internal/hooks/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/hooks/runner.go
git commit -m "feat(hooks): add Runner facade implementing agent.ToolHooks"
```

---

### Task 3.2: Runner 集成测试（D4）

**依赖:** Task 3.1

**Files:**
- Create: `internal/hooks/runner_test.go`

- [ ] **Step 1: 编写 Runner 集成测试**

```go
func TestRunner_PreToolUse_BlockChain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script path varies on Windows")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "block.sh")
	os.WriteFile(script, []byte("#!/bin/sh\nexit 2\n"), 0o755)

	runner := NewRunner([]ResolvedHook{{
		Event: EventPreToolUse,
		HookConfig: HookConfig{Command: script, Match: "echo", Timeout: 5000},
	}}, dir, DefaultSpawner)

	blocked, msg := runner.PreToolUse(context.Background(), "echo", map[string]any{"input": "x"})
	if !blocked {
		t.Fatalf("expected block, msg=%q", msg)
	}
}

func TestRunner_EmptyHooks_NoOp(t *testing.T) {
	runner := NewRunner(nil, t.TempDir(), DefaultSpawner)
	if err := runner.UserPromptSubmit(context.Background(), "hi"); err != nil {
		t.Fatal(err)
	}
	blocked, _ := runner.PreToolUse(context.Background(), "bash", nil)
	if blocked {
		t.Fatal("empty hooks should not block")
	}
	force, ok := runner.Stop(context.Background(), nil)
	if ok || force != "" {
		t.Fatalf("force=%q ok=%v", force, ok)
	}
}
```

- [ ] **Step 2: 运行测试**

Run: `go test ./internal/hooks/ -run TestRunner -count=1 -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/hooks/runner_test.go
git commit -m "test(hooks): add Runner ToolHooks integration tests"
```

---

## 第 4 组：Agent 集成改造

### Task 4.1: 修改 WithHooks 与 Agent struct（D1）

**依赖:** Task 1.1, Task 3.1

**Files:**
- Modify: `internal/agent/option.go`
- Modify: `internal/agent/agent.go`（struct 字段、`Hooks()` 返回类型）

**变更描述:** `hooks` 字段从 `*hooks.Registry` 改为 `ToolHooks`；移除 agent 对 hooks 包的 import（subagent flag 仍由 subagent.go import hooks）。

- [ ] **Step 1: 修改 `internal/agent/option.go`**

```go
// hooksOpt 注入 ToolHooks
type hooksOpt struct{ h ToolHooks }

func (o hooksOpt) apply(a *Agent) {
	a.hooks = o.h
}

// WithHooks 注入事件 hook 实现（可为 nil 或空 Runner）
func WithHooks(h ToolHooks) Option {
	return hooksOpt{h: h}
}
```

删除 `import "github.com/wsx864321/coding-agent/internal/hooks"`。

- [ ] **Step 2: 修改 `internal/agent/agent.go` struct**

```go
type Agent struct {
	// ...
	hooks ToolHooks  // was: *hooks.Registry
	// ...
}

// Hooks 返回当前注入的 ToolHooks（可能为 nil）
func (a *Agent) Hooks() ToolHooks {
	return a.hooks
}
```

- [ ] **Step 3: 验证编译（预期失败于 loop/agent 旧调用）**

Run: `go build ./internal/agent/...`
Expected: FAIL on `Trigger*` methods — 后续 Task 4.2–4.5 修复

- [ ] **Step 4: Commit（可与 4.2 合并提交，若中间态不可编译则 Task 4.1–4.5 同一 commit）**

---

### Task 4.2: UserPromptSubmit 触发点改造（D1）

**依赖:** Task 4.1

**Files:**
- Modify: `internal/agent/agent.go`（约 L252、L305，`Run` / `RunStreaming`）

**变更描述:** `TriggerUserPromptSubmit` → `UserPromptSubmit`；error 仍忽略（与旧行为一致：旧 Registry 也忽略 error）。

- [ ] **Step 1: 替换触发调用**

```go
// Before
if a.hooks != nil {
	a.hooks.TriggerUserPromptSubmit(ctx, userInput)
}

// After
if a.hooks != nil {
	_ = a.hooks.UserPromptSubmit(ctx, userInput)
}
```

两处：`Run()` 与 `RunStreaming()` 中各一处。

- [ ] **Step 2: 验证**

Run: `go build ./internal/agent/...`
Expected: 仍有 loop.go 编译错误（Pre/Post/Stop 未改）

---

### Task 4.3: PreToolUse / PostToolUse 触发点改造（D1）

**依赖:** Task 4.1

**Files:**
- Modify: `internal/agent/loop.go`（`invokeTool`，约 L154–191）

**变更描述:** 旧 Registry 返回 `(blocked bool, reason)` 其中 blocked 来自非空 block 字符串；新 interface 返回 `(block bool, message string)`。

- [ ] **Step 1: 替换 PreToolUse**

```go
if a.hooks != nil {
	if blocked, msg := a.hooks.PreToolUse(ctx, name, args); blocked {
		result = fmt.Sprintf("Blocked by hook: %s", msg)
		return result
	}
}
```

- [ ] **Step 2: 替换 PostToolUse（两处：error 路径与 success 路径）**

```go
a.hooks.PostToolUse(ctx, name, args, fmt.Sprintf("Error: %v", err))
// ...
a.hooks.PostToolUse(ctx, name, args, out)
```

- [ ] **Step 3: 验证编译**

Run: `go build ./internal/agent/...`
Expected: 仍有 Stop 相关编译错误

---

### Task 4.4: 内联 TodoGuard 逻辑（D5）

**依赖:** 无（可与 4.1–4.3 并行）

**Files:**
- Create: `internal/agent/todo_guard.go`
- Modify: `internal/agent/loop.go`（`loopStepWithText` Stop 段，约 L88–103）

**Interfaces:**
- Consumes: `evidence.FromContext(ctx)`、`ledger.CurrentTodos()`、`ledger.IncrementGuardBlock()`（现有 `internal/hooks/builtin/todo_guard.go` 逻辑）
- Produces: `func (a *Agent) checkTodoGuard(ctx context.Context) string`

- [ ] **Step 1: 创建 `internal/agent/todo_guard.go`**

从 `internal/hooks/builtin/todo_guard.go` 迁移逻辑（去掉 Sink，改用标准 log 或静默）：

```go
package agent

import (
	"context"
	"fmt"
	"log"

	"github.com/wsx864321/coding-agent/internal/evidence"
)

const maxGuardBlocks = 3

func (a *Agent) checkTodoGuard(ctx context.Context) string {
	ledger, ok := evidence.FromContext(ctx)
	if !ok {
		return ""
	}
	todos := ledger.CurrentTodos()
	if len(todos) == 0 {
		return ""
	}
	var incomplete []string
	for _, t := range todos {
		if t.Status != "completed" {
			incomplete = append(incomplete, fmt.Sprintf("%s [%s]", t.Content, t.Status))
		}
	}
	if len(incomplete) == 0 {
		return ""
	}
	blocks := ledger.IncrementGuardBlock()
	if blocks > maxGuardBlocks {
		log.Printf("[agent] 终答守卫: %d/%d 未完成，已超过最大阻断次数 (%d)，放行",
			len(incomplete), len(todos), maxGuardBlocks)
		return ""
	}
	log.Printf("[agent] 终答守卫: 阻断最终回答 — %d/%d 待办未完成（第 %d/%d 次阻断）",
		len(incomplete), len(todos), blocks, maxGuardBlocks)

	force := fmt.Sprintf(
		"宿主就绪检查失败: %d/%d 待办仍未完成。请先完成剩余任务再给出最终回答:\n",
		len(incomplete), len(todos))
	for _, item := range incomplete {
		force += "  - " + item + "\n"
	}
	force += "请执行必要的工具调用（complete_step + todo_write），待所有待办完成后再回答。"
	return force
}
```

- [ ] **Step 2: 修改 `loop.go` Stop 判断顺序（D5）**

```go
if len(msg.ToolCalls) == 0 {
	if a.memSet != nil {
		a.maybeExtractMemories(ctx)
	}

	// 1. 内置 TodoGuard（优先于外部 hooks）
	if force := a.checkTodoGuard(ctx); force != "" {
		a.messages = append(a.messages, provider.Message{
			Role: provider.RoleUser, Content: force,
		})
		return "", nil
	}

	// 2. 外部 Stop hooks
	if a.hooks != nil {
		force, ok := a.hooks.Stop(ctx, a.messages)
		if ok {
			a.messages = append(a.messages, provider.Message{
				Role: provider.RoleUser, Content: force,
			})
			return "", nil
		}
	}
	return msg.Content, nil
}
```

- [ ] **Step 3: 编写 TodoGuard 单元测试**

Create: `internal/agent/todo_guard_test.go`（使用 mock ledger context）

- [ ] **Step 4: 运行测试**

Run: `go test ./internal/agent/ -run TestCheckTodoGuard -count=1 -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/todo_guard.go internal/agent/todo_guard_test.go internal/agent/loop.go
git commit -m "feat(agent): inline TodoGuard before external Stop hooks"
```

---

### Task 4.5: Stop 触发点改造（D1）

**依赖:** Task 4.4（Stop 段已在 4.4 一并修改则本 task 仅验证）

**Files:**
- Modify: `internal/agent/loop.go`

**验证步骤:** 确认 `TriggerStop` 已全部替换为 `a.hooks.Stop(ctx, a.messages)`，且无 Registry 引用。

Run: `grep -r TriggerStop internal/agent/` → 无匹配

---

### Task 4.6: Subagent hook 传递改造（D1）

**依赖:** Task 1.1, Task 4.1

**Files:**
- Modify: `internal/agent/subagent.go`
- Modify: `internal/agent/agent.go`（`WireTaskTool`、`WireSkillTools` 中 subHooks 逻辑）

**变更描述:** `SubagentOptions.Hooks` 从 `*hooks.Registry` 改为 `ToolHooks`；`WithoutStopAndPrompt()` 改为 `NewSubsetHooks()`。

- [ ] **Step 1: 修改 `subagent.go`**

```go
type SubagentOptions struct {
	SystemPrompt string
	MaxTurns     int
	Registry     *tools.Registry
	Hooks        ToolHooks  // was: *hooks.Registry
	Checker      permission.Checker
}
```

保留 `hooks.WithSubagentFlag(ctx)` import。

- [ ] **Step 2: 修改 `agent.go` WireTaskTool / WireSkillTools**

```go
var subHooks ToolHooks
if a.hooks != nil {
	subHooks = NewSubsetHooks(a.hooks)
}
return RunSubAgent(ctx, a, prompt, SubagentOptions{
	Hooks:   subHooks,
	Checker: a.checker,
})
```

- [ ] **Step 3: 验证编译**

Run: `go build ./internal/agent/...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/agent/subagent.go internal/agent/agent.go internal/agent/option.go internal/agent/loop.go
git commit -m "refactor(agent): migrate hook triggers to ToolHooks interface"
```

---

## 第 5 组：CLI 装配层适配

### Task 5.1: 修改 once.go（D4）

**依赖:** Task 3.1

**Files:**
- Modify: `cmd/cli/once.go`

**变更描述:** 移除 `builtin.NewDefault`；改用 `hooks.Load` + `hooks.NewRunner`。

- [ ] **Step 1: 替换 import 与装配**

```go
import (
	"github.com/wsx864321/coding-agent/internal/hooks"
	// 删除: "github.com/wsx864321/coding-agent/internal/hooks/builtin"
)

// runOnce 内
hookRunner := hooks.NewRunner(
	hooks.Load(hooks.LoadOptions{ProjectRoot: workdir}),
	workdir,
	hooks.DefaultSpawner,
)
a, err := agent.NewAgent(buildConfig(cmd),
	agent.WithRegistry(registry),
	agent.WithChecker(checker),
	agent.WithHooks(hookRunner),
	agent.WithSkillStore(skillStore),
)
```

- [ ] **Step 2: 验证编译**

Run: `go build ./cmd/cli/...`
Expected: PASS（chat_setup 仍引用 builtin 则可能部分失败，Task 5.2 补齐）

- [ ] **Step 3: Commit**

```bash
git add cmd/cli/once.go
git commit -m "refactor(cli): wire shell hook Runner in once command"
```

---

### Task 5.2: 修改 chat_setup.go（D4）

**依赖:** Task 3.1

**Files:**
- Modify: `cmd/cli/chat_setup.go`

- [ ] **Step 1: 同 Task 5.1 替换 hook 装配**

```go
hookRunner := hooks.NewRunner(
	hooks.Load(hooks.LoadOptions{ProjectRoot: workdir}),
	workdir,
	hooks.DefaultSpawner,
)
// ...
agent.WithHooks(hookRunner),
```

- [ ] **Step 2: 验证编译**

Run: `go build ./cmd/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add cmd/cli/chat_setup.go
git commit -m "refactor(cli): wire shell hook Runner in chat/tui setup"
```

---

### Task 5.3: 适配 chat.go /hooks 命令（D4）

**依赖:** Task 4.1, Task 3.1

**Files:**
- Modify: `cmd/cli/chat.go`（`/hooks` 命令与启动 banner，约 L109–111、L185–191）

**变更描述:** `a.Hooks()` 现返回 `ToolHooks`；通过 type assert 到 `*hooks.Runner` 获取 `Count()`。

- [ ] **Step 1: 更新 hook 计数展示**

```go
func formatAgentHooks(h agent.ToolHooks) string {
	if h == nil {
		return "未配置"
	}
	if r, ok := h.(*hooks.Runner); ok {
		return formatHookCounts(r.Count())
	}
	return "已配置（自定义 ToolHooks）"
}

// 调用处
if h := a.Hooks(); h != nil {
	fmt.Printf("[coding-agent] 已注册 hooks: %s\n", formatAgentHooks(h))
}
```

- [ ] **Step 2: 验证**

Run: `go build ./cmd/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add cmd/cli/chat.go
git commit -m "refactor(cli): adapt /hooks command for ToolHooks Runner"
```

---

## 第 6 组：清理旧代码

### Task 6.1: 删除 builtin 目录（D6）

**依赖:** Task 5.1, Task 5.2, Task 4.4

**Files:**
- Delete: `internal/hooks/builtin/` 整个目录（default.go、sink.go、log.go、large_output.go、context_inject.go、todo_guard.go、summary.go）

**验证步骤:**

Run: `grep -r "hooks/builtin" . --include="*.go"`
Expected: 无匹配

Run: `go build ./...`
Expected: PASS

- [ ] **Step 1: 删除目录并 commit**

```bash
git rm -r internal/hooks/builtin/
git commit -m "refactor(hooks): remove builtin in-process hook implementations"
```

---

### Task 6.2: 移除 Registry 并重写 hooks.go（D6）

**依赖:** Task 6.1, Task 4.6

**Files:**
- Delete/Rewrite: `internal/hooks/hooks.go`（删除 Registry/Register*/Trigger*/WithoutStopAndPrompt/Count）
- Rewrite: `internal/hooks/hooks_test.go`

**变更描述:** `context.go` 已在 Task 1.2 迁出；`hooks.go` 可删除或保留为包文档注释文件。确保 subagent 辅助函数仅在 `context.go`。

- [ ] **Step 1: 删除 `internal/hooks/hooks.go`**

若包内无其它顶层声明问题，直接删除；subagent 函数已在 `context.go`。

- [ ] **Step 2: 删除旧 Registry 测试**

删除 `hooks_test.go` 中全部 `TestRegistry_*` 测试（Load/Run/Runner 测试已在其它文件）。

- [ ] **Step 3: 全量编译**

Run: `go build ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/hooks/
git commit -m "refactor(hooks): remove Registry-based in-process hook system"
```

---

### Task 6.3: 适配 agent_test.go（D1）

**依赖:** Task 4.6, Task 6.2

**Files:**
- Modify: `internal/agent/agent_test.go`

**变更描述:** 测试用 mock `ToolHooks` 替代 `*hooks.Registry`。

- [ ] **Step 1: 添加 testToolHooks**

```go
type testToolHooks struct {
	onUserPromptSubmit func(context.Context, string) error
	onPreToolUse       func(context.Context, string, map[string]any) (bool, string)
	onPostToolUse      func(context.Context, string, map[string]any, string)
	onStop             func(context.Context, []provider.Message) (string, bool)
}

func (h *testToolHooks) UserPromptSubmit(ctx context.Context, c string) error {
	if h.onUserPromptSubmit != nil {
		return h.onUserPromptSubmit(ctx, c)
	}
	return nil
}
func (h *testToolHooks) PreToolUse(ctx context.Context, name string, args map[string]any) (bool, string) {
	if h.onPreToolUse != nil {
		return h.onPreToolUse(ctx, name, args)
	}
	return false, ""
}
func (h *testToolHooks) PostToolUse(ctx context.Context, name string, args map[string]any, result string) {
	if h.onPostToolUse != nil {
		h.onPostToolUse(ctx, name, args, result)
	}
}
func (h *testToolHooks) Stop(ctx context.Context, msgs []provider.Message) (string, bool) {
	if h.onStop != nil {
		return h.onStop(ctx, msgs)
	}
	return "", false
}
```

- [ ] **Step 2: 替换 hook 测试**

```go
// Before
hr := newTestHookRegistry()
hr.RegisterPreToolUse(func(...) (string, string) {
	return "Blocked by hook: not allowed", "test"
})

// After
hr := &testToolHooks{
	onPreToolUse: func(_ context.Context, _ string, _ map[string]any) (bool, string) {
		return true, "not allowed"
	},
}
```

注意：旧测试 block 返回的第一个 string 是 message（`"Blocked by hook: not allowed"`），新 interface 的 message 是 `"not allowed"`，loop 会格式化为 `"Blocked by hook: not allowed"` — 测试断言保持不变。

删除 `newTestHookRegistry()` 和 `hooks` 包 import（若不再需要）。

- [ ] **Step 3: 运行 agent 测试**

Run: `go test ./internal/agent/ -run "Hook|Stop|PreTool|PostTool|UserPrompt" -count=1 -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/agent/agent_test.go
git commit -m "test(agent): adapt hook integration tests to ToolHooks interface"
```

---

## 第 7 组：测试与验证

### Task 7.1: 端到端 shell hook 测试（D3, D4, D7）

**依赖:** Task 3.2, Task 4.3

**Files:**
- Create: `internal/hooks/e2e_test.go`（或 `runner_e2e_test.go`）

- [ ] **Step 1: 编写 E2E 测试**

在 temp dir 创建 `.coding-agent/hooks.json` 和 hook 脚本：

```go
func TestE2E_PreToolUse_BlockAndPass(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("e2e shell scripts require sh")
	}
	root := t.TempDir()
	hookDir := filepath.Join(root, ".coding-agent", "hooks")
	os.MkdirAll(hookDir, 0o755)

	blockScript := filepath.Join(hookDir, "block-echo.sh")
	os.WriteFile(blockScript, []byte(`#!/bin/sh
if [ "$TOOL" = "echo" ]; then exit 2; fi
exit 0
`), 0o755)

	writeHooksJSON(t, root, ".coding-agent/hooks.json", fmt.Sprintf(`{
	  "hooks": {
	    "PreToolUse": [{
	      "command": "TOOL=$(jq -r .toolName); . %s",
	      "match": "echo"
	    }]
	  }
	}`, blockScript))

	runner := NewRunner(Load(LoadOptions{ProjectRoot: root, HomeDir: t.TempDir()}), root, DefaultSpawner)
	blocked, _ := runner.PreToolUse(context.Background(), "echo", map[string]any{"input": "x"})
	if !blocked {
		t.Fatal("expected block for echo")
	}
	blocked, _ = runner.PreToolUse(context.Background(), "read_file", nil)
	if blocked {
		t.Fatal("expected pass for read_file")
	}
}
```

简化版可用 `command: "exit 2"` + `match: "echo"` 无需 jq。

- [ ] **Step 2: 运行**

Run: `go test ./internal/hooks/ -run TestE2E -count=1 -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/hooks/e2e_test.go
git commit -m "test(hooks): add e2e PreToolUse block and pass verification"
```

---

### Task 7.2: 零 hook 降级验证（D4）

**依赖:** Task 5.2, Task 6.3

**Files:**
- 无新文件；运行现有测试

- [ ] **Step 1: 验证空 Runner 不阻断 agent**

Run: `go test ./internal/hooks/ -run TestRunner_EmptyHooks -count=1 -v`
Expected: PASS

Run: `go test ./internal/agent/ -run TestRun_Stop_NoForce -count=1 -v`
Expected: PASS（使用 nil/empty hooks mock）

- [ ] **Step 2: 手动 smoke test（可选）**

```bash
go run ./cmd/cli once -m "say hi" -q
```
Expected: 正常输出，无 hook 相关 panic

---

### Task 7.3: TodoGuard 集成验证（D5）

**依赖:** Task 4.4

**Files:**
- Modify/Create: `internal/agent/todo_guard_test.go` 或扩展 `agent_test.go`

- [ ] **Step 1: 编写 TodoGuard 强制续跑测试**

模拟 ledger 含未完成 todo → `checkTodoGuard` 返回非空 force → loop 注入 user 消息并继续。

- [ ] **Step 2: 运行**

Run: `go test ./internal/agent/ -run "TodoGuard|Stop_Force" -count=1 -v`
Expected: PASS

- [ ] **Step 3: Commit（若有新测试）**

---

### Task 7.4: 全量编译与测试（最终门禁）

**依赖:** Task 6.1–6.3, Task 7.1–7.3

- [ ] **Step 1: 全量构建**

Run: `go build ./...`
Expected: PASS，无 `hooks/builtin` 或 `Registry` 残留引用

- [ ] **Step 2: 全量测试**

Run: `go test ./... -count=1`
Expected: 全部 PASS

- [ ] **Step 3: 更新 tasks.md 全部打勾**

修改: `openspec/changes/refactor-hooks-to-shell/tasks.md`

- [ ] **Step 4: Commit**

```bash
git add openspec/changes/refactor-hooks-to-shell/tasks.md
git commit -m "chore: mark refactor-hooks-to-shell tasks complete"
```

---

## 任务依赖图

```
1.1 ──┬──► 4.1 ──► 4.2 ──► 4.5
1.2 ──┼──► 1.3 ──► 1.4
      ├──► 2.1 ──► 2.2 ──► 2.3
      │              │
      │              ├──► 3.1 ──► 3.2 ──► 5.1, 5.2, 5.3
      │              │              │
      │              │              └──► 7.1
      └──► 4.4 (TodoGuard) ──► 7.3
4.1 + 1.1 ──► 4.6
5.x + 4.4 ──► 6.1 ──► 6.2 ──► 6.3 ──► 7.2, 7.4
```

## Self-Review 清单

| 规格要求 | 对应 Task | 状态 |
|---------|-----------|------|
| D1 ToolHooks interface | 1.1, 4.1–4.6 | ✓ |
| D2 JSON 配置加载 | 1.2–1.4 | ✓ |
| D3 Spawner + exit code | 2.1–2.3 | ✓ |
| D4 Runner 门面 | 3.1–3.2, 5.1–5.3 | ✓ |
| D5 TodoGuard 内联 | 4.4, 7.3 | ✓ |
| D6 移除 builtin | 6.1 | ✓ |
| D7 JSON Payload | 1.2, 2.2, 3.1 | ✓ |
| PreToolUse 后仍走 permission | 4.3（loop 顺序不变） | ✓ |
| Subagent 仅 Pre/Post | 4.6 | ✓ |
| nil hooks 安全 | 3.2, 7.2 | ✓ |
| chat.go /hooks 适配 | 5.3 | ✓ |
| agent_test 迁移 | 6.3 | ✓ |

**已知偏差说明:** openspec `design.md` D7 提到 Stop payload 含 `messages` 字段，但 canonical design doc（`docs/superpowers/specs/...`）的 `Payload` struct 与 `Runner.Stop` 仅传 `event+cwd`。本计划按 canonical design doc 实现；若后续需 messages，在 Task 3.1 扩展 `Payload` 并在 `Runner.Stop` 序列化最近 N 条即可。

---

**Plan complete and saved to `docs/superpowers/plans/2026-06-23-refactor-hooks-to-shell.md`.**

**Two execution options:**

1. **Subagent-Driven（推荐）** — 每个 Task 派发独立 subagent，任务间双阶段审查，快速迭代
2. **Inline Execution** — 当前 session 使用 executing-plans 批量执行，checkpoint 处暂停审查

**Which approach?**
