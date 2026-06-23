package hooks

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"
)

func testHook(event Event, cfg HookConfig) ResolvedHook {
	h := ResolvedHook{Event: event, HookConfig: cfg}
	if cfg.Match != "" {
		re, err := regexp.Compile(cfg.Match)
		if err != nil {
			panic(err)
		}
		h.compiledMatch = re
	}
	return h
}

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

func TestRun_PreToolUse_Pass(t *testing.T) {
	hooks := []ResolvedHook{{
		Event: EventPreToolUse, HookConfig: HookConfig{Command: "pass-hook"},
	}}
	sp := mockSpawner(map[string]SpawnResult{"pass-hook": {ExitCode: 0}})
	rep := Run(context.Background(), Payload{
		Event: EventPreToolUse, Cwd: "/tmp", ToolName: "bash",
	}, hooks, sp, nil)
	if rep.Blocked || len(rep.Outcomes) != 1 || rep.Outcomes[0].Decision != DecisionPass {
		t.Fatalf("rep=%+v", rep)
	}
}

func TestRun_PreToolUse_BlockShortCircuit(t *testing.T) {
	hooks := []ResolvedHook{
		testHook(EventPreToolUse, HookConfig{Command: "block-hook", Match: "bash"}),
		testHook(EventPreToolUse, HookConfig{Command: "second-hook"}),
	}
	sp := mockSpawner(map[string]SpawnResult{
		"block-hook":  {ExitCode: 2, Stderr: "denied"},
		"second-hook": {ExitCode: 0},
	})
	rep := Run(context.Background(), Payload{
		Event: EventPreToolUse, Cwd: "/tmp", ToolName: "bash", ToolArgs: map[string]any{"command": "rm"},
	}, hooks, sp, nil)
	if !rep.Blocked || len(rep.Outcomes) != 1 {
		t.Fatalf("rep=%+v", rep)
	}
	if rep.Outcomes[0].Decision != DecisionBlock {
		t.Fatalf("decision=%q", rep.Outcomes[0].Decision)
	}
}

func TestRun_PreToolUse_MatchFilter(t *testing.T) {
	hooks := []ResolvedHook{testHook(EventPreToolUse, HookConfig{Command: "only-bash", Match: "^bash$"})}
	called := false
	sp := Spawner(func(_ context.Context, in SpawnInput) SpawnResult {
		called = true
		return SpawnResult{ExitCode: 0}
	})
	Run(context.Background(), Payload{Event: EventPreToolUse, ToolName: "read_file"}, hooks, sp, nil)
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
	rep := Run(context.Background(), Payload{Event: EventStop, Cwd: "/tmp"}, hooks, sp, nil)
	if rep.Force != "请继续完成待办" {
		t.Fatalf("force=%q", rep.Force)
	}
}

func TestRun_Stop_ForceRequiresStdout(t *testing.T) {
	hooks := []ResolvedHook{{
		Event: EventStop, HookConfig: HookConfig{Command: "stop-hook"},
	}}
	sp := mockSpawner(map[string]SpawnResult{
		"stop-hook": {ExitCode: 2, Stdout: ""},
	})
	rep := Run(context.Background(), Payload{Event: EventStop, Cwd: "/tmp"}, hooks, sp, nil)
	if rep.Force != "" {
		t.Fatalf("force=%q, expected empty without stdout", rep.Force)
	}
}

func TestRun_PostToolUse_NonBlockingWarn(t *testing.T) {
	hooks := []ResolvedHook{{
		Event: EventPostToolUse, HookConfig: HookConfig{Command: "warn-hook"},
	}}
	sp := mockSpawner(map[string]SpawnResult{"warn-hook": {ExitCode: 2}})
	rep := Run(context.Background(), Payload{Event: EventPostToolUse, ToolName: "bash"}, hooks, sp, nil)
	if rep.Blocked {
		t.Fatal("PostToolUse exit 2 should warn, not block")
	}
	if len(rep.Outcomes) != 1 || rep.Outcomes[0].Decision != DecisionWarn {
		t.Fatalf("rep=%+v", rep)
	}
}

func TestRun_PostToolUse_NonZeroWarn(t *testing.T) {
	hooks := []ResolvedHook{{
		Event: EventPostToolUse, HookConfig: HookConfig{Command: "err-hook"},
	}}
	sp := mockSpawner(map[string]SpawnResult{"err-hook": {ExitCode: 1}})
	rep := Run(context.Background(), Payload{Event: EventPostToolUse, ToolName: "bash"}, hooks, sp, nil)
	if rep.Blocked || rep.Outcomes[0].Decision != DecisionWarn {
		t.Fatalf("rep=%+v", rep)
	}
}

