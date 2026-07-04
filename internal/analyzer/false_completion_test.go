package analyzer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestParseIssueRefNumber(t *testing.T) {
	cases := map[string]int{
		"- [ ] 1. Foundation <!-- Issue #1 -->":  1,
		"- [ ] 12. Big task <!-- Issue #123 -->": 123,
		"- [ ] 3. No reference here":             0,
		"- [ ] 4. Malformed <!-- Issue # -->":    0,
	}
	for line, want := range cases {
		if got := parseIssueRefNumber(line); got != want {
			t.Errorf("parseIssueRefNumber(%q) = %d, want %d", line, got, want)
		}
	}
}

// writeSpecTasks writes a one-task tasks.md (with an issue ref) for spec "demo".
func writeSpecTasks(t *testing.T, tmpDir, line string) *Config {
	t.Helper()
	specDir := filepath.Join(tmpDir, ".ai", "specs", "demo")
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "tasks.md"), []byte(line+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := &Config{}
	cfg.Specs.BasePath = ".ai/specs"
	cfg.Specs.Active = []string{"demo"}
	cfg.GitHub.Labels = DefaultLabels()
	return cfg
}

func TestCheckTasksFiles_FalseCompletion(t *testing.T) {
	const taskLine = "- [ ] 1. Foundation <!-- Issue #1 -->"

	t.Run("closed issue with no merged PR is re-scheduled", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := writeSpecTasks(t, tmpDir, taskLine)
		mock := NewMockGitHubClient()
		mock.IssueStates[1] = "CLOSED" // closed, but no merged branch recorded
		a := newTestAnalyzer(tmpDir, cfg, mock)

		d := a.checkTasksFiles(context.Background())
		if d == nil || d.NextAction != ActionDispatchWorker || d.IssueNumber != 1 {
			t.Fatalf("want dispatch_worker for #1, got %+v", d)
		}
		if len(mock.ReopenedIssues) != 1 || mock.ReopenedIssues[0] != 1 {
			t.Errorf("issue #1 should have been reopened, got %v", mock.ReopenedIssues)
		}
	})

	t.Run("genuinely merged task is left alone", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := writeSpecTasks(t, tmpDir, taskLine)
		mock := NewMockGitHubClient()
		mock.IssueStates[1] = "CLOSED"
		mock.MergedBranches["feat/ai-issue-1"] = true
		a := newTestAnalyzer(tmpDir, cfg, mock)

		if d := a.checkTasksFiles(context.Background()); d != nil {
			t.Fatalf("a merged task must not be re-scheduled, got %+v", d)
		}
		if len(mock.ReopenedIssues) != 0 {
			t.Errorf("a merged task must not be reopened, got %v", mock.ReopenedIssues)
		}
	})

	t.Run("open in-progress task is not a false completion", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := writeSpecTasks(t, tmpDir, taskLine)
		mock := NewMockGitHubClient()
		mock.IssueStates[1] = "OPEN" // still in the pipeline — handled by earlier steps
		a := newTestAnalyzer(tmpDir, cfg, mock)

		if d := a.checkTasksFiles(context.Background()); d != nil {
			t.Fatalf("an open task must be left to earlier steps, got %+v", d)
		}
	})

	t.Run("API error never misclassifies as false completion", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := writeSpecTasks(t, tmpDir, taskLine)
		mock := NewMockGitHubClient()
		mock.IssueStates[1] = "CLOSED"
		mock.MergedBranchesError = fmt.Errorf("github down")
		a := newTestAnalyzer(tmpDir, cfg, mock)

		if d := a.checkTasksFiles(context.Background()); d != nil {
			t.Fatalf("on API error the analyzer must not re-schedule, got %+v", d)
		}
	})

	t.Run("task with no issue ref still takes create_task priority", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfg := writeSpecTasks(t, tmpDir, "- [ ] 1. Foundation (never dispatched)")
		mock := NewMockGitHubClient()
		a := newTestAnalyzer(tmpDir, cfg, mock)

		d := a.checkTasksFiles(context.Background())
		if d == nil || d.NextAction != ActionCreateTask {
			t.Fatalf("want create_task for an unreferenced task, got %+v", d)
		}
	})
}
