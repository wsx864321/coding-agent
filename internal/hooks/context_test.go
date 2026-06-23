package hooks

import (
	"context"
	"testing"
)

func TestWithSubagentFlag(t *testing.T) {
	ctx := WithSubagentFlag(context.Background())
	if !IsSubagent(ctx) {
		t.Error("expected IsSubagent=true after WithSubagentFlag")
	}
}

func TestIsSubagent_DefaultFalse(t *testing.T) {
	if IsSubagent(context.Background()) {
		t.Error("expected IsSubagent=false for plain context")
	}
}
