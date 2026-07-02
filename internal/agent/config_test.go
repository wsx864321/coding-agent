package agent

import (
	"strings"
	"testing"
)

func TestArchiveProjectBucket(t *testing.T) {
	tests := []struct {
		name    string
		workdir string
		expect  string // expected prefix (before the 12-char hash)
	}{
		{
			name:    "empty workdir falls back to cwd",
			workdir: "",
			// on Windows at D:\project\coding-agent, cwd path segments become D-project-coding-agent
			// just verify the result is non-empty and has the expected suffix pattern
		},
		{
			name:    "fallback to workspace",
			workdir: "C:\\",
			// C: → after trim of colon → "C" → join → "C"
			expect: "C",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := archiveProjectBucket(tt.workdir)
			if result == "" {
				t.Fatal("expected non-empty result")
			}

			// Must end with -<12-char-hex>
			dashIdx := strings.LastIndex(result, "-")
			if dashIdx < 0 {
				t.Fatalf("expected dash in result, got: %s", result)
			}
			hashPart := result[dashIdx+1:]
			if len(hashPart) != 12 {
				t.Fatalf("expected 12-char hex suffix, got %q in %s", hashPart, result)
			}
			for _, c := range hashPart {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
					t.Fatalf("hash suffix contains non-hex char %c in %s", c, result)
				}
			}

			namePart := result[:dashIdx]
			if tt.expect != "" {
				if namePart != tt.expect {
					t.Errorf("expected prefix %q, got %q (full: %s)", tt.expect, namePart, result)
				}
			}

			// Name should not contain illegal filename chars
			illegal := []string{"\\", "/", ":", "<", ">", "|", "?", "*", `"`}
			for _, ch := range illegal {
				if strings.Contains(namePart, ch) {
					t.Errorf("name part contains illegal char %q: %s", ch, namePart)
				}
			}
		})
	}
}

func TestArchiveProjectBucket_Encoding(t *testing.T) {
	// These tests work on the current OS; they verify encoding shape.
	tests := []struct {
		name    string
		workdir string
		contain string // substring the name part must contain
	}{
		// Windows paths (native on this OS)
		{name: "windows simple", workdir: `D:\project\apo`, contain: "D-project-apo"},
		{name: "windows with spaces", workdir: `D:\my projects\agent v2`, contain: "D-my"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := archiveProjectBucket(tt.workdir)
			dashIdx := strings.LastIndex(result, "-")
			namePart := result[:dashIdx]
			if tt.contain != "" && !strings.Contains(namePart, tt.contain) {
				t.Errorf("expected name %q to contain %q", namePart, tt.contain)
			}
		})
	}
}

func TestArchiveProjectBucket_SameHash(t *testing.T) {
	// Same path should produce same hash.
	a := archiveProjectBucket(`D:\project\apo`)
	b := archiveProjectBucket(`D:\project\apo`)
	if a != b {
		t.Errorf("same path should produce same result: %q vs %q", a, b)
	}
}

func TestArchiveProjectBucket_DifferentPaths(t *testing.T) {
	// Different paths should produce different hash.
	a := archiveProjectBucket(`D:\project\apo`)
	b := archiveProjectBucket(`D:\project\agent`)
	if a == b {
		t.Errorf("different paths should produce different results: %q", a)
	}
}
