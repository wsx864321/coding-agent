package hooks

import (
	"context"
	"encoding/json"
	"log"
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

		body, err := json.Marshal(payload)
		if err != nil {
			log.Printf("[hooks] marshal payload for hook %q: %v", h.Command, err)
			continue
		}
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
		if res.Err != nil && !res.TimedOut {
			log.Printf("[hooks] spawn failed for hook %q: %v", h.Command, res.Err)
		}
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
			break
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
	return e == EventPreToolUse
}

func matchesHook(h ResolvedHook, p Payload) bool {
	if h.Match == "" {
		return true
	}
	if p.Event != EventPreToolUse && p.Event != EventPostToolUse {
		return true
	}
	if h.compiledMatch == nil {
		log.Printf("[hooks] invalid match regex %q in hook %q: not compiled", h.Match, h.Command)
		return false
	}
	return h.compiledMatch.MatchString(p.ToolName)
}

func decideOutcome(event Event, res SpawnResult) Decision {
	blocking := isBlockingEvent(event)
	if res.Err != nil && !res.TimedOut {
		if blocking {
			return DecisionError
		}
		return DecisionWarn
	}
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
