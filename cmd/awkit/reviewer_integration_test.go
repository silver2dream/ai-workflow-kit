package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Integration tests for awkit reviewer commands
// Converted from .ai/tests/test_review_scripts.sh

// findAwkitBinaryForReviewer locates the awkit binary for testing
func findAwkitBinaryForReviewer(t *testing.T) string {
	t.Helper()

	// First try to find in repo root
	repoRoot := getReviewerTestRepoRoot(t)
	paths := []string{
		filepath.Join(repoRoot, "awkit"),
		filepath.Join(repoRoot, "awkit.exe"),
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Try PATH
	if path, err := exec.LookPath("awkit"); err == nil {
		return path
	}

	t.Skip("awkit binary not found, skipping integration test")
	return ""
}

// getReviewerTestRepoRoot returns the repository root directory for tests
func getReviewerTestRepoRoot(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	// Traverse up to find go.mod (repo root)
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root from %s", wd)
		}
		dir = parent
	}
}

func TestReviewer_PrepareReviewHelp(t *testing.T) {
	awkit := findAwkitBinaryForReviewer(t)

	t.Run("awkit prepare-review --help works", func(t *testing.T) {
		cmd := exec.Command(awkit, "prepare-review", "--help")
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Some commands exit with non-zero for --help, that's OK if we get output
			if len(output) == 0 {
				t.Fatalf("prepare-review --help failed with no output: %v", err)
			}
		}
	})

	t.Run("prepare-review help mentions PR/issue/diff", func(t *testing.T) {
		cmd := exec.Command(awkit, "prepare-review", "--help")
		output, _ := cmd.CombinedOutput()
		outputStr := strings.ToLower(string(output))

		if !strings.Contains(outputStr, "pr") &&
			!strings.Contains(outputStr, "issue") &&
			!strings.Contains(outputStr, "diff") {
			t.Error("prepare-review help should mention PR, issue, or diff")
		}
	})
}

func TestReviewer_SubmitReviewHelp(t *testing.T) {
	awkit := findAwkitBinaryForReviewer(t)

	t.Run("awkit submit-review --help works", func(t *testing.T) {
		cmd := exec.Command(awkit, "submit-review", "--help")
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Some commands exit with non-zero for --help, that's OK if we get output
			if len(output) == 0 {
				t.Fatalf("submit-review --help failed with no output: %v", err)
			}
		}
	})

	t.Run("submit-review help mentions score/decision/review", func(t *testing.T) {
		cmd := exec.Command(awkit, "submit-review", "--help")
		output, _ := cmd.CombinedOutput()
		outputStr := strings.ToLower(string(output))

		if !strings.Contains(outputStr, "score") &&
			!strings.Contains(outputStr, "decision") &&
			!strings.Contains(outputStr, "review") {
			t.Error("submit-review help should mention score, decision, or review")
		}
	})
}

func TestReviewer_GoImplementationFilesExist(t *testing.T) {
	repoRoot := getReviewerTestRepoRoot(t)

	tests := []struct {
		name string
		path string
	}{
		{"internal/reviewer/prepare.go", filepath.Join(repoRoot, "internal", "reviewer", "prepare.go")},
		{"internal/reviewer/submit.go", filepath.Join(repoRoot, "internal", "reviewer", "submit.go")},
	}

	for _, tt := range tests {
		t.Run(tt.name+" exists", func(t *testing.T) {
			if _, err := os.Stat(tt.path); os.IsNotExist(err) {
				t.Errorf("%s does not exist", tt.name)
			}
		})
	}
}

func TestReviewer_PrepareGoHandlesIssueContent(t *testing.T) {
	repoRoot := getReviewerTestRepoRoot(t)
	prepareFile := filepath.Join(repoRoot, "internal", "reviewer", "prepare.go")

	content, err := os.ReadFile(prepareFile)
	if err != nil {
		t.Fatalf("failed to read prepare.go: %v", err)
	}

	contentStr := strings.ToLower(string(content))
	if !strings.Contains(contentStr, "issue") {
		t.Error("prepare.go should handle Issue content")
	}
}

func TestReviewer_PrepareGoHandlesPRChanges(t *testing.T) {
	repoRoot := getReviewerTestRepoRoot(t)
	prepareFile := filepath.Join(repoRoot, "internal", "reviewer", "prepare.go")

	content, err := os.ReadFile(prepareFile)
	if err != nil {
		t.Fatalf("failed to read prepare.go: %v", err)
	}

	contentStr := strings.ToLower(string(content))
	// The implementation uses commits to show PR changes instead of diff
	if !strings.Contains(contentStr, "commits") && !strings.Contains(contentStr, "diff") {
		t.Error("prepare.go should handle PR changes (commits or diff)")
	}
}

func TestReviewer_SubmitGoHandlesMerge(t *testing.T) {
	repoRoot := getReviewerTestRepoRoot(t)
	submitFile := filepath.Join(repoRoot, "internal", "reviewer", "submit.go")

	content, err := os.ReadFile(submitFile)
	if err != nil {
		t.Fatalf("failed to read submit.go: %v", err)
	}

	contentStr := strings.ToLower(string(content))
	if !strings.Contains(contentStr, "merge") {
		t.Error("submit.go should handle merge")
	}
}

func TestReviewer_RunIssueHelp(t *testing.T) {
	awkit := findAwkitBinaryForReviewer(t)

	t.Run("awkit run-issue --help works", func(t *testing.T) {
		cmd := exec.Command(awkit, "run-issue", "--help")
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Some commands exit with non-zero for --help, that's OK if we get output
			if len(output) == 0 {
				t.Fatalf("run-issue --help failed with no output: %v", err)
			}
		}
	})

	t.Run("run-issue help mentions issue", func(t *testing.T) {
		cmd := exec.Command(awkit, "run-issue", "--help")
		output, _ := cmd.CombinedOutput()
		outputStr := strings.ToLower(string(output))

		if !strings.Contains(outputStr, "issue") {
			t.Error("run-issue help should mention issue")
		}
	})
}

func TestReviewer_WorkerImplementationFilesExist(t *testing.T) {
	repoRoot := getReviewerTestRepoRoot(t)

	t.Run("internal/worker/runner.go exists", func(t *testing.T) {
		runnerFile := filepath.Join(repoRoot, "internal", "worker", "runner.go")
		if _, err := os.Stat(runnerFile); os.IsNotExist(err) {
			t.Error("internal/worker/runner.go does not exist")
		}
	})
}

func TestReviewer_WorkerHandlesAGENTSmd(t *testing.T) {
	repoRoot := getReviewerTestRepoRoot(t)

	// Check runner.go or ticket.go for AGENTS reference
	files := []string{
		filepath.Join(repoRoot, "internal", "worker", "runner.go"),
		filepath.Join(repoRoot, "internal", "worker", "ticket.go"),
	}

	found := false
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		contentStr := strings.ToLower(string(content))
		if strings.Contains(contentStr, "agents") {
			found = true
			break
		}
	}

	if !found {
		t.Error("worker implementation should reference AGENTS.md")
	}
}
