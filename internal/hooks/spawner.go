package hooks

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func DefaultSpawner(ctx context.Context, in SpawnInput) SpawnResult {
	if in.Timeout <= 0 {
		in.Timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, in.Timeout)
	defer cancel()

	name, args := shellCommand(in.Command)
	cmd := exec.CommandContext(ctx, name, args...)
	if in.Cwd != "" {
		cmd.Dir = in.Cwd
	}
	cmd.Stdin = strings.NewReader(in.Stdin)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := SpawnResult{
		Stdout: strings.TrimSpace(stdout.String()),
		Stderr: strings.TrimSpace(stderr.String()),
	}
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			res.TimedOut = true
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		} else if res.TimedOut {
			res.ExitCode = -1
		} else {
			res.Err = err
		}
	}
	return res
}

func shellCommand(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("sh"); err == nil {
			return "sh", []string{"-c", command}
		}
		return "cmd", []string{"/c", command}
	}
	return "sh", []string{"-c", command}
}
