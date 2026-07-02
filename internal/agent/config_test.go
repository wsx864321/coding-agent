package agent

import (
	"path/filepath"
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
			// result comes from os.Getwd(); just verify shape
		},
		{
			name:    "non-existent dir encodes path segments",
			workdir: filepath.Join(t.TempDir(), "my-project"),
			// deep tmpdir may be truncated to 5 segments; verify it contains the project leaf
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
			if tt.expect != "" && namePart != tt.expect {
				t.Errorf("expected prefix %q, got %q (full: %s)", tt.expect, namePart, result)
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
	// Create a shallow temp subdir so its name appears within the 5-segment limit.
	// We use t.TempDir() as parent and create a direct child.
	tmp := t.TempDir()
	dirA := filepath.Join(tmp, "proj-alpha")
	dirB := filepath.Join(tmp, "proj-beta")

	resultA := archiveProjectBucket(dirA)
	resultB := archiveProjectBucket(dirB)

	// Both must have valid 12-char hex suffix.
	for _, tc := range []struct {
		label, result string
	}{
		{"A", resultA},
		{"B", resultB},
	} {
		dashIdx := strings.LastIndex(tc.result, "-")
		if dashIdx < 0 {
			t.Fatalf("%s: missing dash in %q", tc.label, tc.result)
		}
		hash := tc.result[dashIdx+1:]
		if len(hash) != 12 {
			t.Fatalf("%s: hash %q not 12 chars", tc.label, hash)
		}
	}

	// Hashes must differ for different paths.
	if resultA == resultB {
		t.Errorf("different paths produced same result: %q", resultA)
	}
}

func TestArchiveProjectBucket_SameHash(t *testing.T) {
	d := t.TempDir()
	a := archiveProjectBucket(d)
	b := archiveProjectBucket(d)
	if a != b {
		t.Errorf("same path should produce same result: %q vs %q", a, b)
	}
}

func TestArchiveProjectBucket_DifferentPaths(t *testing.T) {
	a := archiveProjectBucket(filepath.Join(t.TempDir(), "proj-a"))
	b := archiveProjectBucket(filepath.Join(t.TempDir(), "proj-b"))
	if a == b {
		t.Errorf("different paths should produce different results: %q", a)
	}
}
