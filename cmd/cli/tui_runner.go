package cli

import (
	"context"

	"github.com/wsx864321/coding-agent/internal/agent"
	"github.com/wsx864321/coding-agent/internal/tui"
)

// agentRunner 将 *agent.Agent 适配为 TUI Runner。
type agentRunner struct {
	agent *agent.Agent
}

func newAgentRunner(a *agent.Agent) tui.Runner {
	return agentRunner{agent: a}
}

func (r agentRunner) RunTurn(ctx context.Context, prompt string, emit tui.StreamEmitter) error {
	_, err := r.agent.RunStreaming(ctx, prompt, emit)
	if err != nil {
		emit.OnError(err)
		return err
	}
	emit.OnDone()
	return nil
}