func TestRun_Timeout_BlockOnPreToolUse(t *testing.T) {
	hooks := []ResolvedHook{{
		Event: EventPreToolUse, HookConfig: HookConfig{Command: "slow", Timeout: 1},
	}}
	sp := Spawner(func(context.Context, SpawnInput) SpawnResult {
		return SpawnResult{TimedOut: true, ExitCode: -1}
	})
	rep := Run(context.Background(), Payload{Event: EventPreToolUse, ToolName: "bash"}, hooks, sp, nil)
	if !rep.Blocked {
		t.Fatal("timeout on blocking event should block")
	}
	if rep.Outcomes[0].Decision != DecisionBlock {
		t.Fatalf("decision=%q", rep.Outcomes[0].Decision)
	}
}

func TestRun_Timeout_WarnOnPostToolUse(t *testing.T) {
	hooks := []ResolvedHook{{
		Event: EventPostToolUse, HookConfig: HookConfig{Command: "slow"},
	}}
	sp := Spawner(func(context.Context, SpawnInput) SpawnResult {
		return SpawnResult{TimedOut: true, ExitCode: -1}
	})
	rep := Run(context.Background(), Payload{Event: EventPostToolUse, ToolName: "bash"}, hooks, sp, nil)
	if rep.Blocked || rep.Outcomes[0].Decision != DecisionWarn {
		t.Fatalf("rep=%+v", rep)
	}
}

func TestRun_EventFilter(t *testing.T) {
	hooks := []ResolvedHook{{
		Event: EventStop, HookConfig: HookConfig{Command: "stop-only"},
	}}
	called := false
	sp := Spawner(func(context.Context, SpawnInput) SpawnResult {
		called = true
		return SpawnResult{ExitCode: 0}
	})
	rep := Run(context.Background(), Payload{Event: EventPreToolUse, ToolName: "bash"}, hooks, sp, nil)
	if called || len(rep.Outcomes) != 0 {
		t.Fatalf("wrong event hook should be skipped, rep=%+v called=%v", rep, called)
	}
}

func TestRun_Stop_ForceFirstWins(t *testing.T) {
	hooks := []ResolvedHook{
		{Event: EventStop, HookConfig: HookConfig{Command: "first-stop"}},
		{Event: EventStop, HookConfig: HookConfig{Command: "second-stop"}},
	}
	sp := mockSpawner(map[string]SpawnResult{
		"first-stop":  {ExitCode: 2, Stdout: "first force"},
		"second-stop": {ExitCode: 2, Stdout: "second force"},
	})
	rep := Run(context.Background(), Payload{Event: EventStop, Cwd: "/tmp"}, hooks, sp, nil)
	if rep.Force != "first force" {
		t.Fatalf("force=%q, want first hook to win", rep.Force)
	}
	if len(rep.Outcomes) != 1 {
		t.Fatalf("expected short-circuit after first force, got %d outcomes", len(rep.Outcomes))
	}
}

func TestRun_SpawnFailure_PreToolUseError(t *testing.T) {
	hooks := []ResolvedHook{{
		Event: EventPreToolUse, HookConfig: HookConfig{Command: "missing-hook"},
	}}
	sp := Spawner(func(context.Context, SpawnInput) SpawnResult {
		return SpawnResult{Err: errors.New("exec: command not found")}
	})
	rep := Run(context.Background(), Payload{Event: EventPreToolUse, ToolName: "bash"}, hooks, sp, nil)
	if rep.Blocked {
		t.Fatal("spawn failure should not block PreToolUse")
	}
	if len(rep.Outcomes) != 1 || rep.Outcomes[0].Decision != DecisionError {
		t.Fatalf("rep=%+v", rep)
	}
}

func TestRun_SpawnFailure_PostToolUseWarn(t *testing.T) {
	hooks := []ResolvedHook{{
		Event: EventPostToolUse, HookConfig: HookConfig{Command: "missing-hook"},
	}}
	sp := Spawner(func(context.Context, SpawnInput) SpawnResult {
		return SpawnResult{Err: errors.New("exec: command not found")}
	})
	rep := Run(context.Background(), Payload{Event: EventPostToolUse, ToolName: "bash"}, hooks, sp, nil)
	if rep.Blocked || rep.Outcomes[0].Decision != DecisionWarn {
		t.Fatalf("rep=%+v", rep)
	}
}

