package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractPRNumber(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"https://github.com/owner/repo/pull/123", 123},
		{"PR #456 is ready", 456},
		{"no pr here", 0},
		{"https://github.com/owner/repo/pull/789/files", 789},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ExtractPRNumber(tt.input)
			if got != tt.want {
				t.Errorf("ExtractPRNumber(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewAnalyzer(t *testing.T) {
	a := New("", nil)

	if a.StateRoot != "." {
		t.Errorf("StateRoot = %q, want '.'", a.StateRoot)
	}

	if a.GHClient == nil {
		t.Error("GHClient should not be nil")
	}
}

func TestUpdateLoopCount(t *testing.T) {
	tmpDir := t.TempDir()
	a := New(tmpDir, nil)

	// First call should return 1
	count, err := a.updateLoopCount()
	if err != nil {
		t.Fatalf("updateLoopCount() error = %v", err)
	}
	if count != 1 {
		t.Errorf("first call count = %d, want 1", count)
	}

	// Second call should return 2
	count, err = a.updateLoopCount()
	if err != nil {
		t.Fatalf("updateLoopCount() error = %v", err)
	}
	if count != 2 {
		t.Errorf("second call count = %d, want 2", count)
	}
}

func TestReadConsecutiveFailures(t *testing.T) {
	tmpDir := t.TempDir()
	a := New(tmpDir, nil)

	// No file should return 0
	count := a.readConsecutiveFailures()
	if count != 0 {
		t.Errorf("no file count = %d, want 0", count)
	}

	// Create file with value
	stateDir := filepath.Join(tmpDir, ".ai", "state")
	os.MkdirAll(stateDir, 0755)
	os.WriteFile(filepath.Join(stateDir, "consecutive_failures"), []byte("3"), 0644)

	count = a.readConsecutiveFailures()
	if count != 3 {
		t.Errorf("with file count = %d, want 3", count)
	}
}

func TestConstants(t *testing.T) {
	if MaxLoop <= 0 {
		t.Error("MaxLoop should be positive")
	}

	if MaxConsecutiveFailures <= 0 {
		t.Error("MaxConsecutiveFailures should be positive")
	}
}
