package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestTuiCommandRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"tui"})
	if err != nil {
		t.Fatalf("Find tui: %v", err)
	}
	if cmd.Use != "tui" {
		t.Errorf("Use = %q, want tui", cmd.Use)
	}
	if cmd.Short != "Bubble Tea 交互式聊天" {
		t.Errorf("Short = %q, want Bubble Tea 交互式聊天", cmd.Short)
	}
}

func TestTuiHelp(t *testing.T) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"tui", "--help"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Bubble Tea") {
		t.Errorf("help output missing description: %q", out)
	}
	if !strings.Contains(out, "--provider") {
		t.Errorf("help output missing inherited --provider flag: %q", out)
	}
}

func TestRunTuiSetupRequiresAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	err := runTui(tuiCmd, nil)
	if err == nil {
		t.Fatal("expected setup error without API key")
	}
}

func TestExistingCommandsStillRegistered(t *testing.T) {
	for _, name := range []string{"chat", "once"} {
		if _, _, err := rootCmd.Find([]string{name}); err != nil {
			t.Errorf("%s command not found: %v", name, err)
		}
	}
}