func TestDecideOutcome(t *testing.T) {
	tests := []struct {
		event Event
		res   SpawnResult
		want  Decision
	}{
		{EventPreToolUse, SpawnResult{ExitCode: 0}, DecisionPass},
		{EventPreToolUse, SpawnResult{ExitCode: 2}, DecisionBlock},
		{EventPreToolUse, SpawnResult{ExitCode: 1}, DecisionWarn},
		{EventPreToolUse, SpawnResult{TimedOut: true}, DecisionBlock},
		{EventPreToolUse, SpawnResult{Err: errors.New("spawn failed")}, DecisionError},
		{EventPostToolUse, SpawnResult{ExitCode: 2}, DecisionWarn},
		{EventPostToolUse, SpawnResult{ExitCode: 1}, DecisionWarn},
		{EventPostToolUse, SpawnResult{TimedOut: true}, DecisionWarn},
		{EventPostToolUse, SpawnResult{Err: errors.New("spawn failed")}, DecisionWarn},
		{EventUserPromptSubmit, SpawnResult{ExitCode: 2}, DecisionWarn},
		{EventStop, SpawnResult{ExitCode: 2}, DecisionWarn},
	}
	for _, tc := range tests {
		got := decideOutcome(tc.event, tc.res)
		if got != tc.want {
			t.Errorf("decideOutcome(%q, %+v) = %q, want %q", tc.event, tc.res, got, tc.want)
		}
	}
}

func TestRun_PassesPayloadAsStdin(t *testing.T) {
	var gotStdin string
	sp := Spawner(func(_ context.Context, in SpawnInput) SpawnResult {
		gotStdin = in.Stdin
		return SpawnResult{ExitCode: 0}
	})
	hooks := []ResolvedHook{{
		Event: EventPreToolUse, HookConfig: HookConfig{Command: "echo"},
	}}
	Run(context.Background(), Payload{
		Event: EventPreToolUse, Cwd: "/proj", ToolName: "bash",
	}, hooks, sp, nil)
	if !strings.Contains(gotStdin, `"event":"PreToolUse"`) || !strings.Contains(gotStdin, `"cwd":"/proj"`) {
		t.Fatalf("stdin=%q", gotStdin)
	}
}

func TestRun_UsesHookCwdOverPayload(t *testing.T) {
	var gotCwd string
	sp := Spawner(func(_ context.Context, in SpawnInput) SpawnResult {
		gotCwd = in.Cwd
		return SpawnResult{ExitCode: 0}
	})
	hooks := []ResolvedHook{{
		Event: EventStop, HookConfig: HookConfig{Command: "pwd", Cwd: "/hook-cwd"},
	}}
	Run(context.Background(), Payload{Event: EventStop, Cwd: "/payload-cwd"}, hooks, sp, nil)
	if gotCwd != "/hook-cwd" {
		t.Fatalf("cwd=%q", gotCwd)
	}
}

func TestRun_UsesPayloadCwdWhenHookEmpty(t *testing.T) {
	var gotCwd string
	sp := Spawner(func(_ context.Context, in SpawnInput) SpawnResult {
		gotCwd = in.Cwd
		return SpawnResult{ExitCode: 0}
	})
	hooks := []ResolvedHook{{
		Event: EventStop, HookConfig: HookConfig{Command: "pwd"},
	}}
	Run(context.Background(), Payload{Event: EventStop, Cwd: "/payload-cwd"}, hooks, sp, nil)
	if gotCwd != "/payload-cwd" {
		t.Fatalf("cwd=%q", gotCwd)
	}
}

func TestRun_PassesTimeoutToSpawner(t *testing.T) {
	var gotTimeout time.Duration
	sp := Spawner(func(_ context.Context, in SpawnInput) SpawnResult {
		gotTimeout = in.Timeout
		return SpawnResult{ExitCode: 0}
	})
	hooks := []ResolvedHook{{
		Event: EventStop, HookConfig: HookConfig{Command: "sleep", Timeout: 5000},
	}}
	Run(context.Background(), Payload{Event: EventStop}, hooks, sp, nil)
	if gotTimeout != 5*time.Second {
		t.Fatalf("timeout=%v", gotTimeout)
	}
}

func TestRun_NotifyOnSpawnError(t *testing.T) {
	var notified string
	hooks := []ResolvedHook{{Event: EventPostToolUse, HookConfig: HookConfig{Command: "x"}}}
	sp := Spawner(func(_ context.Context, _ SpawnInput) SpawnResult {
		return SpawnResult{Err: errors.New("spawn boom")}
	})
	notify := func(msg string) { notified = msg }
	Run(context.Background(), Payload{Event: EventPostToolUse, Cwd: "/tmp"}, hooks, sp, notify)
	if notified == "" {
		t.Fatal("expected notify on spawn failure")
	}
}

func TestRun_NotifyOnInvalidRegex(t *testing.T) {
	var notified string
	h := ResolvedHook{
		Event: EventPreToolUse, HookConfig: HookConfig{Command: "hook", Match: "bad["},
	}
	notify := func(msg string) { notified = msg }
	Run(context.Background(), Payload{Event: EventPreToolUse, ToolName: "bash"}, []ResolvedHook{h}, mockSpawner(nil), notify)
	if notified == "" {
		t.Fatal("expected notify on invalid regex")
	}
}
