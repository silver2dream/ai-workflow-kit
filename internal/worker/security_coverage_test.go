package worker

import (
	"runtime"
	"testing"
)

// ---------------------------------------------------------------------------
// normalizePath
// ---------------------------------------------------------------------------

func TestCov_NormalizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"path/to/file", "path/to/file"},
		{"path\\to\\file", "path/to/file"},
		{"path/trailing/", "path/trailing"},
		{"mixed\\path/file", "mixed/path/file"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizePath(tt.input)
			// On Windows, result is lowercased
			if runtime.GOOS == "windows" {
				if got != tt.want {
					t.Errorf("normalizePath(%q) = %q, want %q", tt.input, got, tt.want)
				}
			} else {
				if got != tt.want {
					t.Errorf("normalizePath(%q) = %q, want %q", tt.input, got, tt.want)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// splitLines
// ---------------------------------------------------------------------------

func TestCov_SplitLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"single line", "hello", 1},
		{"multiple lines", "a\nb\nc", 3},
		{"empty lines filtered", "a\n\nb\n\n", 2},
		{"whitespace only lines filtered", "a\n   \nb\n  \t  \n", 2},
		{"carriage returns", "a\r\nb\r\nc", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitLines(tt.input)
			if len(result) != tt.want {
				t.Errorf("splitLines(%q) = %d lines, want %d", tt.input, len(result), tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// findProtectedChanges (additional cases)
// ---------------------------------------------------------------------------

func TestCov_FindProtectedChanges_Additional(t *testing.T) {
	t.Run("empty file list", func(t *testing.T) {
		violations := findProtectedChanges(nil, "")
		if len(violations) != 0 {
			t.Errorf("expected 0 violations, got %d", len(violations))
		}
	})

	t.Run("empty string in file list", func(t *testing.T) {
		files := []string{"", ".ai/scripts/test.sh", ""}
		violations := findProtectedChanges(files, "")
		if len(violations) != 1 {
			t.Errorf("expected 1 violation, got %d", len(violations))
		}
	})

	t.Run("no protected files", func(t *testing.T) {
		files := []string{"src/main.go", "README.md", "internal/foo.go"}
		violations := findProtectedChanges(files, "")
		if len(violations) != 0 {
			t.Errorf("expected 0 violations, got %d", len(violations))
		}
	})

	t.Run("all protected", func(t *testing.T) {
		files := []string{".ai/scripts/a.sh", ".ai/commands/b.md"}
		violations := findProtectedChanges(files, "")
		if len(violations) != 2 {
			t.Errorf("expected 2 violations, got %d", len(violations))
		}
	})
}

// ---------------------------------------------------------------------------
// findSensitiveMatches (additional cases)
// ---------------------------------------------------------------------------

func TestCov_FindSensitiveMatches_Additional(t *testing.T) {
	t.Run("empty diff", func(t *testing.T) {
		matches := findSensitiveMatches("", nil)
		if len(matches) != 0 {
			t.Errorf("expected 0 matches, got %d", len(matches))
		}
	})

	t.Run("no sensitive patterns", func(t *testing.T) {
		matches := findSensitiveMatches("just normal code", nil)
		if len(matches) != 0 {
			t.Errorf("expected 0 matches, got %d", len(matches))
		}
	})

	t.Run("GITHUB_TOKEN detected", func(t *testing.T) {
		diff := "+export GITHUB_TOKEN=ghp_xxxx"
		matches := findSensitiveMatches(diff, nil)
		if len(matches) == 0 {
			t.Error("expected GITHUB_TOKEN to be detected")
		}
	})

	t.Run("private key detected", func(t *testing.T) {
		diff := "+-----BEGIN RSA PRIVATE KEY-----"
		matches := findSensitiveMatches(diff, nil)
		if len(matches) == 0 {
			t.Error("expected private key to be detected")
		}
	})

	t.Run("AWS key detected", func(t *testing.T) {
		diff := "+AWS_SECRET_ACCESS_KEY=xxxx"
		matches := findSensitiveMatches(diff, nil)
		if len(matches) == 0 {
			t.Error("expected AWS key to be detected")
		}
	})

	t.Run("empty custom pattern skipped", func(t *testing.T) {
		matches := findSensitiveMatches("some code", []string{"", ""})
		if len(matches) != 0 {
			t.Errorf("expected 0 matches for empty patterns, got %d", len(matches))
		}
	})

	t.Run("invalid custom regex skipped", func(t *testing.T) {
		matches := findSensitiveMatches("some code", []string{"[invalid"})
		if len(matches) != 0 {
			t.Errorf("expected 0 matches for invalid regex, got %d", len(matches))
		}
	})
}
