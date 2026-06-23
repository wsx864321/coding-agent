package hooks

import (
	"context"
	"time"
)

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
