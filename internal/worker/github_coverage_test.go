package worker

import (
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// IssueInfo.HasLabel
// ---------------------------------------------------------------------------

func TestCov_IssueInfo_HasLabel_Extended(t *testing.T) {
	info := &IssueInfo{
		Labels: []string{"ai-task", "in-progress", "P0"},
	}

	tests := []struct {
		label string
		want  bool
	}{
		{"ai-task", true},
		{"in-progress", true},
		{"P0", true},
		{"missing", false},
		{"AI-TASK", true},   // case insensitive
		{"In-Progress", true}, // case insensitive
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			if got := info.HasLabel(tt.label); got != tt.want {
				t.Errorf("HasLabel(%q) = %v, want %v", tt.label, got, tt.want)
			}
		})
	}
}

func TestCov_IssueInfo_HasLabel_EmptyLabels(t *testing.T) {
	info := &IssueInfo{Labels: nil}
	if info.HasLabel("anything") {
		t.Error("expected false for nil labels")
	}
}

// ---------------------------------------------------------------------------
// IssueInfo.IsOpen
// ---------------------------------------------------------------------------

func TestCov_IssueInfo_IsOpen_Extended(t *testing.T) {
	tests := []struct {
		state string
		want  bool
	}{
		{"OPEN", true},
		{"open", true},
		{"Open", true},
		{"CLOSED", false},
		{"closed", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			info := &IssueInfo{State: tt.state}
			if got := info.IsOpen(); got != tt.want {
				t.Errorf("IsOpen() = %v, want %v (state=%q)", got, tt.want, tt.state)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ExtractPRNumber
// ---------------------------------------------------------------------------

func TestCov_ExtractPRNumber_Extended(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"empty", "", 0},
		{"full URL", "https://github.com/org/repo/pull/42", 42},
		{"full URL with text", "Created PR at https://github.com/org/repo/pull/123 done", 123},
		{"relative URL", "/pull/99", 99},
		{"PR hash ref", "PR #456", 456},
		{"PR no space", "PR#789", 789},
		{"pull request ref", "pull request #100", 100},
		{"no match", "just some text", 0},
		{"issues not matched", "https://github.com/org/repo/issues/42", 0},
		{"pulls list not matched", "/pulls/42", 0},
		{"large number", "https://github.com/org/repo/pull/99999", 99999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractPRNumber(tt.body); got != tt.want {
				t.Errorf("ExtractPRNumber(%q) = %d, want %d", tt.body, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// NewGitHubClient
// ---------------------------------------------------------------------------

func TestCov_NewGitHubClient(t *testing.T) {
	t.Run("default timeout", func(t *testing.T) {
		client := NewGitHubClient(0)
		if client.Timeout != 30*time.Second {
			t.Errorf("expected 30s default, got %v", client.Timeout)
		}
	})

	t.Run("custom timeout", func(t *testing.T) {
		client := NewGitHubClient(60 * time.Second)
		if client.Timeout != 60*time.Second {
			t.Errorf("expected 60s, got %v", client.Timeout)
		}
	})
}
