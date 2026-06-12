package skill

import "testing"

func TestParseFrontmatter_Normal(t *testing.T) {
	content := `---
name: code-review
description: Perform code reviews
runAs: subagent
---

# Code Review

Review the code carefully.`

	meta, body := ParseFrontmatter(content)

	if meta["name"] != "code-review" {
		t.Errorf("name = %q, want %q", meta["name"], "code-review")
	}
	if meta["description"] != "Perform code reviews" {
		t.Errorf("description = %q, want %q", meta["description"], "Perform code reviews")
	}
	if meta["runAs"] != "subagent" {
		t.Errorf("runAs = %q, want %q", meta["runAs"], "subagent")
	}
	if body != "# Code Review\n\nReview the code carefully." {
		t.Errorf("body = %q", body)
	}
}

func TestParseFrontmatter_QuotedValues(t *testing.T) {
	content := `---
name: "my-skill"
description: 'A cool skill'
---

Body here.`

	meta, body := ParseFrontmatter(content)

	if meta["name"] != "my-skill" {
		t.Errorf("name = %q, want %q", meta["name"], "my-skill")
	}
	if meta["description"] != "A cool skill" {
		t.Errorf("description = %q, want %q", meta["description"], "A cool skill")
	}
	if body != "Body here." {
		t.Errorf("body = %q", body)
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	content := "# Just a normal markdown file\n\nNo frontmatter here."
	meta, body := ParseFrontmatter(content)

	if len(meta) != 0 {
		t.Errorf("meta should be empty, got %v", meta)
	}
	if body != content {
		t.Errorf("body should equal content")
	}
}

func TestParseFrontmatter_EmptyContent(t *testing.T) {
	meta, body := ParseFrontmatter("")
	if len(meta) != 0 {
		t.Errorf("meta should be empty")
	}
	if body != "" {
		t.Errorf("body should be empty")
	}
}

func TestParseFrontmatter_FrontmatterOnly(t *testing.T) {
	content := `---
name: test
---`

	meta, body := ParseFrontmatter(content)
	if meta["name"] != "test" {
		t.Errorf("name = %q, want %q", meta["name"], "test")
	}
	if body != "" {
		t.Errorf("body should be empty, got %q", body)
	}
}
