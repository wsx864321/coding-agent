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
