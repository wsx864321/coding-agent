package skill

import (
	"strings"
	"testing"
)

func TestApplyIndex_Empty(t *testing.T) {
	base := "你是一个编码助手。"
	result := ApplyIndex(base, nil)
	if result != base {
		t.Errorf("expected no change for empty skills, got %q", result)
	}
}

func TestApplyIndex_WithSkills(t *testing.T) {
	base := "你是一个编码助手。"
	skills := []Skill{
		{Name: "code-review", Description: "代码审查", RunAs: RunInline},
		{Name: "explore", Description: "代码探索", RunAs: RunSubagent},
	}

	result := ApplyIndex(base, skills)

	if !strings.Contains(result, "code-review") {
		t.Error("missing code-review in index")
	}
	if !strings.Contains(result, "explore") {
		t.Error("missing explore in index")
	}
	if !strings.Contains(result, "[subagent]") {
		t.Error("missing [subagent] tag for explore")
	}
	if !strings.HasPrefix(result, base) {
		t.Error("base prompt should be preserved")
	}
}

func TestCatalog_Empty(t *testing.T) {
	result := Catalog(nil)
	if !strings.Contains(result, "未加载") {
		t.Errorf("expected empty message, got %q", result)
	}
}

func TestCatalog_WithSkills(t *testing.T) {
	skills := []Skill{
		{Name: "skill-a", Description: "Desc A", RunAs: RunInline, Scope: ScopeProject},
		{Name: "skill-b", Description: "Desc B", RunAs: RunSubagent, Scope: ScopeBuiltin},
	}

	result := Catalog(skills)
	if !strings.Contains(result, "skill-a") {
		t.Error("missing skill-a")
	}
	if !strings.Contains(result, "subagent") {
		t.Error("missing subagent mode indicator")
	}
}
