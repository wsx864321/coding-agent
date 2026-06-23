package hooks

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeHooksJSON(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_ProjectAndGlobalMerge(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()

	writeHooksJSON(t, root, ".coding-agent/hooks.json", `{
	  "hooks": {
	    "PreToolUse": [{"command": "echo project", "match": "bash"}]
	  }
	}`)
	writeHooksJSON(t, home, ".coding-agent/hooks.json", `{
	  "hooks": {
	    "Stop": [{"command": "echo global"}]
	  }
	}`)

	got := Load(LoadOptions{ProjectRoot: root, HomeDir: home})
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	if got[0].Scope != ScopeProject || got[0].Command != "echo project" {
		t.Errorf("project hook: %+v", got[0])
	}
	if got[1].Scope != ScopeGlobal || got[1].Event != EventStop {
		t.Errorf("global hook: %+v", got[1])
	}
}

func TestLoad_GlobalOnly(t *testing.T) {
	home := t.TempDir()
	writeHooksJSON(t, home, ".coding-agent/hooks.json", `{
	  "hooks": {"PostToolUse": [{"command": "cat"}]}
	}`)
	got := Load(LoadOptions{ProjectRoot: t.TempDir(), HomeDir: home})
	if len(got) != 1 || got[0].Scope != ScopeGlobal {
		t.Fatalf("got=%+v", got)
	}
}

func TestLoad_NoConfig(t *testing.T) {
	got := Load(LoadOptions{ProjectRoot: t.TempDir(), HomeDir: t.TempDir()})
	if len(got) != 0 {
		t.Fatalf("expected empty, got %+v", got)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	root := t.TempDir()
	writeHooksJSON(t, root, ".coding-agent/hooks.json", `{invalid`)
	got := Load(LoadOptions{ProjectRoot: root, HomeDir: t.TempDir()})
	if len(got) != 0 {
		t.Fatalf("expected skip on bad JSON, got %+v", got)
	}
}

func TestLoad_DefaultTimeout(t *testing.T) {
	root := t.TempDir()
	writeHooksJSON(t, root, ".coding-agent/hooks.json", `{
	  "hooks": {"PreToolUse": [{"command": "true"}]}
	}`)
	got := Load(LoadOptions{ProjectRoot: root, HomeDir: t.TempDir()})
	if len(got) != 1 || got[0].Timeout != defaultHookTimeoutMs {
		t.Fatalf("timeout=%d, want %d", got[0].Timeout, defaultHookTimeoutMs)
	}
}

func TestLoad_ProjectHooksWhenHomeDirFails(t *testing.T) {
	root := t.TempDir()
	writeHooksJSON(t, root, ".coding-agent/hooks.json", `{
	  "hooks": {"PreToolUse": [{"command": "echo project", "match": "bash"}]}
	}`)

	old := userHomeDir
	userHomeDir = func() (string, error) { return "", errors.New("no home") }
	t.Cleanup(func() { userHomeDir = old })

	got := Load(LoadOptions{ProjectRoot: root})
	if len(got) != 1 {
		t.Fatalf("expected project hook only, got %+v", got)
	}
	if got[0].Scope != ScopeProject || got[0].Command != "echo project" {
		t.Errorf("project hook: %+v", got[0])
	}
	if got[0].compiledMatch == nil {
		t.Fatal("expected compiled match for bash")
	}
}

func TestLoad_InvalidMatchRegexSkipped(t *testing.T) {
	root := t.TempDir()
	writeHooksJSON(t, root, ".coding-agent/hooks.json", `{
	  "hooks": {"PreToolUse": [{"command": "bad-hook", "match": "("}]}
	}`)
	got := Load(LoadOptions{ProjectRoot: root, HomeDir: t.TempDir()})
	if len(got) != 0 {
		t.Fatalf("invalid regex hook should be skipped, got %+v", got)
	}
}

func TestLoad_CompilesMatchRegex(t *testing.T) {
	root := t.TempDir()
	writeHooksJSON(t, root, ".coding-agent/hooks.json", `{
	  "hooks": {"PreToolUse": [{"command": "echo", "match": "^bash$"}]}
	}`)
	got := Load(LoadOptions{ProjectRoot: root, HomeDir: t.TempDir()})
	if len(got) != 1 || got[0].compiledMatch == nil {
		t.Fatalf("expected compiled match, got %+v", got)
	}
	if !got[0].compiledMatch.MatchString("bash") {
		t.Fatal("compiled match should match bash")
	}
}
