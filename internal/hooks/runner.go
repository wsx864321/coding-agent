package hooks

import (
	"context"
	"fmt"

	"github.com/wsx864321/coding-agent/internal/provider"
)

type Runner struct {
	hooks   []ResolvedHook
	cwd     string
	spawner Spawner
	notify  func(string)
}

func NewRunner(hooks []ResolvedHook, cwd string, spawner Spawner, notify func(string)) *Runner {
	if spawner == nil {
		spawner = DefaultSpawner
	}
	if notify == nil {
		notify = func(string) {}
	}
	return &Runner{hooks: hooks, cwd: cwd, spawner: spawner, notify: notify}
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
	rep := Run(ctx, payload, r.hooks, r.spawner, r.notify)
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
	rep := Run(ctx, payload, r.hooks, r.spawner, r.notify)
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
	_ = Run(ctx, payload, r.hooks, r.spawner, r.notify)
}

func (r *Runner) Stop(ctx context.Context, messages []provider.Message) (string, bool) {
	_ = messages // D7: Payload 不含 messages；Stop 仅传 event+cwd
	payload := Payload{Event: EventStop, Cwd: r.cwd}
	rep := Run(ctx, payload, r.hooks, r.spawner, r.notify)
	if rep.Force != "" {
		return rep.Force, true
	}
	return "", false
}
