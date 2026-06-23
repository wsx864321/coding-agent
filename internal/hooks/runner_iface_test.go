package hooks_test

import (
	"testing"

	"github.com/wsx864321/coding-agent/internal/agent"
	"github.com/wsx864321/coding-agent/internal/hooks"
)

func TestRunner_ImplementsToolHooks(t *testing.T) {
	var _ agent.ToolHooks = (*hooks.Runner)(nil)
}
