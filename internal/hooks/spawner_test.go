package hooks

import (
	"context"
	"runtime"
	"testing"
	"time"
)

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
	if runtime.GOOS == "windows" {
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

func TestShellCommand(t *testing.T) {
	name, args := shellCommand("echo hi")
	if runtime.GOOS == "windows" {
		if name == "sh" {
			if len(args) != 2 || args[0] != "-c" {
				t.Fatalf("sh args=%v", args)
			}
		} else if name != "cmd" || len(args) != 2 || args[0] != "/c" {
			t.Fatalf("cmd args=%v", args)
		}
	} else {
		if name != "sh" || len(args) != 2 || args[0] != "-c" {
			t.Fatalf("name=%q args=%v", name, args)
		}
	}
}
