package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
)

const defaultHookTimeoutMs = 10000

var userHomeDir = os.UserHomeDir

type LoadOptions struct {
	ProjectRoot string
	HomeDir     string // defaults to os.UserHomeDir()
}

func Load(opts LoadOptions) []ResolvedHook {
	var out []ResolvedHook
	if opts.ProjectRoot != "" {
		p := filepath.Join(opts.ProjectRoot, ".coding-agent", "hooks.json")
		out = append(out, loadFile(p, ScopeProject)...)
	}

	home := opts.HomeDir
	if home == "" {
		var err error
		home, err = userHomeDir()
		if err != nil {
			return out
		}
	}
	globalPath := filepath.Join(home, ".coding-agent", "hooks.json")
	out = append(out, loadFile(globalPath, ScopeGlobal)...)
	return out
}

func loadFile(path string, scope Scope) []ResolvedHook {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil
	}
	abs, _ := filepath.Abs(path)
	var resolved []ResolvedHook
	for event, configs := range settings.Hooks {
		for _, cfg := range configs {
			if cfg.Command == "" {
				continue
			}
			if cfg.Timeout <= 0 {
				cfg.Timeout = defaultHookTimeoutMs
			}
			h := ResolvedHook{
				HookConfig: cfg,
				Event:      event,
				Scope:      scope,
				Source:     abs,
			}
			if cfg.Match != "" {
				re, err := regexp.Compile(cfg.Match)
				if err != nil {
					continue
				}
				h.compiledMatch = re
			}
			resolved = append(resolved, h)
		}
	}
	return resolved
}
