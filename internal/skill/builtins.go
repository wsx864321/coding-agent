package skill

import (
	"embed"
	"strings"
)

//go:embed builtins/*.md
var builtinFS embed.FS

// builtinSkills 返回所有内置 skill
func builtinSkills() []Skill {
	entries, err := builtinFS.ReadDir("builtins")
	if err != nil {
		return nil
	}

	var skills []Skill
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := builtinFS.ReadFile("builtins/" + entry.Name())
		if err != nil {
			continue
		}
		meta, body := ParseFrontmatter(string(data))

		name := meta["name"]
		if name == "" {
			name = strings.TrimSuffix(entry.Name(), ".md")
		}

		runAs := RunInline
		if strings.EqualFold(meta["runAs"], "subagent") {
			runAs = RunSubagent
		}

		skills = append(skills, Skill{
			Name:        name,
			Description: meta["description"],
			Body:        body,
			Scope:       ScopeBuiltin,
			Path:        "(builtin)",
			RunAs:       runAs,
		})
	}
	return skills
}
