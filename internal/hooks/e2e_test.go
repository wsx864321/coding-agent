package hooks

import (
	"context"
	"testing"
)

func TestE2E_PreToolUse_BlockAndPass(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()

	writeHooksJSON(t, root, ".coding-agent/hooks.json", `{
	  "hooks": {
	    "PreToolUse": [{
	      "command": "exit 2",
	      "match": "echo"
	    }]
	  }
	}`)

	loaded := Load(LoadOptions{ProjectRoot: root, HomeDir: home})
	if len(loaded) != 1 {
		t.Fatalf("Load() len=%d, want 1", len(loaded))
	}

	runner := NewRunner(loaded, root, DefaultSpawner)

	blocked, _ := runner.PreToolUse(context.Background(), "echo", map[string]any{"input": "x"})
	if !blocked {
		t.Fatal("expected block for echo")
	}

	blocked, _ = runner.PreToolUse(context.Background(), "read_file", nil)
	if blocked {
		t.Fatal("expected pass for read_file")
	}
}

func TestZeroHookDegradation_LoadEmptyRunner(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()

	loaded := Load(LoadOptions{ProjectRoot: root, HomeDir: home})
	if len(loaded) != 0 {
		t.Fatalf("Load() without config should be empty, got %d hooks", len(loaded))
	}

	runner := NewRunner(loaded, root, DefaultSpawner)

	if err := runner.UserPromptSubmit(context.Background(), "hello"); err != nil {
		t.Fatalf("UserPromptSubmit: %v", err)
	}

	blocked, msg := runner.PreToolUse(context.Background(), "bash", nil)
	if blocked || msg != "" {
		t.Fatalf("PreToolUse should pass, blocked=%v msg=%q", blocked, msg)
	}

	runner.PostToolUse(context.Background(), "bash", map[string]any{"cmd": "ls"}, "ok")

	force, ok := runner.Stop(context.Background(), nil)
	if ok || force != "" {
		t.Fatalf("Stop should not force, force=%q ok=%v", force, ok)
	}
}
